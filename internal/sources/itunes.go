package sources

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"sort"
	"strings"
)

const ITunesXML = "itunes-library-xml-v1"
const ITunesJSON = "itunes-library-json-v1"
const ITunesExpanded = "itunes-expanded-json-v1"

// Each envelope retains a complete source entry. The library header retains
// unknown top-level fields separately from tracks and whole playlists.
type itunesRecord struct {
	Kind      string         `json:"kind"`
	LibraryID string         `json:"library_id,omitempty"`
	Snapshot  string         `json:"snapshot,omitempty"`
	TrackKey  string         `json:"track_key,omitempty"`
	Fields    map[string]any `json:"fields"`
}

// Repeated dictionary keys occur in real iTunes exports. Keep every value,
// distinguishing repetitions from a source array instead of taking the last.
type plistRepeated struct {
	Values []any `json:"$plist_repeated"`
}

// Apple's plist XML has an external DTD declaration. encoding/xml does not
// resolve external entities. Reject unsupported values rather than dropping them.
func plistValue(d *xml.Decoder, start xml.StartElement, depth int) (any, error) {
	if depth > 64 {
		return nil, fmt.Errorf("Library nesting exceeds limit.")
	}
	if start.Name.Local == "dict" || start.Name.Local == "array" {
		object := map[string]any{}
		array := []any{}
		var key *string
		for {
			token, err := d.Token()
			if err != nil {
				return nil, err
			}
			switch t := token.(type) {
			case xml.EndElement:
				if t.Name != start.Name || key != nil {
					return nil, fmt.Errorf("Invalid library structure.")
				}
				if start.Name.Local == "dict" {
					return object, nil
				}
				return array, nil
			case xml.CharData:
				if strings.TrimSpace(string(t)) != "" {
					return nil, fmt.Errorf("Invalid library structure.")
				}
			case xml.StartElement:
				if start.Name.Local == "dict" && key == nil {
					if t.Name.Local != "key" {
						return nil, fmt.Errorf("Library key expected.")
					}
					var name string
					if err := d.DecodeElement(&name, &t); err != nil {
						return nil, err
					}
					key = &name
					continue
				}
				value, err := plistValue(d, t, depth+1)
				if err != nil {
					return nil, err
				}
				if start.Name.Local == "dict" {
					if previous, exists := object[*key]; exists {
						repeated, ok := previous.(plistRepeated)
						if !ok {
							repeated.Values = []any{previous}
						}
						repeated.Values = append(repeated.Values, value)
						object[*key] = repeated
					} else {
						object[*key] = value
					}
					key = nil
				} else {
					array = append(array, value)
				}
			}
		}
	}
	var content strings.Builder
scalar:
	for {
		token, err := d.Token()
		if err != nil {
			return nil, err
		}
		switch t := token.(type) {
		case xml.CharData:
			content.Write(t)
		case xml.EndElement:
			if t.Name != start.Name {
				return nil, fmt.Errorf("Invalid library value.")
			}
			break scalar
		case xml.StartElement:
			return nil, fmt.Errorf("Invalid nested library value.")
		}
	}
	text := content.String()
	switch start.Name.Local {
	case "string", "date", "data":
		return text, nil
	case "integer", "real":
		number := json.Number(strings.TrimSpace(text))
		if _, err := json.Marshal(number); err != nil {
			return nil, fmt.Errorf("Invalid library number.")
		}
		return number, nil
	case "true", "false":
		if strings.TrimSpace(text) != "" {
			return nil, fmt.Errorf("Invalid library boolean.")
		}
		return start.Name.Local == "true", nil
	default:
		return nil, fmt.Errorf("Unsupported library value: %s", start.Name.Local)
	}
}

func parseITunesXML(data []byte) (any, error) {
	d := xml.NewDecoder(bytes.NewReader(data))
	var root xml.StartElement
	for {
		t, err := d.Token()
		if err != nil {
			return nil, err
		}
		if start, ok := t.(xml.StartElement); ok {
			root = start
			break
		}
	}
	if root.Name.Local != "plist" {
		return nil, ErrUnsupportedFile
	}
	var value any
	seen := false
	closed := false
	for {
		t, err := d.Token()
		if err == io.EOF {
			if !seen || !closed {
				return nil, ErrUnsupportedFile
			}
			return value, nil
		}
		if err != nil {
			return nil, err
		}
		if start, ok := t.(xml.StartElement); ok {
			if seen || closed || start.Name.Local != "dict" {
				return nil, ErrUnsupportedFile
			}
			value, err = plistValue(d, start, 0)
			if err != nil {
				return nil, err
			}
			seen = true
		}
		if end, ok := t.(xml.EndElement); ok {
			if end.Name != root.Name || !seen {
				return nil, ErrUnsupportedFile
			}
			closed = true
		}
		if text, ok := t.(xml.CharData); ok && strings.TrimSpace(string(text)) != "" {
			return nil, ErrUnsupportedFile
		}
	}
}

func parseITunes(data []byte) ([]json.RawMessage, string, error) {
	var value any
	parser := ITunesJSON
	if bytes.HasPrefix(data, []byte("<")) {
		parser = ITunesXML
		var err error
		value, err = parseITunesXML(data)
		if err != nil {
			return nil, parser, err
		}
	} else {
		d := json.NewDecoder(bytes.NewReader(data))
		d.UseNumber()
		if err := d.Decode(&value); err != nil {
			return nil, parser, ErrUnsupportedFile
		}
		if d.Decode(new(any)) != io.EOF {
			return nil, parser, ErrUnsupportedFile
		}
	}
	var out []json.RawMessage
	appendRecord := func(kind, library, snapshot, key string, fields map[string]any) error {
		raw, err := json.Marshal(itunesRecord{kind, library, snapshot, key, fields})
		if err == nil {
			out = append(out, raw)
		}
		return err
	}
	if root, ok := value.(map[string]any); ok {
		tracks, ok := root["Tracks"].(map[string]any)
		if !ok {
			return nil, parser, ErrUnsupportedFile
		}
		library, _ := root["Library Persistent ID"].(string)
		snapshot, _ := root["Date"].(string)
		header := map[string]any{}
		for k, v := range root {
			if k != "Tracks" && k != "Playlists" {
				header[k] = v
			}
		}
		if err := appendRecord("library", library, snapshot, "", header); err != nil {
			return nil, parser, err
		}
		keys := make([]string, 0, len(tracks))
		for k := range tracks {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fields, ok := tracks[k].(map[string]any)
			if !ok {
				return nil, parser, fmt.Errorf("Invalid library track.")
			}
			if err := appendRecord("library_track", library, snapshot, k, fields); err != nil {
				return nil, parser, err
			}
		}
		if lists, exists := root["Playlists"]; exists {
			playlists, ok := lists.([]any)
			if !ok {
				return nil, parser, fmt.Errorf("Invalid library playlists.")
			}
			for _, p := range playlists {
				fields, ok := p.(map[string]any)
				if !ok {
					return nil, parser, fmt.Errorf("Invalid library playlist.")
				}
				if err := appendRecord("playlist", library, snapshot, "", fields); err != nil {
					return nil, parser, err
				}
			}
		}
		return out, parser, nil
	}
	tracks, ok := value.([]any)
	if !ok || len(tracks) == 0 {
		return nil, parser, ErrUnsupportedFile
	}
	for _, track := range tracks {
		fields, ok := track.(map[string]any)
		if !ok || fields["Track ID"] == nil || (fields["Persistent ID"] == nil && fields["Track Type"] == nil) {
			return nil, parser, ErrUnsupportedFile
		}
		kind := "library_track"
		if _, derived := fields["Estimated Play Time"]; derived {
			kind = "derived_play"
			parser = ITunesExpanded
		}
		if err := appendRecord(kind, "", "", "", fields); err != nil {
			return nil, parser, err
		}
	}
	return out, parser, nil
}

// The R conversion wraps scalar fields in one-element arrays. Preserve those
// arrays in records and unwrap them only when interpreting a scalar value.
func itunesScalar(raw json.RawMessage) json.RawMessage {
	if bytes.HasPrefix(bytes.TrimSpace(raw), []byte("[")) {
		var values []json.RawMessage
		if json.Unmarshal(raw, &values) != nil || len(values) != 1 {
			return nil
		}
		return values[0]
	}
	return raw
}

func normalizeITunes(raw json.RawMessage) recordValues {
	var row struct {
		Kind   string                     `json:"kind"`
		Fields map[string]json.RawMessage `json:"fields"`
	}
	v := recordValues{Origin: "library", Reason: "library evidence", TimeMeaning: "unknown"}
	if json.Unmarshal(raw, &row) != nil {
		v.Reason = "invalid library record"
		return v
	}
	v.Title = rawString(itunesScalar(row.Fields["Name"]))
	v.Artist = rawString(itunesScalar(row.Fields["Artist"]))
	v.Album = rawString(itunesScalar(row.Fields["Album"]))
	if row.Kind == "derived_play" {
		v.Origin = "derived"
		v.Reason = "derived estimate"
	}
	return v
}

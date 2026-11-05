package sources

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"time"
)

const LastfmCSV = "lastfm-csv-v1"
const LastfmJSON = "lastfm-json-v1"
const LastfmJSONL = "lastfm-jsonl-v1"

var ErrUnsupportedFile = errors.New("Unsupported file.")

type parsedFile struct {
	file      File
	records   []json.RawMessage
	service   string
	parser    string
	snapshots []parsedSnapshot
}

// CSV records keep every original column and value. The original file remains
// authoritative for quoting, line endings, BOM, and container metadata.
type csvRecord struct {
	Columns []string `json:"columns"`
	Values  []string `json:"values"`
}

func (s *Store) parseFile(f File, filenames ...string) (parsedFile, error) {
	p, _ := s.FilePath(f.Hash)
	data, err := os.ReadFile(p)
	if err != nil {
		return parsedFile{}, err
	}
	clean := bytes.TrimSpace(bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf}))
	if len(clean) == 0 {
		return parsedFile{}, ErrUnsupportedFile
	}
	filename := ""
	if len(filenames) > 0 {
		filename = filenames[0]
	}
	if part, e := parseSnapshots(f, clean, filename); !errors.Is(e, ErrUnsupportedFile) {
		return part, e
	}
	if clean[0] == '<' || ((clean[0] == '[' || clean[0] == '{') && (bytes.Contains(clean, []byte(`"Tracks"`)) || bytes.Contains(clean, []byte(`"Track ID"`)))) {
		records, parser, e := parseITunes(clean)
		if e == nil || clean[0] == '<' || !errors.Is(e, ErrUnsupportedFile) {
			return parsedFile{file: f, records: records, service: "itunes", parser: parser}, e
		}
	}
	if clean[0] == '[' {
		if bytes.Equal(clean, []byte("[]")) && strings.HasPrefix(baseFilename(filename), "StreamingHistory") {
			return parsedFile{file: f, service: "spotify", parser: SpotifyStandard}, nil
		}
		if records, e := parseSpotifyStandard(clean); e == nil {
			return parsedFile{file: f, records: records, service: "spotify", parser: SpotifyStandard}, nil
		}
		if records, e := s.parseSpotify(f); e == nil {
			return parsedFile{file: f, records: records, service: "spotify", parser: Parser}, nil
		}
	}
	if clean[0] == '[' || clean[0] == '{' {
		if json.Valid(clean) {
			records, e := parseLastfmJSON(clean)
			return parsedFile{file: f, records: records, service: "lastfm", parser: LastfmJSON}, e
		}
		scanner := bufio.NewScanner(bytes.NewReader(clean))
		scanner.Buffer(make([]byte, 4096), 8<<20)
		var records []json.RawMessage
		for scanner.Scan() {
			line := bytes.TrimSpace(scanner.Bytes())
			if len(line) == 0 {
				continue
			}
			rows, e := parseLastfmJSON(line)
			if e != nil {
				return parsedFile{}, e
			}
			records = append(records, rows...)
		}
		if err = scanner.Err(); err != nil {
			return parsedFile{}, err
		}
		if len(records) > 0 {
			return parsedFile{file: f, records: records, service: "lastfm", parser: LastfmJSONL}, nil
		}
		return parsedFile{}, ErrUnsupportedFile
	}
	records, err := parseLastfmCSV(clean)
	return parsedFile{file: f, records: records, service: "lastfm", parser: LastfmCSV}, err
}

func parseLastfmJSON(data []byte) ([]json.RawMessage, error) {
	var out []json.RawMessage
	recognized := false
	var walk func(json.RawMessage, bool) error
	walk = func(raw json.RawMessage, trackList bool) error {
		raw = bytes.TrimSpace(raw)
		if len(raw) == 0 {
			return ErrUnsupportedFile
		}
		if raw[0] == '[' {
			var rows []json.RawMessage
			if err := json.Unmarshal(raw, &rows); err != nil {
				return err
			}
			for _, row := range rows {
				if err := walk(row, trackList); err != nil {
					return err
				}
			}
			return nil
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
			return ErrUnsupportedFile
		}
		for _, key := range []string{"recenttracks", "track", "scrobbles"} {
			if child, ok := fields[key]; ok && (bytes.HasPrefix(bytes.TrimSpace(child), []byte("[")) || bytes.HasPrefix(bytes.TrimSpace(child), []byte("{"))) {
				recognized = true
				return walk(child, key == "track" || key == "scrobbles")
			}
		}
		var sourceType string
		_ = json.Unmarshal(fields["source_type"], &sourceType)
		_, hasName := fields["name"]
		_, hasArtist := fields["artist"]
		_, hasDate := fields["date"]
		_, hasAttr := fields["@attr"]
		if trackList || strings.HasPrefix(strings.ToLower(sourceType), "lastfm") || (hasName && hasArtist && (hasDate || hasAttr)) {
			out = append(out, bytes.Clone(raw))
			return nil
		}
		return ErrUnsupportedFile
	}
	if err := walk(data, false); err != nil {
		return nil, err
	}
	if len(out) == 0 && !recognized {
		return nil, ErrUnsupportedFile
	}
	return out, nil
}

func parseLastfmCSV(data []byte) ([]json.RawMessage, error) {
	reader := csv.NewReader(bytes.NewReader(data))
	reader.FieldsPerRecord = -1
	first, err := reader.Read()
	if err != nil {
		return nil, ErrUnsupportedFile
	}
	columns := first
	named := false
	known := map[string]bool{}
	for _, c := range columns {
		known[strings.ToLower(strings.TrimSpace(c))] = true
	}
	if known["artist"] && known["title"] && known["timestamp"] {
		named = true
	}
	if !named {
		if len(first) != 4 || !isLastfmDate(first[3]) {
			return nil, ErrUnsupportedFile
		}
		columns = []string{"Artist", "Album", "Title", "Timestamp"}
	}
	var out []json.RawMessage
	appendRow := func(row []string) error {
		raw, e := json.Marshal(csvRecord{Columns: columns, Values: row})
		if e == nil {
			out = append(out, raw)
		}
		return e
	}
	if !named {
		if err = appendRow(first); err != nil {
			return nil, err
		}
	}
	for {
		row, e := reader.Read()
		if e == io.EOF {
			break
		}
		if e != nil {
			return nil, e
		}
		if err = appendRow(row); err != nil {
			return nil, err
		}
	}
	return out, nil
}

var wallDateLayouts = []string{"02 Jan 2006, 15:04:05", "02 Jan 2006, 15:04", "1/2/2006 15:04:05", "1/2/2006 15:04", "2006-01-02 15:04:05", "2006-01-02 15:04", "2006-01-02T15:04:05", "2006-01-02T15:04"}

func isLastfmDate(value string) bool {
	if _, e := time.Parse(time.RFC3339Nano, value); e == nil {
		return true
	}
	for _, layout := range wallDateLayouts {
		if _, e := time.Parse(layout, value); e == nil {
			return true
		}
	}
	return false
}

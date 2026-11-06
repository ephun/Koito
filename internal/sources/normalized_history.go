package sources

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
)

const NormalizedJSONL = "normalized-history-jsonl-v1"
const LedgerDB = "listening-ledger-sqlite-v1"

func normalizedService(kind string) string {
	if strings.HasPrefix(strings.ToLower(kind), "yargfm") {
		return "yargfm"
	}
	for _, service := range []string{"spotify", "lastfm", "itunes", "setlistfm", "yarg"} {
		if strings.HasPrefix(strings.ToLower(kind), service) {
			return service
		}
	}
	return "legacy"
}

func parseNormalizedHistory(file File, data []byte) (parsedFile, error) {
	part := parsedFile{file: file, parser: NormalizedJSONL}
	probe := data
	if len(probe) > 4096 {
		probe = probe[:4096]
	}
	if !bytes.Contains(probe, []byte(`"source_type"`)) || !bytes.Contains(probe, []byte(`"payload_json"`)) {
		return part, ErrUnsupportedFile
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 4096), 32<<20)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var row map[string]json.RawMessage
		if json.Unmarshal(line, &row) != nil || row["source_type"] == nil || row["payload_json"] == nil || row["played_at"] == nil {
			return part, ErrUnsupportedFile
		}
		service := normalizedService(rawString(row["source_type"]))
		if part.service == "" {
			part.service = service
		} else if part.service != service {
			part.service = "legacy"
		}
		part.records = append(part.records, bytes.Clone(line))
	}
	if err := scanner.Err(); err != nil {
		return part, err
	}
	if len(part.records) == 0 {
		return part, ErrUnsupportedFile
	}
	// Last.fm conversions already have a preserved-record reader. Keep its
	// parser identity so an existing import does not acquire a second reading.
	if part.service == "lastfm" {
		return part, ErrUnsupportedFile
	}
	return part, nil
}

// A retained original payload takes precedence over dates from the old
// analysis. Expanded cumulative entries keep their original derived status.
func normalizeHistoryRow(row map[string]any, zone string) recordValues {
	fields := jsonObject(row["payload_json"])
	if fields == nil {
		fields = jsonObject(row["payload"])
	}
	if value, recognized := normalizeLegacyConcert(row, fields); recognized {
		return value
	}
	parser := ""
	switch {
	case fields["ts"] != nil:
		parser = Parser
	case fields["endTime"] != nil:
		parser = SpotifyStandard
	case fields["date"] != nil:
		parser = LastfmJSON
	case fields["track_metadata"] != nil:
		parser = ListenBrainzJSON
	case fields["data"] != nil && fields["meta"] != nil:
		v := normalizeMultiPlay(fields)
		v.Origin = "converted"
		v.Forwarder = "multi-scrobbler"
		return v
	case normalizedService(valueText(row["source_type"])) == "lastfm":
		raw, _ := json.Marshal(row)
		v := normalizeRecord(raw, LastfmJSONL, zone)
		v.OriginalService = "lastfm"
		v.OriginalFormat = LastfmCSV
		return v
	}
	if parser != "" {
		raw, _ := json.Marshal(fields)
		v := normalizeRecord(raw, parser, zone)
		v.Origin = "converted"
		v.OriginalFormat = parser
		switch parser {
		case Parser, SpotifyStandard:
			v.OriginalService = "spotify"
		case LastfmJSON:
			v.OriginalService = "lastfm"
		case ListenBrainzJSON:
			if v.OriginalService == "" {
				v.OriginalService = "listenbrainz"
			}
		}
		return v
	}
	v := recordValues{Title: valueText(row["title_raw"]), Artist: valueText(row["artist_raw"]), Album: valueText(row["album_raw"]), Origin: "derived", Reason: "legacy derived history", TimeMeaning: "estimated"}
	if v.Title == "" {
		v.Title = valueText(row["title"])
	}
	if v.Artist == "" {
		v.Artist = valueText(row["artist"])
	}
	if v.Album == "" {
		v.Album = valueText(row["album"])
	}
	v.TimeText = valueText(row["played_at"])
	v.Time, v.Precision, _ = interpretTime(v.TimeText, zone)
	v.OriginalService = valueText(row["source_service"])
	if v.OriginalService == "" {
		v.OriginalService = normalizedService(valueText(row["source_type"]))
	}
	v.NativeTrackID = valueText(fields["Persistent ID"])
	return v
}

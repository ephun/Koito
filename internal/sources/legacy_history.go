package sources

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

const ListenBrainzJSON = "listenbrainz-json-v1"
const ListenBrainzJSONL = "listenbrainz-jsonl-v1"
const MalojaJSON = "maloja-json-v1"
const KoitoJSON = "koito-json-v1"

func legacyHistoryParser(parser string) bool {
	return parser == ListenBrainzJSON || parser == ListenBrainzJSONL || parser == MalojaJSON || parser == KoitoJSON
}

func parseLegacyHistory(file File, data []byte) (parsedFile, error) {
	part := parsedFile{file: file}
	var root map[string]json.RawMessage
	if json.Unmarshal(data, &root) == nil && root != nil {
		if root["version"] != nil && root["listens"] != nil {
			if rawString(root["version"]) != "1" {
				return part, errors.New("Unsupported Koito export version.")
			}
			if err := json.Unmarshal(root["listens"], &part.records); err != nil {
				return part, err
			}
			part.service, part.parser = "koito", KoitoJSON
			return part, nil
		}
		if root["scrobbles"] != nil {
			var records []json.RawMessage
			if json.Unmarshal(root["scrobbles"], &records) == nil {
				recognized := len(records) == 0
				for _, raw := range records {
					var row map[string]json.RawMessage
					_ = json.Unmarshal(raw, &row)
					if row["time"] != nil && row["track"] != nil {
						recognized = true
					}
				}
				if recognized {
					part.service, part.parser, part.records = "maloja", MalojaJSON, records
					return part, nil
				}
			}
		}
		if root["track_metadata"] != nil {
			part.service, part.parser, part.records = "listenbrainz", ListenBrainzJSON, []json.RawMessage{bytes.Clone(data)}
			return part, nil
		}
		if payload, ok := root["payload"]; ok {
			var rows []json.RawMessage
			var page map[string]json.RawMessage
			if json.Unmarshal(payload, &page) == nil && page["listens"] != nil {
				payload = page["listens"]
			}
			if json.Unmarshal(payload, &rows) == nil && len(rows) > 0 {
				var first map[string]json.RawMessage
				_ = json.Unmarshal(rows[0], &first)
				if first["track_metadata"] != nil {
					part.service, part.parser, part.records = "listenbrainz", ListenBrainzJSON, rows
					return part, nil
				}
			}
		}
	}
	var rows []json.RawMessage
	if bytes.HasPrefix(data, []byte("[")) && json.Unmarshal(data, &rows) == nil && len(rows) > 0 {
		var first map[string]json.RawMessage
		_ = json.Unmarshal(rows[0], &first)
		if first["track_metadata"] != nil {
			part.service, part.parser, part.records = "listenbrainz", ListenBrainzJSON, rows
			return part, nil
		}
	}
	if !json.Valid(data) && bytes.HasPrefix(data, []byte("{")) {
		scanner := bufio.NewScanner(bytes.NewReader(data))
		scanner.Buffer(make([]byte, 4096), 32<<20)
		var records []json.RawMessage
		recognized := false
		for scanner.Scan() {
			line := bytes.TrimSpace(scanner.Bytes())
			if len(line) == 0 {
				continue
			}
			var row map[string]json.RawMessage
			if json.Unmarshal(line, &row) != nil {
				if recognized {
					return part, errors.New("Invalid ListenBrainz history record.")
				}
				return part, ErrUnsupportedFile
			}
			if row["track_metadata"] != nil {
				recognized = true
			}
			records = append(records, bytes.Clone(line))
		}
		if err := scanner.Err(); err != nil {
			return part, err
		}
		if len(records) > 0 && recognized {
			part.service, part.parser, part.records = "listenbrainz", ListenBrainzJSONL, records
			return part, nil
		}
	}
	return part, ErrUnsupportedFile
}

func primaryAlias(value any) string {
	list, _ := value.([]any)
	for _, entry := range list {
		row := object(entry)
		if yes, _ := row["is_primary"].(bool); yes {
			return valueText(row["alias"])
		}
	}
	if len(list) > 0 {
		return valueText(object(list[0])["alias"])
	}
	return ""
}

func normalizeLegacyHistory(raw json.RawMessage, parser string) recordValues {
	var row map[string]any
	_ = json.Unmarshal(raw, &row)
	v := recordValues{Reason: "counted", Origin: "converted", TimeMeaning: "start", Precision: int64(time.Second)}
	switch parser {
	case ListenBrainzJSON, ListenBrainzJSONL:
		meta := object(row["track_metadata"])
		extra := object(meta["additional_info"])
		v.Title, v.Artist, v.Album = valueText(meta["track_name"]), valueText(meta["artist_name"]), valueText(meta["release_name"])
		v.RecordingID = valueText(extra["recording_mbid"])
		if v.RecordingID == "" {
			v.RecordingID = valueText(object(meta["mbid_mapping"])["recording_mbid"])
		}
		v.URI = valueText(extra["spotify_id"])
		if v.URI != "" && !strings.HasPrefix(v.URI, "spotify:track:") {
			v.URI = strings.Replace(v.URI, "https://open.spotify.com/track/", "spotify:track:", 1)
		}
		v.OriginalService = valueText(extra["music_service_name"])
		v.Forwarder = valueText(extra["submission_client"])
		v.Device = valueText(extra["device_name"])
		v.DurationMS = numeric(extra["duration_ms"])
		if v.DurationMS == 0 {
			v.DurationMS = durationMilliseconds(extra["duration"])
		}
		v.TimeText = valueText(row["listened_at"])
		seconds := numeric(row["listened_at"])
		if seconds > 0 {
			v.Time = time.Unix(seconds, 0).UTC()
		} else {
			v.Reason = "invalid timestamp"
		}
	case MalojaJSON:
		track := object(row["track"])
		v.Title = valueText(track["title"])
		v.Album = valueText(object(track["album"])["albumtitle"])
		artists, _ := track["artists"].([]any)
		if len(artists) > 0 {
			v.Artist = valueText(artists[0])
			for _, artist := range artists {
				if text := valueText(artist); strings.Contains(text, " • ") {
					v.Artist = strings.SplitN(text, " • ", 2)[0]
					break
				}
			}
		}
		v.TimeText = valueText(row["time"])
		seconds := numeric(row["time"])
		if seconds > 0 {
			v.Time = time.Unix(seconds, 0).UTC()
		} else {
			v.Reason = "invalid timestamp"
		}
	case KoitoJSON:
		v.Title = primaryAlias(object(row["track"])["aliases"])
		v.Album = primaryAlias(object(row["album"])["aliases"])
		artists, _ := row["artists"].([]any)
		for _, artist := range artists {
			if primary, _ := object(artist)["is_primary"].(bool); primary {
				v.Artist = primaryAlias(object(artist)["aliases"])
				break
			}
		}
		if v.Artist == "" && len(artists) > 0 {
			v.Artist = primaryAlias(object(artists[0])["aliases"])
		}
		v.RecordingID = valueText(object(row["track"])["mbid"])
		v.DurationMS = durationMilliseconds(object(row["track"])["duration"])
		v.TimeText = valueText(row["listened_at"])
		v.Time, v.Precision = explicitTime(v.TimeText)
		if v.Time.IsZero() {
			v.Reason = "invalid timestamp"
		}
		v.Forwarder = valueText(row["client"])
		client := strings.ToLower(v.Forwarder)
		if strings.Contains(client, "setlist") || strings.Contains(client, "itunes") {
			v.Origin = "derived"
			v.Reason = "legacy derived history"
		}
	}
	if v.Reason == "counted" && (v.Title == "" || v.Artist == "") {
		v.Reason = "not an identified music track"
	}
	return v
}

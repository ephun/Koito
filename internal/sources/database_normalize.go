package sources

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"time"
)

func object(value any) map[string]any { v, _ := value.(map[string]any); return v }
func jsonObject(value any) map[string]any {
	if text, ok := value.(string); ok {
		var v map[string]any
		_ = json.Unmarshal([]byte(text), &v)
		return v
	}
	return object(value)
}
func numeric(value any) int64 {
	text := valueText(value)
	if n, e := strconv.ParseInt(text, 10, 64); e == nil {
		return n
	}
	var n float64
	if f, ok := value.(float64); ok {
		n = f
	}
	return int64(n)
}

func normalizeDatabase(raw json.RawMessage, parser, zone string) recordValues {
	var row databaseRecord
	if json.Unmarshal(raw, &row) != nil {
		return recordValues{Reason: "invalid fields"}
	}
	v := recordValues{Reason: "database context", Origin: "original"}
	switch databaseFormat(parser) {
	case NavidromeDB:
		media := object(row.Context["media"])
		if row.Table == "media_file" {
			media = row.Fields
		}
		v.Title, v.Artist, v.Album = valueText(media["title"]), valueText(media["artist"]), valueText(media["album"])
		v.RecordingID = valueText(media["mbz_recording_id"])
		v.NativeTrackID = valueText(media["id"])
		v.DurationMS = durationMilliseconds(media["duration"])
		if user := valueText(row.Fields["user_id"]); user != "" {
			v.NativeLibraryID = "navidrome:" + user
		} else if library := valueText(media["library_id"]); library != "" {
			v.NativeLibraryID = "navidrome:library:" + library
		}
		if row.Kind == "play" {
			v.TimeText = valueText(row.Fields["submission_time"])
			v.TimeMeaning = "submission"
			v.Precision = int64(time.Second)
			seconds := numeric(row.Fields["submission_time"])
			if seconds > 0 {
				v.Time = time.Unix(seconds, 0).UTC()
				v.Reason = "counted"
			} else {
				v.Reason = "invalid timestamp"
			}
			v.EventID = "navidrome:" + valueText(row.Fields["user_id"]) + ":" + row.Key + ":" + valueText(row.Fields["media_file_id"]) + ":" + v.TimeText
		}
	case MultiDB:
		v = normalizeMultiPlay(row.Fields["play"])
		v.Forwarder = "multi-scrobbler"
		component := object(row.Context["component"])
		uid := valueText(row.Fields["uid"])
		if uid != "" {
			v.EventID = "multi:" + uid
		}
		if row.Kind != "play" {
			v.Reason = "forwarding evidence"
			parent := object(row.Context["parent"])
			if root := valueText(parent["root_uid"]); root != "" {
				v.SupportID = "multi:" + root
			} else if uid := valueText(parent["uid"]); uid != "" {
				v.SupportID = "multi:" + uid
			}
			if valueText(component["mode"]) == "client" {
				v.Endpoint = valueText(component["name"])
			}
		}
		if row.Kind == "play" && valueText(row.Fields["state"]) == "duped" {
			v.Reason = "source duplicate"
		}
	case NormalizedDB, KoitoDB, LedgerDB:
		if row.Kind == "legacy_record" {
			v = normalizeHistoryRow(row.Fields, zone)
		}
		if row.Kind == "native_play" {
			v = normalizeNativeKoito(row)
		}
	}
	if v.Reason == "counted" && (strings.TrimSpace(v.Artist) == "" || strings.TrimSpace(v.Title) == "") {
		v.Reason = "not an identified music track"
	}
	return v
}

func normalizeMultiPlay(value any) recordValues {
	play := jsonObject(value)
	data := object(play["data"])
	meta := object(play["meta"])
	v := recordValues{Reason: "counted", Origin: "forwarded", TimeMeaning: "start"}
	v.Title, v.Album = valueText(data["track"]), valueText(data["album"])
	v.DurationMS = durationMilliseconds(data["duration"])
	if artists, ok := data["artists"].([]any); ok {
		names := []string{}
		for _, artist := range artists {
			name := valueText(object(artist)["name"])
			if name == "" {
				name = valueText(artist)
			}
			if name != "" {
				names = append(names, name)
			}
		}
		if len(names) > 0 {
			v.Artist = names[0]
		}
	}
	v.TimeText = valueText(data["playDate"])
	v.Time, v.Precision = explicitTime(v.TimeText)
	if v.Time.IsZero() {
		v.Reason = "invalid timestamp"
	}
	v.OriginalService = valueText(meta["musicService"])
	if v.OriginalService == "" {
		v.OriginalService = valueText(meta["source"])
	}
	v.Device = valueText(meta["deviceId"])
	spotify := object(object(data["meta"])["spotify"])
	if id := valueText(spotify["track"]); id != "" {
		if strings.HasPrefix(id, "spotify:track:") {
			v.URI = id
		} else {
			v.URI = "spotify:track:" + id
		}
	}
	if listened, ok := data["listenedFor"].(float64); ok {
		if listened < 0 || listened >= float64(math.MaxInt64)/1000 {
			v.Reason = "invalid played duration"
		} else {
			v.PlayedMS = int64(listened * 1000)
		}
	}
	if now, _ := meta["nowPlaying"].(bool); now {
		v.NowPlaying = true
		v.Reason = "now playing"
	}
	if v.Reason == "counted" && (v.Title == "" || v.Artist == "") {
		v.Reason = "not an identified music track"
	}
	return v
}

func durationMilliseconds(value any) int64 {
	number, err := strconv.ParseFloat(valueText(value), 64)
	if err != nil || math.IsNaN(number) || math.IsInf(number, 0) || number < 0 || number >= float64(math.MaxInt64)/1000 {
		return 0
	}
	return int64(number * 1000)
}

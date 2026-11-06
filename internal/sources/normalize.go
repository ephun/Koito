package sources

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gabehf/koito/internal/music"
)

type recordValues struct {
	SourceInput      string               `json:"source_input,omitempty"`
	TimeEstimated    bool                 `json:"time_estimated,omitempty"`
	Performance      *performanceValues   `json:"performance,omitempty"`
	EventID          string               `json:"event_id,omitempty"`
	SupportID        string               `json:"support_id,omitempty"`
	NativeLibraryID  string               `json:"native_library_id,omitempty"`
	NativeTrackID    string               `json:"native_track_id,omitempty"`
	OriginalService  string               `json:"original_service,omitempty"`
	OriginalFormat   string               `json:"original_format,omitempty"`
	Forwarder        string               `json:"forwarder,omitempty"`
	Endpoint         string               `json:"endpoint,omitempty"`
	Device           string               `json:"device,omitempty"`
	ReportedComplete *bool                `json:"reported_complete,omitempty"`
	Music            *music.MatchIdentity `json:"music,omitempty"`
	Title            string               `json:"title"`
	Artist           string               `json:"artist"`
	Album            string               `json:"album"`
	URI              string               `json:"uri,omitempty"`
	RecordingID      string               `json:"recording_id,omitempty"`
	TimeText         string               `json:"time_text"`
	TimeMeaning      string               `json:"time_meaning"`
	Precision        int64                `json:"precision_ns"`
	Time             time.Time            `json:"-"`
	PlayedMS         int64                `json:"played_ms,omitempty"`
	DurationMS       int64                `json:"duration_ms,omitempty"`
	Reason           string               `json:"reason"`
	Origin           string               `json:"origin"`
	NowPlaying       bool                 `json:"now_playing,omitempty"`
}

func rawString(raw json.RawMessage) string {
	if bytes.Equal(raw, []byte("true")) || bytes.Equal(raw, []byte("false")) {
		return string(raw)
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	var number json.Number
	if json.Unmarshal(raw, &number) == nil {
		return number.String()
	}
	return ""
}

func itemText(raw json.RawMessage) string {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) == nil && fields != nil {
		if text := rawString(fields["#text"]); text != "" {
			return text
		}
		return rawString(fields["name"])
	}
	return rawString(raw)
}

func normalizeRecord(raw json.RawMessage, parser, zone string) recordValues {
	if parser == MultiAPI {
		return normalizeMultiAPI(raw)
	}
	if parser == SetlistJSON {
		return normalizeConcert(raw)
	}
	if gameBinaryParser(parser) {
		var row databaseRecord
		if json.Unmarshal(raw, &row) != nil {
			return recordValues{Reason: "invalid fields"}
		}
		service := "clonehero"
		if parser == YARGCache {
			service = "yarg"
		}
		return recordValues{Title: valueText(row.Fields["title"]), Artist: valueText(row.Fields["artist"]), Album: valueText(row.Fields["album"]), NativeTrackID: valueText(row.Fields["chart_checksum"]), DurationMS: numeric(row.Fields["duration_ms"]), Origin: "original", OriginalService: service, Reason: "game summary"}
	}
	if parser == NormalizedJSONL {
		var row map[string]any
		_ = json.Unmarshal(raw, &row)
		return normalizeHistoryRow(row, zone)
	}
	if parser == MultiCache || parser == MultiLog {
		return normalizeMultiHistory(raw, parser)
	}
	if legacyHistoryParser(parser) {
		return normalizeLegacyHistory(raw, parser)
	}
	if databaseParser(parser) {
		return normalizeDatabase(raw, parser, zone)
	}
	if snapshotParser(parser) {
		return normalizeSnapshot(raw, parser)
	}
	if parser == SpotifyStandard {
		return normalizeSpotifyStandard(raw)
	}
	if parser == ITunesXML || parser == ITunesJSON || parser == ITunesExpanded {
		return normalizeITunes(raw)
	}
	v := recordValues{Reason: "counted", Origin: "original", TimeMeaning: "start", Precision: int64(time.Second)}
	if parser == Parser {
		var item spotifyItem
		if json.Unmarshal(raw, &item) != nil {
			v.Reason = "invalid fields"
			return v
		}
		v.Title = item.Title
		v.Artist = item.Artist
		v.Album = item.Album
		v.URI = item.URI
		v.PlayedMS = item.Played
		v.Device = item.Platform
		if item.ReasonEnd != "" {
			complete := item.ReasonEnd == "trackdone"
			v.ReportedComplete = &complete
		}
		v.TimeText = item.Timestamp
		v.TimeMeaning = "end"
		v.Time, v.Precision = explicitTime(v.TimeText)
		if v.Time.IsZero() {
			v.Reason = "invalid timestamp"
		} else if v.PlayedMS < 0 {
			v.Reason = "invalid played duration"
		} else if item.ReasonEnd != "trackdone" {
			v.Reason = "not reported complete"
		}
	} else if parser == LastfmCSV {
		var row csvRecord
		if json.Unmarshal(raw, &row) != nil {
			v.Reason = "invalid fields"
			return v
		}
		fields := map[string]string{}
		for i, name := range row.Columns {
			if i < len(row.Values) {
				fields[strings.ToLower(strings.TrimSpace(name))] = row.Values[i]
			}
		}
		v.Title = fields["title"]
		v.Artist = fields["artist"]
		v.Album = fields["album"]
		v.TimeText = fields["timestamp"]
		v.Time, v.Precision, v.Reason = interpretTime(v.TimeText, zone)
		v.Origin = "csv"
	} else {
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil || fields == nil {
			v.Reason = "invalid fields"
			return v
		}
		if strings.HasPrefix(strings.ToLower(rawString(fields["source_type"])), "lastfm") {
			v = normalizeConvertedLastfm(fields, zone)
		} else {
			v = normalizeNativeLastfm(fields, zone)
		}
	}
	if strings.TrimSpace(v.Title) == "" || strings.TrimSpace(v.Artist) == "" {
		v.Reason = "not an identified music track"
	}
	if v.NowPlaying {
		v.Reason = "now playing"
	}
	return v
}

func normalizeNativeLastfm(fields map[string]json.RawMessage, zone string) recordValues {
	v := recordValues{Title: rawString(fields["name"]), Artist: itemText(fields["artist"]), Album: itemText(fields["album"]), Reason: "counted", Origin: "original", TimeMeaning: "start", Precision: int64(time.Second)}
	v.URI = rawString(fields["spotify_track_uri"])
	// Last.fm's generic mbid field is not asserted to be a recording ID. Keep
	// it in the raw record; only use explicitly identified recording IDs here.
	v.RecordingID = rawString(fields["recording_mbid"])
	var attrs map[string]json.RawMessage
	_ = json.Unmarshal(fields["@attr"], &attrs)
	v.NowPlaying = strings.EqualFold(rawString(attrs["nowplaying"]), "true")
	var date map[string]json.RawMessage
	_ = json.Unmarshal(fields["date"], &date)
	if unix := rawString(date["uts"]); unix != "" {
		v.TimeText = unix
		seconds, err := strconv.ParseInt(unix, 10, 64)
		if err != nil || seconds <= 0 || seconds > 253402300799 {
			v.Reason = "invalid timestamp"
		} else {
			v.Time = time.Unix(seconds, 0).UTC()
		}
	} else {
		v.TimeText = rawString(date["#text"])
		v.Time, v.Precision, v.Reason = interpretTime(v.TimeText, zone)
	}
	return v
}

func normalizeConvertedLastfm(fields map[string]json.RawMessage, zone string) recordValues {
	var original map[string]json.RawMessage
	payload := fields["payload_json"]
	if len(payload) > 0 && payload[0] == '"' {
		var text string
		_ = json.Unmarshal(payload, &text)
		payload = []byte(text)
	}
	_ = json.Unmarshal(payload, &original)
	if original != nil && original["date"] != nil {
		v := normalizeNativeLastfm(original, zone)
		v.Origin = "converted"
		return v
	}
	v := recordValues{Title: rawString(fields["title_raw"]), Artist: rawString(fields["artist_raw"]), Album: rawString(fields["album_raw"]), Reason: "counted", Origin: "converted", TimeMeaning: "start", Precision: int64(time.Second)}
	if original != nil {
		if title, exists := original["Title"]; exists {
			v.Title = rawString(title)
		}
		if artist, exists := original["Artist"]; exists {
			v.Artist = rawString(artist)
		}
		if album, exists := original["Album"]; exists {
			v.Album = rawString(album)
		}
		v.TimeText = rawString(original["Timestamp"])
	}
	// The old CSV converter appended Z without establishing the original zone.
	// Prefer its retained original timestamp and preserve that uncertainty.
	if v.TimeText == "" {
		v.TimeText = rawString(fields["played_at"])
		if strings.Contains(strings.ToLower(rawString(fields["source_type"])), "csv") {
			v.TimeText = strings.TrimSuffix(v.TimeText, "Z")
		}
	}
	v.Time, v.Precision, v.Reason = interpretTime(v.TimeText, zone)
	return v
}

func explicitTime(text string) (time.Time, int64) {
	t, err := time.Parse(time.RFC3339Nano, text)
	if err != nil {
		return time.Time{}, int64(time.Second)
	}
	precision := int64(time.Second)
	if dot := strings.IndexByte(text, '.'); dot >= 0 {
		for i := dot + 1; i < len(text) && text[i] >= '0' && text[i] <= '9' && precision > 1; i++ {
			precision /= 10
		}
	}
	return t, precision
}

func interpretTime(text, zone string) (time.Time, int64, string) {
	if t, precision := explicitTime(text); !t.IsZero() {
		return t, precision, "counted"
	}
	var wall time.Time
	var precision int64
	for _, layout := range wallDateLayouts {
		if t, err := time.Parse(layout, text); err == nil {
			wall = t
			precision = int64(time.Second)
			if !strings.Contains(layout, "05") {
				precision = int64(time.Minute)
			}
			break
		}
	}
	if wall.IsZero() {
		return time.Time{}, 0, "invalid timestamp"
	}
	if zone == "" {
		return time.Time{}, precision, "timezone needed"
	}
	location, err := time.LoadLocation(zone)
	if err != nil {
		return time.Time{}, precision, "invalid timezone"
	}
	// Enumerate the surrounding zone offsets. A wall time can have zero valid
	// instants in a spring gap or two in an autumn fold; neither is guessed.
	offsets := map[int]bool{}
	for _, hours := range []int{-48, -24, 0, 24, 48} {
		_, offset := wall.Add(time.Duration(hours) * time.Hour).In(location).Zone()
		offsets[offset] = true
	}
	var candidates []time.Time
	for offset := range offsets {
		candidate := wall.Add(-time.Duration(offset) * time.Second)
		local := candidate.In(location)
		if local.Year() == wall.Year() && local.Month() == wall.Month() && local.Day() == wall.Day() && local.Hour() == wall.Hour() && local.Minute() == wall.Minute() && local.Second() == wall.Second() {
			candidates = append(candidates, candidate)
		}
	}
	if len(candidates) == 0 {
		return time.Time{}, precision, "nonexistent local time"
	}
	if len(candidates) > 1 {
		return time.Time{}, precision, "ambiguous local time"
	}
	return candidates[0].UTC(), precision, "counted"
}

func requireZone(zone string) error {
	if zone == "" {
		return nil
	}
	if _, err := time.LoadLocation(zone); err != nil {
		return fmt.Errorf("Unknown time zone: %s", zone)
	}
	return nil
}

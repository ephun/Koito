package sources

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

const SetlistJSON = "setlistfm-json-v1"

var legacyConcertAlbum = regexp.MustCompile(`^Live at (.*) \((\d{2}-\d{2}-\d{4})\)$`)
var legacyConcertReference = regexp.MustCompile(`^SetlistFM Concert ([a-fA-F0-9]+)$`)

func normalizeLegacyConcert(row, payload map[string]any) (recordValues, bool) {
	fields := object(payload["native_original_fields"])
	if !strings.Contains(strings.ToLower(valueText(fields["client"])), "setlist") {
		return recordValues{}, false
	}
	v := recordValues{Title: valueText(row["title"]), Artist: valueText(row["artist"]), Album: valueText(fields["release_title"]), Origin: "derived", OriginalService: "setlistfm", OriginalFormat: "koito-native-listen", Forwarder: valueText(fields["client"]), TimeMeaning: "date", Precision: int64(24 * time.Hour), Reason: "unresolved concert date"}
	if v.Title == "" {
		v.Title = valueText(row["title_raw"])
	}
	if v.Artist == "" {
		v.Artist = valueText(row["artist_raw"])
	}
	v.EventID = valueText(payload["native_event_id"])
	if v.EventID == "" {
		v.EventID = fmt.Sprintf("koito-concert:%s:%s:%s", valueText(fields["user_id"]), valueText(fields["track_id"]), valueText(fields["listened_at"]))
	}
	v.Performance = &performanceValues{Kind: "concert", DateOnly: true, Performer: v.Artist, Legacy: true}
	if reference := legacyConcertReference.FindStringSubmatch(v.Album); len(reference) == 2 {
		v.Performance.SetlistID = strings.ToLower(reference[1])
		v.Performance.ID = v.Performance.SetlistID
	}
	parts := legacyConcertAlbum.FindStringSubmatch(v.Album)
	if len(parts) != 3 {
		return v, true
	}
	date, err := time.Parse("02-01-2006", parts[2])
	if err != nil {
		return v, true
	}
	v.Time, v.TimeText = date, parts[2]
	v.Performance.LocalDate, v.Performance.Venue = date.Format("2006-01-02"), parts[1]
	if v.Title != "" && v.Artist != "" {
		v.Reason = "counted"
	}
	return v, true
}

func jsonList(value any, key string) []any {
	if rows, ok := value.([]any); ok {
		return rows
	}
	if object := object(value); object != nil {
		if nested, ok := object[key]; ok {
			return jsonList(nested, key)
		}
		return []any{object}
	}
	return nil
}

func parseSetlists(file File, data []byte) (parsedFile, error) {
	part := parsedFile{file: file, service: "setlistfm", parser: SetlistJSON}
	var root any
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	if d.Decode(&root) != nil {
		return part, ErrUnsupportedFile
	}
	var walk func(any, map[string]any) bool
	emit := func(kind, key string, fields, context map[string]any) {
		raw, _ := json.Marshal(databaseRecord{Kind: kind, Table: "setlists", Key: key, Fields: fields, Context: context})
		part.records = append(part.records, raw)
	}
	walk = func(value any, inherited map[string]any) bool {
		if rows, ok := value.([]any); ok {
			if len(rows) == 0 {
				return false
			}
			for _, row := range rows {
				if !walk(row, inherited) {
					return false
				}
			}
			return true
		}
		fields := object(value)
		if fields == nil {
			return false
		}
		if valueText(fields["type"]) == "setlist" || fields["eventDate"] != nil && fields["artist"] != nil {
			id, revision := valueText(fields["id"]), valueText(fields["versionId"])
			metadata := map[string]any{}
			for field, value := range fields {
				if field != "sets" && field != "set" {
					metadata[field] = value
				}
			}
			if metadata["attended"] == nil && inherited != nil {
				metadata["attended"] = inherited["attended"]
				metadata["attendance"] = inherited["attendance"]
			}
			encoded, _ := json.Marshal(fields)
			metadata["revision_hash"] = digest(encoded)
			emit("concert", id+":"+revision, fields, map[string]any{"performance": metadata})
			sets := fields["sets"]
			if sets == nil {
				sets = fields["set"]
			}
			for setIndex, setValue := range jsonList(sets, "set") {
				set := object(setValue)
				for songIndex, songValue := range jsonList(set["song"], "song") {
					song := object(songValue)
					if song == nil {
						return false
					}
					emit("concert_song", fmt.Sprintf("%s:%s:%d:%d", id, revision, setIndex, songIndex), song, map[string]any{"performance": metadata, "set": setIndex + 1, "position": songIndex + 1, "set_fields": set})
				}
			}
			return true
		}
		for _, name := range []string{"setlists", "setlist"} {
			if child, exists := fields[name]; exists {
				metadata := map[string]any{}
				for key, value := range fields {
					if key != name {
						metadata[key] = value
					}
				}
				emit("concert_page", "page", metadata, nil)
				if rows, ok := child.([]any); ok && len(rows) == 0 {
					return true
				}
				return walk(child, metadata)
			}
		}
		return false
	}
	if !walk(root, nil) {
		return part, ErrUnsupportedFile
	}
	return part, nil
}

func normalizeConcert(raw json.RawMessage) recordValues {
	var row databaseRecord
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if d.Decode(&row) != nil {
		return recordValues{Reason: "invalid fields"}
	}
	v := recordValues{Origin: "original", OriginalService: "setlistfm", Reason: "concert context", TimeMeaning: "date", Precision: int64(24 * time.Hour)}
	metadata := object(row.Context["performance"])
	if metadata == nil {
		return v
	}
	p := &performanceValues{Kind: "concert", ID: valueText(metadata["id"]), SetlistID: valueText(metadata["id"]), Revision: valueText(metadata["versionId"]), RevisionHash: valueText(metadata["revision_hash"]), Updated: valueText(metadata["lastUpdated"]), DateOnly: true, Venue: valueText(object(metadata["venue"])["name"]), Performer: valueText(object(metadata["artist"])["name"])}
	if attended, ok := metadata["attended"].(bool); ok {
		p.Attended = &attended
	} else if attended, ok := object(metadata["attendance"])["attended"].(bool); ok {
		p.Attended = &attended
	}
	v.Performance = p
	date, err := time.Parse("02-01-2006", valueText(metadata["eventDate"]))
	if err != nil {
		v.Reason = "invalid concert date"
		return v
	}
	p.LocalDate = date.Format("2006-01-02")
	v.TimeText = valueText(metadata["eventDate"])
	// The calendar boundary orders date-only events. It is not a claimed song
	// timestamp. The view and matching retain the full day and date-only flag.
	v.Time = date
	if row.Kind != "concert_song" {
		return v
	}
	p.Set, p.Position = int(numeric(row.Context["set"])), int(numeric(row.Context["position"]))
	p.Encore = int(numeric(object(row.Context["set_fields"])["encore"]))
	p.CoverArtist = valueText(object(row.Fields["cover"])["name"])
	p.Tape, _ = row.Fields["tape"].(bool)
	v.Title, v.Artist = valueText(row.Fields["name"]), p.Performer
	v.EventID = fmt.Sprintf("setlist:%s:%d:%d", p.SetlistID, p.Set, p.Position)
	v.Reason = "counted"
	switch {
	case p.SetlistID == "":
		v.Reason = "missing performance identifier"
	case strings.TrimSpace(v.Title) == "" || strings.TrimSpace(v.Artist) == "":
		v.Reason = "not an identified music track"
	case p.Tape:
		v.Reason = "not reported complete"
	}
	return v
}

func setlistUpdated(value string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.999999999-0700"} {
		if t, err := time.Parse(layout, value); err == nil {
			return t
		}
	}
	return time.Time{}
}

func selectConcertRevisions(records []matchingRecord) {
	type revision struct {
		updated time.Time
		hash    string
	}
	versions := map[string]map[string]revision{}
	groupKey := func(r matchingRecord) string { return fmt.Sprintf("%d:%s", r.UserID, r.Values.Performance.SetlistID) }
	for _, r := range records {
		p := r.Values.Performance
		if p == nil || p.Kind != "concert" || p.SetlistID == "" || p.Legacy {
			continue
		}
		key := groupKey(r)
		if versions[key] == nil {
			versions[key] = map[string]revision{}
		}
		id := p.Revision
		if id == "" {
			id = p.RevisionHash
		}
		updated := setlistUpdated(p.Updated)
		previous, exists := versions[key][id]
		if exists && previous.hash != p.RevisionHash {
			versions[key][id] = revision{hash: "conflict"}
		} else {
			versions[key][id] = revision{updated, p.RevisionHash}
		}
	}
	selected := map[string]string{}
	for key, revisions := range versions {
		var latest time.Time
		conflict := false
		for id, candidate := range revisions {
			if candidate.hash == "conflict" || len(revisions) > 1 && candidate.updated.IsZero() {
				conflict = true
				break
			}
			if selected[key] == "" || candidate.updated.After(latest) {
				selected[key], latest = id, candidate.updated
				conflict = false
			} else if candidate.updated.Equal(latest) {
				conflict = true
			}
		}
		if conflict {
			selected[key] = ""
		}
	}
	for i := range records {
		r := &records[i]
		p := r.Values.Performance
		if p == nil || p.Kind != "concert" || p.SetlistID == "" || p.Legacy {
			continue
		}
		id := p.Revision
		if id == "" {
			id = p.RevisionHash
		}
		if active := selected[groupKey(*r)]; active == "" {
			r.Values.Reason = "conflicting setlist revisions"
		} else if id != active {
			r.Values.Reason = "superseded setlist revision"
		}
	}
}

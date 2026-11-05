package sources

import (
	"context"
	"encoding/json"

	"github.com/gabehf/koito/internal/filters"
	"github.com/gabehf/koito/internal/music"
)

func membershipKey(level string, v recordValues) string {
	parts := []string{v.Artist, v.Album}
	if level == "track" {
		parts = []string{v.Artist, v.Title}
	}
	if parts[0] == "" || parts[1] == "" {
		return ""
	}
	raw, _ := json.Marshal(parts)
	return level + ":" + string(raw)
}

func attachRecordMemberships(ctx context.Context, reader sqlReader, records []matchingRecord) error {
	items, err := currentSnapshotRecords(ctx, reader)
	if err != nil {
		return err
	}
	byID := map[int64]int{}
	for i, r := range records {
		byID[r.ID] = i
	}
	// Inspection can request a subset of history. Read the remaining current
	// membership evidence in one query rather than borrowing stale view flags.
	missing := map[int64]matchingRecord{}
	if len(items) > 0 {
		rows, e := reader.QueryContext(ctx, `SELECT r.id,r.parser,r.payload FROM source_records r JOIN source_snapshot_items i ON i.record_id=r.id JOIN source_snapshots s ON s.id=i.snapshot_id WHERE `+currentSnapshotCondition)
		if e != nil {
			return e
		}
		for rows.Next() {
			var r matchingRecord
			if e = rows.Scan(&r.ID, &r.Parser, &r.Payload); e != nil {
				rows.Close()
				return e
			}
			if _, ok := byID[r.ID]; !ok {
				r.Values = normalizeRecord([]byte(r.Payload), r.Parser, "")
				r.Payload = ""
				missing[r.ID] = r
			}
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
	}
	type entry struct {
		item snapshotItem
		uri  string
	}
	index := map[string][]entry{}
	for _, item := range items {
		i, ok := byID[item.RecordID]
		r := missing[item.RecordID]
		if ok {
			r = records[i]
		}
		level := "album"
		if item.Kind == "playlist" || item.Kind == "saved_tracks" {
			level = "track"
		} else if item.Kind != "saved_albums" && item.Kind != "owned" && item.Kind != "wanted" {
			continue
		}
		if key := membershipKey(level, r.Values); key != "" {
			index[key] = append(index[key], entry{item, r.Values.URI})
		}
		if level == "track" && r.Values.URI != "" {
			index[r.Values.URI] = append(index[r.Values.URI], entry{item, r.Values.URI})
		}
	}
	// Membership is current item evidence. Identical item identities can share
	// this immutable environment across their individual history records.
	environments := map[string]filters.Env{}
	for i := range records {
		r := &records[i]
		identity := membershipKey("track", r.Values) + "\x00" + membershipKey("album", r.Values) + "\x00" + r.Values.URI
		if env, ok := environments[identity]; ok {
			r.LibraryFields = env
			continue
		}
		env := music.EmptyLibraryEnvironment()
		r.LibraryFields = env
		environments[identity] = env
		seen := map[int64]bool{}
		playlists := map[string]bool{}
		names := []filters.Value{}
		ids := []filters.Value{}
		for _, key := range []string{membershipKey("album", r.Values), membershipKey("track", r.Values), r.Values.URI} {
			for _, e := range index[key] {
				if seen[e.item.RecordID] || r.Values.URI != "" && e.uri != "" && r.Values.URI != e.uri {
					continue
				}
				seen[e.item.RecordID] = true
				ids = append(ids, filters.Integer(e.item.RecordID))
				switch e.item.Kind {
				case "saved_tracks", "saved_albums":
					env["music.saved"] = filters.Bool(true)
				case "owned":
					env["music.owned"] = filters.Bool(true)
				case "wanted":
					env["music.wanted"] = filters.Bool(true)
				case "playlist":
					if !playlists[e.item.Name] {
						playlists[e.item.Name] = true
						names = append(names, filters.String(e.item.Name))
					}
				}
			}
		}
		env["music.playlists"] = filters.Values(names...)
		env["music.membership_records"] = filters.Values(ids...)
	}
	return nil
}

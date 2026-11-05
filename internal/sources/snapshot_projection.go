package sources

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
)

const currentSnapshotCondition = `s.id=(SELECT MAX(n.id) FROM source_snapshots n WHERE n.source_id=s.source_id AND n.set_key=s.set_key)
 AND (s.group_key='' OR s.file_hash=(SELECT n.file_hash FROM source_snapshots n WHERE n.source_id=s.source_id AND n.group_key=s.group_key ORDER BY n.id DESC LIMIT 1))`

const currentSnapshotItems = `SELECT i.record_id,s.id,s.kind,s.name,i.position FROM source_snapshots s
 JOIN source_snapshot_items i ON i.snapshot_id=s.id
 WHERE ` + currentSnapshotCondition + `
 ORDER BY s.id,i.position`

type snapshotItem struct {
	RecordID, SnapshotID int64
	Kind, Name           string
	Position             int
}

func currentSnapshotRecords(ctx context.Context, reader sqlReader) ([]snapshotItem, error) {
	rows, err := reader.QueryContext(ctx, currentSnapshotItems)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []snapshotItem{}
	for rows.Next() {
		var item snapshotItem
		if err = rows.Scan(&item.RecordID, &item.SnapshotID, &item.Kind, &item.Name, &item.Position); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func snapshotExternalID(r matchingRecord) string {
	if r.Parser == DiscogsCSV {
		return csvFields([]byte(r.Payload))["release_id"]
	}
	if strings.HasPrefix(r.Parser, SpotifyCSV) {
		f := csvFields([]byte(r.Payload))
		if f["Track URI"] != "" {
			return f["Track URI"]
		}
		return f["Album URI"]
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal([]byte(r.Payload), &fields)
	if id := rawString(fields["uri"]); id != "" {
		return id
	}
	var track map[string]json.RawMessage
	_ = json.Unmarshal(fields["track"], &track)
	return rawString(track["trackUri"])
}

func projectSnapshotRelease(ctx context.Context, tx *sql.Tx, r matchingRecord) (int64, error) {
	v := r.Values
	if v.Artist == "" || v.Album == "" {
		return 0, nil
	}
	var id int64
	if r.ExternalID != "" {
		err := tx.QueryRowContext(ctx, `SELECT release_id FROM source_release_ids WHERE service=? AND external_id=?`, r.Service, r.ExternalID).Scan(&id)
		if err == nil {
			return id, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return 0, err
		}
	}
	var artist int64
	err := tx.QueryRowContext(ctx, `SELECT artist_id FROM artist_aliases WHERE alias=? ORDER BY is_primary DESC,artist_id LIMIT 1`, v.Artist).Scan(&artist)
	if errors.Is(err, sql.ErrNoRows) {
		artist, err = insertID(ctx, tx, `INSERT INTO artists DEFAULT VALUES`)
		if err != nil {
			return 0, err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO artist_aliases(artist_id,alias,source,is_primary) VALUES(?,?,'sources',1)`, artist, v.Artist)
	}
	if err != nil {
		return 0, err
	}
	err = tx.QueryRowContext(ctx, `SELECT a.release_id FROM release_aliases a JOIN artist_releases ar ON ar.release_id=a.release_id WHERE a.alias=? AND ar.artist_id=? AND NOT EXISTS(SELECT 1 FROM source_release_ids e WHERE e.release_id=a.release_id AND e.service=? AND e.external_id<>?) ORDER BY a.release_id LIMIT 1`, v.Album, artist, r.Service, r.ExternalID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		id, err = insertID(ctx, tx, `INSERT INTO releases DEFAULT VALUES`)
		if err != nil {
			return 0, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO release_aliases(release_id,alias,source,is_primary) VALUES(?,?,'sources',1)`, id, v.Album); err != nil {
			return 0, err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO artist_releases(artist_id,release_id,is_primary) VALUES(?,?,1)`, artist, id)
	}
	if err != nil {
		return 0, err
	}
	if r.ExternalID != "" {
		_, err = tx.ExecContext(ctx, `INSERT INTO source_release_ids(service,external_id,release_id) VALUES(?,?,?) ON CONFLICT DO NOTHING`, r.Service, r.ExternalID, id)
	}
	return id, err
}

func publishSnapshots(ctx context.Context, tx *sql.Tx, records map[int64]*matchingRecord, naming []namingProgram, historyAssignments map[int64]string) error {
	items, err := currentSnapshotRecords(ctx, tx)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM source_music_states`); err != nil {
		return err
	}
	for _, item := range items {
		r := records[item.RecordID]
		if r == nil {
			continue
		}
		var track, release any
		if r.Values.Title != "" && r.Values.Artist != "" && (item.Kind == "playlist" || item.Kind == "saved_tracks") {
			v, e := applyNaming(ctx, naming, *r, spotifyItem{Title: r.Values.Title, Artist: r.Values.Artist, Album: r.Values.Album, URI: r.Values.URI, RecordingID: r.Values.RecordingID})
			if e != nil {
				return e
			}
			id, e := projectTrack(ctx, tx, v)
			if e != nil {
				return e
			}
			id, e = historyVariant(ctx, tx, id, historyComparisonKey(*r), historyAssignments)
			if e != nil {
				return e
			}
			track = id
			var album int64
			if e = tx.QueryRowContext(ctx, `SELECT release_id FROM tracks WHERE id=?`, id).Scan(&album); e != nil {
				return e
			}
			release = album
		} else if item.Kind == "saved_albums" || item.Kind == "owned" || item.Kind == "wanted" || item.Kind == "inventory" || item.Kind == "contributions" {
			id, e := projectSnapshotRelease(ctx, tx, *r)
			if e != nil {
				return e
			}
			if id > 0 {
				release = id
			}
		}
		if track == nil && release == nil {
			continue
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO source_music_states(record_id,snapshot_id,track_id,release_id,external_id,kind,name,position) VALUES(?,?,?,?,?,?,?,?)`, r.ID, item.SnapshotID, track, release, r.ExternalID, item.Kind, item.Name, item.Position)
		if err != nil {
			return err
		}
	}
	return nil
}

type MusicState struct {
	Kind     string  `json:"kind"`
	Name     string  `json:"name"`
	Records  []int64 `json:"records"`
	Quantity int     `json:"quantity"`
}

func (s *Store) MusicStates(ctx context.Context, track, album int64) ([]MusicState, error) {
	query := `SELECT m.kind,m.name,m.record_id FROM source_music_states m WHERE `
	args := []any{}
	if track > 0 {
		query += `m.track_id IN (SELECT t.track_id FROM track_histories t JOIN track_histories h ON h.music_track_id=t.music_track_id WHERE h.track_id=?) OR m.track_id=? OR (m.track_id IS NULL AND m.release_id IN (SELECT release_id FROM tracks WHERE id=?))`
		args = []any{track, track, track}
	} else {
		query += `m.release_id=?`
		args = []any{album}
	}
	query += ` ORDER BY m.kind,m.name,m.record_id`
	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	states := []MusicState{}
	indices := map[string]int{}
	for rows.Next() {
		var kind, name string
		var id int64
		if err = rows.Scan(&kind, &name, &id); err != nil {
			return nil, err
		}
		key := kind + "\x00" + name
		index, ok := indices[key]
		if !ok {
			index = len(states)
			indices[key] = index
			states = append(states, MusicState{Kind: kind, Name: name, Records: []int64{}})
		}
		states[index].Records = append(states[index].Records, id)
		states[index].Quantity++
	}
	return states, rows.Err()
}

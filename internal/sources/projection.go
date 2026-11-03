package sources

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type ProjectionResult struct {
	Policy     string `json:"policy"`
	Records    int    `json:"records"`
	Plays      int    `json:"plays"`
	Collisions int    `json:"collisions"`
}
type spotifyItem struct {
	Timestamp string `json:"ts"`
	Title     string `json:"master_metadata_track_name"`
	Artist    string `json:"master_metadata_album_artist_name"`
	Album     string `json:"master_metadata_album_album_name"`
	URI       string `json:"spotify_track_uri"`
	ReasonEnd string `json:"reason_end"`
	Played    int64  `json:"ms_played"`
}
type projectionRecord struct {
	id, source int64
	user       int32
	payload    string
}

// Rebuild publishes a complete interpretation and its display rows in one SQLite
// transaction. Raw evidence and unrelated catalog/listening rows are untouched.
func (s *Store) Rebuild(ctx context.Context) (result ProjectionResult, err error) {
	writeMu.Lock()
	defer writeMu.Unlock()
	result.Policy = Policy
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT r.id,r.source_id,a.user_id,r.payload FROM source_records r JOIN source_accounts a ON a.id=r.source_id WHERE r.parser=? ORDER BY r.id`, Parser)
	if err != nil {
		return result, err
	}
	records := []projectionRecord{}
	for rows.Next() {
		var r projectionRecord
		if err = rows.Scan(&r.id, &r.source, &r.user, &r.payload); err != nil {
			rows.Close()
			return result, err
		}
		records = append(records, r)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return result, err
	}
	rows.Close()
	for _, q := range []string{
		`DELETE FROM listens WHERE EXISTS(SELECT 1 FROM source_projection p WHERE p.owned=1 AND p.track_id=listens.track_id AND p.listened_at=listens.listened_at AND p.user_id=listens.user_id AND listens.client='sources:spotify-v1')`,
		`DELETE FROM source_projection`,
		`DELETE FROM source_plays WHERE policy='spotify-v1'`,
		`DELETE FROM source_interpretations WHERE policy='spotify-v1'`,
	} {
		if _, err = tx.ExecContext(ctx, q); err != nil {
			return result, err
		}
	}
	seen := map[string]bool{}
	trackCache := map[string]int64{}
	for _, r := range records {
		if err = ctx.Err(); err != nil {
			return result, err
		}
		result.Records++
		var v spotifyItem
		reason, key := "counted", ""
		if e := json.Unmarshal([]byte(r.payload), &v); e != nil {
			reason = "invalid fields"
		}
		timestamp, e := time.Parse(time.RFC3339Nano, v.Timestamp)
		switch {
		case reason != "counted":
		case e != nil:
			reason = "invalid timestamp"
		case v.Played < 0:
			reason = "invalid played duration"
		case strings.TrimSpace(v.Title) == "" || strings.TrimSpace(v.Artist) == "":
			reason = "not an identified music track"
		case v.ReasonEnd != "trackdone":
			reason = "not reported complete"
		}
		if reason == "counted" {
			identity := []string{v.Title, v.Artist, v.Album}
			if v.URI != "" {
				identity = []string{v.URI}
			}
			b, _ := json.Marshal([]any{r.source, v.Timestamp, identity})
			key = digest(b)
			if seen[key] {
				reason = "matched existing play"
			} else {
				seen[key] = true
				result.Plays++
				music, _ := json.Marshal([]string{v.Artist, v.Album, v.Title})
				cacheKey := string(music)
				track, ok := trackCache[cacheKey]
				if !ok {
					track, err = projectTrack(ctx, tx, v)
					if err != nil {
						return result, err
					}
					trackCache[cacheKey] = track
				}
				unix := timestamp.Unix()
				insert, e := tx.ExecContext(ctx, `INSERT INTO listens(track_id,listened_at,user_id,client) VALUES(?,?,?,'sources:spotify-v1') ON CONFLICT(track_id,listened_at) DO NOTHING`, track, unix, r.user)
				if e != nil {
					return result, e
				}
				added, e := insert.RowsAffected()
				if e != nil {
					return result, e
				}
				disposition := "counted"
				if added == 0 {
					result.Collisions++
					disposition = "display key already exists"
					reason = disposition
				}
				_, err = tx.ExecContext(ctx, `INSERT INTO source_projection(track_id,listened_at,user_id,policy,owned) VALUES(?,?,?,?,?) ON CONFLICT(track_id,listened_at) DO NOTHING`, track, unix, r.user, Policy, added)
				if err != nil {
					return result, err
				}
				_, err = tx.ExecContext(ctx, `INSERT INTO source_plays(policy,play_key,source_id,representative_record,track_id,listened_at,disposition) VALUES(?,?,?,?,?,?,?)`, Policy, key, r.source, r.id, track, unix, disposition)
				if err != nil {
					return result, err
				}
			}
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO source_interpretations(record_id,policy,play_key,reason) VALUES(?,?,?,?)`, r.id, Policy, key, reason)
		if err != nil {
			return result, err
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO source_projection_runs(policy,created_at,records,plays,collisions) VALUES(?,?,?,?,?)`, Policy, time.Now().Unix(), result.Records, result.Plays, result.Collisions)
	if err != nil {
		return result, err
	}
	err = tx.Commit()
	return result, err
}

func insertID(ctx context.Context, tx *sql.Tx, query string, args ...any) (int64, error) {
	res, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// Exact alias association only: no remote metadata lookup or destructive alias
// normalization. This compatibility catalog is separate from original fields.
func projectTrack(ctx context.Context, tx *sql.Tx, v spotifyItem) (int64, error) {
	var artist, album, track int64
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
	title := v.Album
	if strings.TrimSpace(title) == "" {
		title = "Unknown Album"
	}
	err = tx.QueryRowContext(ctx, `SELECT r.release_id FROM release_aliases r JOIN artist_releases a ON a.release_id=r.release_id WHERE r.alias=? AND a.artist_id=? ORDER BY r.release_id LIMIT 1`, title, artist).Scan(&album)
	if errors.Is(err, sql.ErrNoRows) {
		album, err = insertID(ctx, tx, `INSERT INTO releases DEFAULT VALUES`)
		if err != nil {
			return 0, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO release_aliases(release_id,alias,source,is_primary) VALUES(?,?,'sources',1)`, album, title); err != nil {
			return 0, err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO artist_releases(artist_id,release_id,is_primary) VALUES(?,?,1)`, artist, album)
	}
	if err != nil {
		return 0, err
	}
	err = tx.QueryRowContext(ctx, `SELECT t.id FROM tracks t JOIN track_aliases a ON a.track_id=t.id JOIN artist_tracks ar ON ar.track_id=t.id WHERE t.release_id=? AND a.alias=? AND ar.artist_id=? ORDER BY t.id LIMIT 1`, album, v.Title, artist).Scan(&track)
	if errors.Is(err, sql.ErrNoRows) {
		track, err = insertID(ctx, tx, `INSERT INTO tracks(release_id) VALUES(?)`, album)
		if err != nil {
			return 0, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO track_aliases(track_id,alias,source,is_primary) VALUES(?,?,'sources',1)`, track, v.Title); err != nil {
			return 0, err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO artist_tracks(artist_id,track_id,is_primary) VALUES(?,?,1)`, artist, track)
	}
	if err != nil {
		return 0, fmt.Errorf("project track: %w", err)
	}
	return track, nil
}

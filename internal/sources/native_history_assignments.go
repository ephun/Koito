package sources

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"github.com/gabehf/koito/internal/filters"
	"github.com/gabehf/koito/internal/music"
)

// Reserve the actual native comparison, so identical imported evidence can
// report a legacy overlap while source-specific comparisons stay separate.
func nativeHistoryAssignments(ctx context.Context, tx *sql.Tx, config RuleConfig, legacy map[[3]int64]bool) (map[int64]string, error) {
	result := map[int64]string{}
	if len(legacy) == 0 {
		return result, nil
	}
	ids := map[int64]bool{}
	for key := range legacy {
		ids[key[0]] = true
	}
	programs, err := compileMusicPrograms(config)
	if err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT t.id,o.original_track_id,t.title,r.title,COALESCE((SELECT json_group_array(name) FROM(SELECT a.name FROM artist_tracks ar JOIN artists_with_name a ON a.id=ar.artist_id WHERE ar.track_id=t.id ORDER BY ar.artist_id)),'[]') FROM tracks_with_title t JOIN releases_with_title r ON r.id=t.release_id JOIN catalog_track_origins o ON o.track_id=t.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, origin int64
		var title, album, rawNames string
		if err = rows.Scan(&id, &origin, &title, &album, &rawNames); err != nil {
			return nil, err
		}
		if !ids[id] {
			continue
		}
		var names []string
		if err = json.Unmarshal([]byte(rawNames), &names); err != nil {
			return nil, err
		}
		artist := strings.Join(names, " & ")
		if album == "Unknown Album" {
			album = ""
		}
		env := filters.Env{"catalog.kind": filters.String("track"), "catalog.id": filters.Integer(origin), "source.service": filters.String("koito"), "original.artist": filters.String(artist), "original.album": filters.String(album), "original.title": filters.String(title), "original.identifier": filters.String("")}
		value, _, err := programs.Resolve(ctx, env, music.MatchIdentity{Artist: artist, Album: album, Title: title})
		if err != nil {
			return nil, err
		}
		key, _ := json.Marshal([]string{"matching", value.Artist, value.Title, value.Album})
		result[id] = string(key)
	}
	return result, rows.Err()
}

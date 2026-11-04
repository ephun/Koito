// Package music maintains derived catalog identities independently of album
// appearances, source records, and individual listening events.
package music

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"github.com/gabehf/koito/internal/filters"
)

type Database interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func CaseSensitive(ctx context.Context, db Database) (bool, error) {
	var sensitive bool
	err := db.QueryRowContext(ctx, `SELECT COALESCE((SELECT
	    CASE WHEN json_extract(j.value,'$.enabled')=1 THEN json_extract(j.value,'$.settings.case_sensitive') ELSE 1 END
	    FROM filter_versions v JOIN filter_state s ON s.version_id=v.id,json_each(v.config_json,'$.rules') j
	    WHERE s.id=1 AND json_extract(j.value,'$.type')='matching' AND json_extract(j.value,'$.subtype')='music' LIMIT 1),1)`).Scan(&sensitive)
	return sensitive, err
}

func fold(value string) string {
	return strings.Map(func(r rune) rune {
		minimum := r
		for next := unicode.SimpleFold(r); next != r; next = unicode.SimpleFold(next) {
			if next < minimum {
				minimum = next
			}
		}
		return minimum
	}, value)
}

// RefreshHistories assigns the name-based track history separately from
// recording identity. Matching names can share a page while their recordings
// and album appearances remain independently addressable.
func RefreshHistories(ctx context.Context, db Database, caseSensitive bool, config ...string) error {
	return histories(ctx, db, caseSensitive, 0, config...)
}

func AssignHistory(ctx context.Context, db Database, trackID int64, caseSensitive bool) error {
	return histories(ctx, db, caseSensitive, trackID)
}

func histories(ctx context.Context, db Database, caseSensitive bool, trackID int64, config ...string) error {
	var raw string
	if len(config) > 0 {
		raw = config[0]
	} else {
		err := db.QueryRowContext(ctx, `SELECT v.config_json FROM filter_versions v JOIN filter_state s ON s.version_id=v.id WHERE s.id=1`).Scan(&raw)
		if err != nil {
			return err
		}
	}
	var settings struct {
		Rules []IdentityFilter `json:"rules"`
	}
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return err
	}
	programs, err := CompileIdentityPrograms(settings.Rules)
	if err != nil {
		return err
	}
	query := `SELECT t.id,t.title,COALESCE((SELECT group_concat(artist_id,',') FROM
	    (SELECT artist_id FROM artist_tracks WHERE track_id=t.id ORDER BY artist_id)),'')
	    ,COALESCE((SELECT json_group_array(name) FROM (SELECT a.name FROM artist_tracks ar JOIN artists_with_name a ON a.id=ar.artist_id WHERE ar.track_id=t.id ORDER BY ar.artist_id)),'[]'),COALESCE(r.title,''),COALESCE(o.comparison_key,'')
	    FROM tracks_with_title t LEFT JOIN releases_with_title r ON r.id=t.release_id LEFT JOIN track_history_overrides o ON o.track_id=t.id`
	var args []any
	if trackID != 0 {
		query += ` WHERE t.id=?`
		args = append(args, trackID)
	}
	query += ` ORDER BY t.id`
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	type item struct {
		id                     int64
		title, artists         string
		names, album, override string
	}
	var items []item
	for rows.Next() {
		var v item
		if err = rows.Scan(&v.id, &v.title, &v.artists, &v.names, &v.album, &v.override); err != nil {
			rows.Close()
			return err
		}
		items = append(items, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, v := range items {
		title := v.title
		if !caseSensitive {
			title = fold(title)
		}
		key, _ := json.Marshal([]string{v.artists, title})
		if programs.Active() {
			var names []string
			if err := json.Unmarshal([]byte(v.names), &names); err != nil {
				return err
			}
			artist := strings.Join(names, " & ")
			album := v.album
			if album == "Unknown Album" {
				album = ""
			}
			env := filters.Env{"original.artist": filters.String(artist), "original.album": filters.String(album), "original.title": filters.String(v.title), "source.service": filters.String("koito")}
			value, _, err := programs.Resolve(ctx, env, MatchIdentity{Artist: artist, Album: album, Title: v.title})
			if err != nil {
				return err
			}
			key, _ = json.Marshal([]string{"matching", value.Artist, value.Title})
			if strings.Contains(v.artists, ",") {
				key, _ = json.Marshal([]string{"matching-multi", v.artists, value.Artist, value.Title})
			}
		}
		if v.override != "" {
			key = []byte(v.override)
		}
		if _, err = db.ExecContext(ctx, `INSERT INTO music_tracks(identity_key) VALUES(?) ON CONFLICT DO NOTHING`, string(key)); err != nil {
			return err
		}
		if _, err = db.ExecContext(ctx, `INSERT INTO track_histories(track_id,music_track_id)
		    SELECT ?,id FROM music_tracks WHERE identity_key=?
		    ON CONFLICT(track_id) DO UPDATE SET music_track_id=excluded.music_track_id`, v.id, string(key)); err != nil {
			return err
		}
	}
	return nil
}

func RecordingKey(trackID int64, mbid, uri string) string {
	if mbid != "" {
		return "mbid:" + mbid
	}
	if uri != "" {
		return "spotify:" + uri
	}
	return fmt.Sprintf("appearance:%d", trackID)
}

func AssignRecording(ctx context.Context, db Database, trackID int64, mbid, uri string) error {
	key := RecordingKey(trackID, mbid, uri)
	_, err := db.ExecContext(ctx, `INSERT INTO recordings(recording_key,musicbrainz_id,spotify_uri) VALUES(?,NULLIF(?,''),NULLIF(?,''))
	    ON CONFLICT(recording_key) DO NOTHING`, key, mbid, uri)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `INSERT INTO track_recordings(track_id,recording_key) VALUES(?,?)
	    ON CONFLICT(track_id) DO UPDATE SET recording_key=excluded.recording_key`, trackID, key)
	return err
}

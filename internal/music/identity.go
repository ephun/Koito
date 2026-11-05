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
	var previous int64
	if trackID != 0 {
		err := db.QueryRowContext(ctx, `SELECT music_track_id FROM track_histories WHERE track_id=?`, trackID).Scan(&previous)
		if err != nil && err != sql.ErrNoRows {
			return err
		}
	}
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
	for i := range settings.Rules {
		r := &settings.Rules[i]
		if r.Type == "matching" && r.Subtype == "music" && r.Program == nil {
			legacy := IdentitySettings{}
			if r.Settings != nil {
				legacy = *r.Settings
			}
			legacy.CaseSensitive = caseSensitive
			r.Settings = &legacy
		}
	}
	programs, err := CompileIdentityPrograms(settings.Rules)
	if err != nil {
		return err
	}
	query := `SELECT t.id,t.title,COALESCE((SELECT group_concat(artist_id,',') FROM
	    (SELECT artist_id FROM artist_tracks WHERE track_id=t.id ORDER BY artist_id)),'')
	    ,COALESCE((SELECT json_group_array(name) FROM (SELECT a.name FROM artist_tracks ar JOIN artists_with_name a ON a.id=ar.artist_id WHERE ar.track_id=t.id ORDER BY ar.artist_id)),'[]'),COALESCE(r.title,''),COALESCE(o.comparison_key,'')
	    ,c.artist,COALESCE((SELECT group_concat(artist_id,',') FROM(SELECT DISTINCT COALESCE(mt.artist_id,ma.matched_artist_id,a.artist_id) AS artist_id FROM artist_tracks a LEFT JOIN music_track_artists mt ON mt.track_id=a.track_id AND mt.original_artist_id=a.artist_id LEFT JOIN music_artist_matches ma ON ma.artist_id=a.artist_id WHERE a.track_id=t.id ORDER BY artist_id)),'')
	    FROM tracks_with_title t LEFT JOIN releases_with_title r ON r.id=t.release_id LEFT JOIN track_history_overrides o ON o.track_id=t.id LEFT JOIN music_album_comparisons c ON c.track_id=t.id`
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
		comparisonArtist       *string
		matchedArtists         string
	}
	var items []item
	for rows.Next() {
		var v item
		if err = rows.Scan(&v.id, &v.title, &v.artists, &v.names, &v.album, &v.override, &v.comparisonArtist, &v.matchedArtists); err != nil {
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
			origin, e := TrackOrigin(ctx, db, v.id)
			if e != nil {
				return e
			}
			env := comparisonEnvironment("track", origin, artist, album, v.title, "")
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
			var comparison []string
			if err := json.Unmarshal(key, &comparison); err != nil {
				return err
			}
			if len(comparison) == 4 && comparison[0] == "matching" {
				key, _ = json.Marshal(comparison[:3])
			}
		}
		if programs.Active() {
			var comparison []string
			if err := json.Unmarshal(key, &comparison); err != nil {
				return err
			}
			if comparison[0] == "matching" && v.comparisonArtist != nil {
				comparison[1] = *v.comparisonArtist
			}
			if comparison[0] == "matching-multi" {
				comparison[1] = v.matchedArtists
			}
			comparison = append(comparison, "artists", v.matchedArtists)
			key, _ = json.Marshal(comparison)
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
	if trackID == 0 {
		return refreshAllHistoryCanonicals(ctx, db)
	}
	if previous != 0 {
		if err := refreshHistoryCanonical(ctx, db, previous); err != nil {
			return err
		}
	}
	return RefreshHistoryCanonicalForTrack(ctx, db, trackID)
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

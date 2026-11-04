package sources

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/gabehf/koito/internal/music"
)

type ProjectionResult struct {
	Policy     string `json:"policy"`
	Records    int    `json:"records"`
	Plays      int    `json:"plays"`
	Collisions int    `json:"collisions"`
	Estimated  int64  `json:"estimated"`
	LastPlayed int64  `json:"last_played"`
	Undated    int64  `json:"undated"`
}
type spotifyItem struct {
	Timestamp   string `json:"ts"`
	Title       string `json:"master_metadata_track_name"`
	Artist      string `json:"master_metadata_album_artist_name"`
	Album       string `json:"master_metadata_album_album_name"`
	URI         string `json:"spotify_track_uri"`
	RecordingID string `json:"recording_id,omitempty"`
	ReasonEnd   string `json:"reason_end"`
	Played      int64  `json:"ms_played"`
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
	// A shared name supplies a history, not proof of a shared performance.
	// Known conflicting recording identities retain separate appearances.
	identity := ""
	if v.RecordingID != "" || v.URI != "" {
		identity = music.RecordingKey(0, v.RecordingID, v.URI)
	}
	err = tx.QueryRowContext(ctx, `SELECT t.id FROM tracks t JOIN track_aliases a ON a.track_id=t.id JOIN artist_tracks ar ON ar.track_id=t.id
	    LEFT JOIN track_recordings tr ON tr.track_id=t.id
	    WHERE t.release_id=? AND a.alias=? AND ar.artist_id=? AND
	    ((?='' AND (tr.recording_key IS NULL OR tr.recording_key LIKE 'appearance:%')) OR tr.recording_key=?)
	    ORDER BY t.id LIMIT 1`, album, v.Title, artist, identity, identity).Scan(&track)
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
	if err = music.AssignRecording(ctx, tx, track, v.RecordingID, v.URI); err != nil {
		return 0, err
	}
	return track, nil
}

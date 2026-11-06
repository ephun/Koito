package music

import (
	"context"
	"database/sql"
)

// A native listen needs its own comparison context when an imported appearance
// has source-specific values. Its original release and recording stay linked.
func NativeAppearance(ctx context.Context, db Database, track int64) (int64, bool, error) {
	var imported bool
	if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM track_catalog_comparisons WHERE track_id=?)`, track).Scan(&imported); err != nil {
		return 0, false, err
	}
	if !imported {
		return track, false, nil
	}
	var native int64
	err := db.QueryRowContext(ctx, `SELECT track_id FROM native_track_variants WHERE original_track_id=?`, track).Scan(&native)
	if err == nil {
		return native, false, nil
	}
	if err != sql.ErrNoRows {
		return 0, false, err
	}
	result, err := db.ExecContext(ctx, `INSERT INTO tracks(release_id,duration) SELECT release_id,duration FROM tracks WHERE id=?`, track)
	if err != nil {
		return 0, false, err
	}
	native, err = result.LastInsertId()
	if err != nil {
		return 0, false, err
	}
	for _, q := range []string{
		`INSERT INTO track_aliases(track_id,alias,source,is_primary) SELECT ?,alias,source,is_primary FROM track_aliases WHERE track_id=?`,
		`INSERT INTO artist_tracks(track_id,artist_id,is_primary) SELECT ?,artist_id,is_primary FROM artist_tracks WHERE track_id=?`,
		`INSERT INTO track_recordings(track_id,recording_key) SELECT ?,recording_key FROM track_recordings WHERE track_id=?`,
	} {
		if _, err = db.ExecContext(ctx, q, native, track); err != nil {
			return 0, false, err
		}
	}
	_, err = db.ExecContext(ctx, `INSERT INTO native_track_variants(original_track_id,track_id) VALUES(?,?)`, track, native)
	return native, true, err
}

func TrackOrigin(ctx context.Context, db Database, id int64) (int64, error) {
	var origin int64
	err := db.QueryRowContext(ctx, `SELECT original_track_id FROM catalog_track_origins WHERE track_id=?`, id).Scan(&origin)
	return origin, err
}

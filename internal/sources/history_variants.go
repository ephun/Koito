package sources

import (
	"context"
	"database/sql"
	"fmt"
)

// One displayed album appearance can need different comparison identities when
// source-scoped filters disagree. Keep each group's view links separate without
// changing the original appearance, aliases, artists, or recordings.
func historyVariant(ctx context.Context, tx *sql.Tx, base int64, key string, assigned map[int64]string) (int64, error) {
	if key == "" {
		return base, nil
	}
	var selected int64
	err := tx.QueryRowContext(ctx, `SELECT track_id FROM source_history_variants WHERE base_track_id=? AND comparison_key=?`, base, key).Scan(&selected)
	if err != nil && err != sql.ErrNoRows {
		return 0, err
	}
	if err == sql.ErrNoRows {
		selected = base
	}
	if previous, exists := assigned[selected]; exists && previous != key {
		result, err := tx.ExecContext(ctx, `INSERT INTO tracks(release_id,duration) SELECT release_id,duration FROM tracks WHERE id=?`, base)
		if err != nil {
			return 0, err
		}
		selected, err = result.LastInsertId()
		if err != nil {
			return 0, err
		}
		for _, query := range []string{
			`INSERT INTO track_aliases(track_id,alias,source,is_primary) SELECT ?,alias,source,is_primary FROM track_aliases WHERE track_id=?`,
			`INSERT INTO artist_tracks(track_id,artist_id,is_primary) SELECT ?,artist_id,is_primary FROM artist_tracks WHERE track_id=?`,
			`INSERT INTO track_recordings(track_id,recording_key) SELECT ?,recording_key FROM track_recordings WHERE track_id=?`,
		} {
			if _, err = tx.ExecContext(ctx, query, selected, base); err != nil {
				return 0, fmt.Errorf("Preserve history appearance: %w", err)
			}
		}
	}
	assigned[selected] = key
	if _, err = tx.ExecContext(ctx, `INSERT INTO source_history_variants(base_track_id,comparison_key,track_id) VALUES(?,?,?) ON CONFLICT(base_track_id,comparison_key) DO UPDATE SET track_id=excluded.track_id`, base, key, selected); err != nil {
		return 0, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO track_history_overrides(track_id,comparison_key) VALUES(?,?) ON CONFLICT(track_id) DO UPDATE SET comparison_key=excluded.comparison_key`, selected, key)
	return selected, err
}

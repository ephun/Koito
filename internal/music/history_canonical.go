package music

import (
	"context"
	"database/sql"
)

const historyCanonicalOrder = ` ORDER BY EXISTS(SELECT 1 FROM catalog_names n WHERE n.kind='track' AND n.item_id=m.track_id) DESC,
 (EXISTS(SELECT 1 FROM listens l WHERE l.track_id=m.track_id) OR EXISTS(SELECT 1 FROM source_aggregates a WHERE a.track_id=m.track_id AND a.undated_count>0)) DESC,m.track_id LIMIT 1`

func refreshHistoryCanonical(ctx context.Context, db Database, history int64) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM music_history_canonicals WHERE music_track_id=?`, history); err != nil {
		return err
	}
	_, err := db.ExecContext(ctx, `INSERT INTO music_history_canonicals(music_track_id,track_id) SELECT ?,m.track_id FROM track_histories m WHERE m.music_track_id=?`+historyCanonicalOrder, history, history)
	return err
}

// Cache the selected appearance so statistics do not repeat its selection for
// every play. Explicit names take precedence, followed by contributing tracks.
func RefreshHistoryCanonicalForTrack(ctx context.Context, db Database, track int64) error {
	var history int64
	err := db.QueryRowContext(ctx, `SELECT music_track_id FROM track_histories WHERE track_id=?`, track).Scan(&history)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	return refreshHistoryCanonical(ctx, db, history)
}

func refreshAllHistoryCanonicals(ctx context.Context, db Database) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM music_history_canonicals`); err != nil {
		return err
	}
	_, err := db.ExecContext(ctx, `INSERT INTO music_history_canonicals(music_track_id,track_id)
 SELECT h.music_track_id,(SELECT m.track_id FROM track_histories m WHERE m.music_track_id=h.music_track_id`+historyCanonicalOrder+`) FROM track_histories h GROUP BY h.music_track_id`)
	return err
}

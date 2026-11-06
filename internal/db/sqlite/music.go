package sqlite

import (
	"context"
	"database/sql"

	"github.com/gabehf/koito/internal/models"
)

// IDs in existing URLs continue to address album appearances. This predicate
// expands them into the shared track history while preserving every listen.
func historyPredicate(column string) string {
	return column + ` IN (SELECT track_id FROM track_history_members WHERE canonical_track_id=
	    (SELECT canonical_track_id FROM track_history_members WHERE track_id=?))`
}

func (s *Sqlite) trackHistory(ctx context.Context, t *models.Track) error {
	err := s.db.QueryRowContext(ctx, `SELECT music_track_id FROM track_histories WHERE track_id=?`, t.ID).Scan(&t.MusicTrackID)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT a.id,COALESCE((SELECT display_name FROM catalog_names cn WHERE cn.kind='album' AND cn.item_id=a.id),a.title),COALESCE(SUM(c.amount),0)
	    FROM tracks p JOIN releases_with_title a ON a.id=p.release_id
	    LEFT JOIN listening_counts c ON c.track_id=p.id
	    WHERE `+historyPredicate("p.id")+` GROUP BY a.id,a.title ORDER BY SUM(c.amount) DESC,a.title,a.id`, t.ID)
	if err != nil {
		return err
	}
	t.Albums = []models.TrackAlbum{}
	for rows.Next() {
		var a models.TrackAlbum
		if err = rows.Scan(&a.ID, &a.Title, &a.ListenCount); err != nil {
			rows.Close()
			return err
		}
		t.Albums = append(t.Albums, a)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	rows, err = s.db.QueryContext(ctx, `SELECT DISTINCT r.recording_key,COALESCE(r.musicbrainz_id,''),COALESCE(r.spotify_uri,'')
	    FROM track_recordings p JOIN recordings r ON r.recording_key=p.recording_key
	    WHERE `+historyPredicate("p.track_id")+` ORDER BY r.recording_key`, t.ID)
	if err != nil {
		return err
	}
	t.Recordings = []models.Recording{}
	for rows.Next() {
		var r models.Recording
		if err = rows.Scan(&r.Key, &r.MusicBrainzID, &r.SpotifyURI); err != nil {
			rows.Close()
			return err
		}
		t.Recordings = append(t.Recordings, r)
	}
	err = rows.Err()
	rows.Close()
	return err
}

package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/gabehf/koito/internal/catalog"
	"github.com/gabehf/koito/internal/db"
	"github.com/gabehf/koito/internal/models"
)

// Native ingestion reads original catalog evidence. View corrections must not
// replace the identity or credits supplied by a subsequent submission.
func (s *Sqlite) originalArtist(ctx context.Context, id int32) (*models.Artist, error) {
	a := &models.Artist{Genres: []string{}, Aliases: []string{}}
	var mbid, image sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT a.id,n.name,a.musicbrainz_id,a.image FROM artists a JOIN artists_with_name n ON n.id=a.id WHERE a.id=?`, id).Scan(&a.ID, &a.Name, &mbid, &image)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, db.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	a.MbzID = parseNullableUUID(mbid)
	a.Image = catalog.BuildImageList(parseNullableUUID(image))
	rows, err := s.db.QueryContext(ctx, `SELECT alias FROM artist_aliases WHERE artist_id=? ORDER BY is_primary DESC,alias`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			return nil, err
		}
		a.Aliases = append(a.Aliases, name)
	}
	return a, rows.Err()
}

func (s *Sqlite) originalCredits(ctx context.Context, kind string, id int32) ([]models.SimpleArtist, error) {
	table, column := "artist_tracks", "track_id"
	if kind == "album" {
		table, column = "artist_releases", "release_id"
	}
	rows, err := s.db.QueryContext(ctx, `SELECT a.id,a.name FROM `+table+` c JOIN artists_with_name a ON a.id=c.artist_id WHERE c.`+column+`=? ORDER BY c.is_primary DESC,a.id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	artists := []models.SimpleArtist{}
	for rows.Next() {
		var a models.SimpleArtist
		if err = rows.Scan(&a.ID, &a.Name); err != nil {
			return nil, err
		}
		artists = append(artists, a)
	}
	return artists, rows.Err()
}

func (s *Sqlite) originalAlbum(ctx context.Context, id int32) (*models.Album, error) {
	a := &models.Album{}
	var mbid, image sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT id,title,musicbrainz_id,image,various_artists FROM releases_with_title WHERE id=?`, id).Scan(&a.ID, &a.Title, &mbid, &image, &a.VariousArtists)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, db.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	a.MbzID = parseNullableUUID(mbid)
	a.Image = catalog.BuildImageList(parseNullableUUID(image))
	a.Artists, err = s.originalCredits(ctx, "album", id)
	return a, err
}

func (s *Sqlite) originalTrack(ctx context.Context, id int32) (*models.Track, error) {
	t := &models.Track{}
	var mbid, image sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT t.id,t.title,t.release_id,t.duration,COALESCE(t.musicbrainz_id,(SELECT r.musicbrainz_id FROM track_recordings tr JOIN recordings r ON r.recording_key=tr.recording_key WHERE tr.track_id=t.id)),r.image FROM tracks_with_title t JOIN releases r ON r.id=t.release_id WHERE t.id=?`, id).Scan(&t.ID, &t.Title, &t.AlbumID, &t.Duration, &mbid, &image)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, db.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	t.MbzID = parseNullableUUID(mbid)
	t.Image = catalog.BuildImageList(parseNullableUUID(image))
	t.Artists, err = s.originalCredits(ctx, "track", id)
	return t, err
}

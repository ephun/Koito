package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/gabehf/koito/internal/models"
	"strings"
)

func (s *Sqlite) GetGenres(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT genre FROM artist_genres ORDER BY genre`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []string{}
	for rows.Next() {
		var genre string
		if err = rows.Scan(&genre); err != nil {
			return nil, err
		}
		result = append(result, genre)
	}
	return result, rows.Err()
}

// Only an unambiguous exact artist alias links this source evidence. Labels
// remain evidence, independently of later manual or filter assignments.
func (s *Sqlite) artistGenreEvidence(ctx context.Context, artistID int32) ([]models.GenreEvidence, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT g.record_id,r.source_id,g.value,g.field
 FROM source_genres g JOIN source_records r ON r.id=g.record_id
 WHERE json_type(r.payload,'$.fields.Artist')='text'
 AND EXISTS(SELECT 1 FROM artist_aliases a WHERE a.artist_id=? AND a.alias=json_extract(r.payload,'$.fields.Artist'))
 AND NOT EXISTS(SELECT 1 FROM artist_aliases a WHERE a.artist_id<>? AND a.alias=json_extract(r.payload,'$.fields.Artist'))
 ORDER BY g.record_id,g.position`, artistID, artistID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []models.GenreEvidence{}
	for rows.Next() {
		var g models.GenreEvidence
		if err = rows.Scan(&g.RecordID, &g.SourceID, &g.Value, &g.Field); err != nil {
			return nil, err
		}
		result = append(result, g)
	}
	return result, rows.Err()
}

func (s *Sqlite) artistGenres(ctx context.Context, id int32) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT genre FROM artist_genres WHERE artist_id=? ORDER BY genre`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	genres := []string{}
	for rows.Next() {
		var genre string
		if err = rows.Scan(&genre); err != nil {
			return nil, err
		}
		genres = append(genres, genre)
	}
	return genres, rows.Err()
}

func (s *Sqlite) genreChartRows(ctx context.Context, query, genre string, artistChart bool, args ...any) (*sql.Rows, error) {
	if genre != "" {
		predicate := `EXISTS(SELECT 1 FROM artist_genres ag JOIN displayed_artist_tracks gat ON gat.artist_id=ag.artist_id WHERE gat.track_id=l.track_id AND ag.genre=?)`
		if artistChart {
			predicate = `EXISTS(SELECT 1 FROM artist_genres ag WHERE ag.artist_id=at2.artist_id AND ag.genre=?)`
		}
		query = strings.Replace(query, "WHERE ", "WHERE "+predicate+" AND ", 1)
		args = append([]any{genre}, args...)
	}
	return s.db.QueryContext(ctx, query, args...)
}

func (s *Sqlite) artistGenreIssues(ctx context.Context, id int32) ([]models.GenreIssue, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT issues_json FROM artist_genre_issues WHERE artist_id=?`, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return []models.GenreIssue{}, nil
	}
	if err != nil {
		return nil, err
	}
	var issues []models.GenreIssue
	err = json.Unmarshal([]byte(raw), &issues)
	return issues, err
}

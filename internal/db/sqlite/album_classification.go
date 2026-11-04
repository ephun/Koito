package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/gabehf/koito/internal/models"
)

func (s *Sqlite) albumClassification(ctx context.Context, id int32, original bool) (bool, error) {
	var value bool
	err := s.db.QueryRowContext(ctx, `SELECT various_artists FROM album_classification WHERE album_id=?`, id).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return original, nil
	}
	return value, err
}

func (s *Sqlite) albumClassificationIssues(ctx context.Context, id int32) ([]models.FilterIssue, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT issues_json FROM album_classification_issues WHERE album_id=?`, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var issues []models.FilterIssue
	err = json.Unmarshal([]byte(raw), &issues)
	return issues, err
}

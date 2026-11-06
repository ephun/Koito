package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/gabehf/koito/internal/models"
)

func (s *Sqlite) catalogName(ctx context.Context, kind string, id int32, original string) (string, error) {
	var name string
	err := s.db.QueryRowContext(ctx, `SELECT display_name FROM catalog_names WHERE kind=? AND item_id=?`, kind, id).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return original, nil
	}
	return name, err
}

func (s *Sqlite) catalogNameIssues(ctx context.Context, kind string, id int32) ([]models.FilterIssue, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT issues_json FROM catalog_name_issues WHERE kind=? AND item_id=?`, kind, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return []models.FilterIssue{}, nil
	}
	if err != nil {
		return nil, err
	}
	var issues []models.FilterIssue
	err = json.Unmarshal([]byte(raw), &issues)
	return issues, err
}

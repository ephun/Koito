package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/gabehf/koito/internal/models"
	"github.com/google/uuid"
)

func (s *Sqlite) catalogIdentifier(ctx context.Context, kind string, id int32, original *uuid.UUID) (*uuid.UUID, error) {
	var identifier string
	err := s.db.QueryRowContext(ctx, `SELECT identifier FROM catalog_identifiers WHERE kind=? AND item_id=?`, kind, id).Scan(&identifier)
	if errors.Is(err, sql.ErrNoRows) {
		return original, nil
	}
	if err != nil {
		return nil, err
	}
	if identifier == "" {
		return nil, nil
	}
	value, err := uuid.Parse(identifier)
	if err != nil {
		return nil, err
	}
	return &value, nil
}
func (s *Sqlite) catalogIdentifierIssues(ctx context.Context, kind string, id int32) ([]models.FilterIssue, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT issues_json FROM catalog_identifier_issues WHERE kind=? AND item_id=?`, kind, id).Scan(&raw)
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

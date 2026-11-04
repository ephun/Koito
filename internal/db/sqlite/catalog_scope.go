package sqlite

import (
	"context"
	"encoding/json"

	"github.com/gabehf/koito/internal/models"
)

func (s *Sqlite) trackScopeIssues(ctx context.Context, id int32) ([]models.FilterIssue, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT issues_json FROM track_scope_issues WHERE `+historyPredicate("track_id")+` ORDER BY track_id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	issues := []models.FilterIssue{}
	seen := map[string]bool{}
	for rows.Next() {
		var raw string
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		var entries []models.FilterIssue
		if err = json.Unmarshal([]byte(raw), &entries); err != nil {
			return nil, err
		}
		for _, entry := range entries {
			key := entry.FilterID + "\x00" + entry.Path + "\x00" + entry.Reason
			if !seen[key] {
				issues = append(issues, entry)
				seen[key] = true
			}
		}
	}
	return issues, rows.Err()
}

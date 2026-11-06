package music

import (
	"context"
	"encoding/json"

	"github.com/gabehf/koito/internal/filters"
)

// Undated counts share evidence and attribution conditions with dated plays.
// They deliberately supply no play timestamp, key, or playback client.
func refreshAggregateScopes(ctx context.Context, db Database, programs ScopePrograms, items []ScopeItem, trackID int64) error {
	if !programs.UsesPlays() {
		return nil
	}
	envs := map[int64]filters.Env{}
	for _, item := range items {
		envs[item.ID] = item.Env
	}
	query := `SELECT a.aggregate_key,a.track_id,COALESCE(json_extract(a.details_json,'$.attribution.listener'),'self'),COALESCE(json_extract(a.details_json,'$.attribution.basis'),''),COALESCE((SELECT json_group_array(record_id) FROM source_aggregate_records r WHERE r.aggregate_key=a.aggregate_key),'[]') FROM source_aggregates a WHERE a.undated_count>0 AND a.track_id IS NOT NULL`
	args := []any{}
	if trackID != 0 {
		query += ` AND a.track_id=?`
		args = append(args, trackID)
	}
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	type count struct {
		key, listener, basis, records string
		track                         int64
	}
	var counts []count
	for rows.Next() {
		var c count
		if err = rows.Scan(&c.key, &c.track, &c.listener, &c.basis, &c.records); err != nil {
			rows.Close()
			return err
		}
		counts = append(counts, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, c := range counts {
		env := filters.Env{}
		for field, value := range envs[c.track] {
			env[field] = value
		}
		env["play.evidence_level"] = filters.String("undated")
		env["play.listener"] = filters.String(c.listener)
		env["play.attribution_basis"] = filters.String(c.basis)
		var ids []int64
		if err = json.Unmarshal([]byte(c.records), &ids); err != nil {
			return err
		}
		values := make([]filters.Value, 0, len(ids))
		for _, id := range ids {
			values = append(values, filters.Integer(id))
		}
		env["play.record_ids"] = filters.Values(values...)
		included, issues, err := programs.Resolve(ctx, env)
		if err != nil {
			return err
		}
		raw, err := json.Marshal(issues)
		if err != nil {
			return err
		}
		if _, err = db.ExecContext(ctx, `INSERT INTO aggregate_scope(aggregate_key,track_id,included,issues_json) VALUES(?,?,?,?)`, c.key, c.track, included, string(raw)); err != nil {
			return err
		}
	}
	return nil
}

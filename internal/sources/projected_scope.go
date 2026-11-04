package sources

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/gabehf/koito/internal/music"
)

type scopeSnapshotItem struct{ Field, State string }

func scopeSnapshot(ctx context.Context, tx *sql.Tx) (map[string]scopeSnapshotItem, error) {
	items, err := music.ScopeItems(ctx, tx, 0)
	if err != nil {
		return nil, err
	}
	titles := map[int64]string{}
	for _, item := range items {
		titles[item.ID] = item.Title
	}
	rows, err := tx.QueryContext(ctx, `SELECT l.track_id,l.listened_at,l.play_key,COALESCE(s.included,t.included,1) FROM listens l LEFT JOIN listen_scope s ON s.track_id=l.track_id AND s.listened_at=l.listened_at AND s.play_key=l.play_key LEFT JOIN track_scope t ON t.track_id=l.track_id ORDER BY l.track_id,l.listened_at,l.play_key`)
	if err != nil {
		return nil, err
	}
	result := map[string]scopeSnapshotItem{}
	for rows.Next() {
		var track, at int64
		var key string
		var included bool
		if err = rows.Scan(&track, &at, &key, &included); err != nil {
			rows.Close()
			return nil, err
		}
		identity := "play:" + key
		if key == "" {
			identity = fmt.Sprintf("native:%d:%d", track, at)
		}
		state := "Excluded"
		if included {
			state = "Included"
		}
		result[identity] = scopeSnapshotItem{titles[track] + " at " + time.Unix(at, 0).UTC().Format(time.RFC3339), state}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = tx.QueryContext(ctx, `SELECT a.aggregate_key,a.track_id,a.undated_count,COALESCE(s.included,1) FROM source_aggregates a LEFT JOIN track_scope s ON s.track_id=a.track_id WHERE a.undated_count>0 ORDER BY a.aggregate_key`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var key string
		var track, count int64
		var included bool
		if err = rows.Scan(&key, &track, &count, &included); err != nil {
			rows.Close()
			return nil, err
		}
		state := fmt.Sprintf("%d excluded", count)
		if included {
			state = fmt.Sprintf("%d included", count)
		}
		result["aggregate:"+key] = scopeSnapshotItem{"Undated counts: " + titles[track], state}
	}
	err = rows.Err()
	rows.Close()
	return result, err
}

// The pending projection uses the publication path, including newly allocated
// catalog identities. Its savepoint is always rolled back before returning.
func previewProjectedScope(ctx context.Context, tx *sql.Tx, base int64, before, after RuleConfig, plan matchPlan) (examples []NamingExample, err error) {
	examples = []NamingExample{}
	active := false
	for _, config := range []RuleConfig{before, after} {
		for _, rule := range config.Rules {
			if rule.Enabled && rule.Type == "inclusion" && rule.Subtype == "scope" {
				active = true
			}
		}
	}
	if !active {
		return examples, nil
	}
	old, err := scopeSnapshot(ctx, tx)
	if err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `SAVEPOINT scope_preview`); err != nil {
		return nil, err
	}
	defer func() {
		_, rollbackErr := tx.ExecContext(context.Background(), `ROLLBACK TO scope_preview`)
		_, releaseErr := tx.ExecContext(context.Background(), `RELEASE scope_preview`)
		if err == nil {
			if rollbackErr != nil {
				err = rollbackErr
			} else if releaseErr != nil {
				err = releaseErr
			}
		}
	}()
	raw, err := json.Marshal(after)
	if err != nil {
		return nil, err
	}
	id, err := insertID(ctx, tx, `INSERT INTO filter_versions(parent_id,config_json,label,created_at) VALUES(?,?,?,?)`, base, string(raw), "Preview", time.Now().Unix())
	if err != nil {
		return nil, err
	}
	if _, err = publishPlan(ctx, tx, plan, id); err != nil {
		return nil, err
	}
	next, err := scopeSnapshot(ctx, tx)
	if err != nil {
		return nil, err
	}
	keys := map[string]bool{}
	for key := range old {
		keys[key] = true
	}
	for key := range next {
		keys[key] = true
	}
	ordered := []string{}
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)
	for _, key := range ordered {
		a, existsBefore := old[key]
		b, existsAfter := next[key]
		if existsBefore && existsAfter && a.State == b.State {
			continue
		}
		if !existsBefore {
			a.State = "Absent"
		}
		if !existsAfter {
			b.State = "Absent"
			b.Field = a.Field
		}
		examples = append(examples, NamingExample{Field: b.Field, Before: a.State, After: b.State})
		if len(examples) == 12 {
			break
		}
	}
	return examples, nil
}

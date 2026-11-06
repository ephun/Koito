package sources

import (
	"context"
	"database/sql"
	"encoding/json"
)

func publishPlayContexts(ctx context.Context, tx *sql.Tx, plan matchPlan) error {
	contexts := map[string]*performanceValues{}
	for _, r := range plan.Records {
		if key := plan.Keys[r.ID]; key != "" && r.Values.Performance != nil {
			if previous := contexts[key]; previous == nil || r.Values.Performance.ID != "" && (previous.ID == "" || previous.ID == r.Values.Performance.ID) {
				contexts[key] = r.Values.Performance
			}
		} else if key != "" && r.Values.TimeEstimated && contexts[key] == nil {
			contexts[key] = &performanceValues{Kind: "unknown", TimeEstimated: true}
		}
	}
	statement, err := tx.PrepareContext(ctx, `INSERT INTO source_play_context(play_key,context_json) VALUES(?,?)`)
	if err != nil {
		return err
	}
	defer statement.Close()
	for key, performance := range contexts {
		raw, err := json.Marshal(performance)
		if err != nil {
			return err
		}
		if _, err = statement.ExecContext(ctx, key, string(raw)); err != nil {
			return err
		}
	}
	return nil
}

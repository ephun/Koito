package sources

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
)

func publishPersonalEvidence(ctx context.Context, tx *sql.Tx, plan matchPlan, config RuleConfig, versionID int64) error {
	// Delete child links in bulk before parent rows to keep large rebuilds linear.
	if _, err := tx.ExecContext(ctx, `DELETE FROM source_personal_evidence_music`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM source_personal_evidence`); err != nil {
		return err
	}
	index := buildPersonalEvidence(plan, config)
	entries, err := tx.PrepareContext(ctx, `INSERT INTO source_personal_evidence(play_key,record_id,details_json) VALUES(?,?,?) ON CONFLICT DO NOTHING`)
	if err != nil {
		return err
	}
	defer entries.Close()
	keys, err := tx.PrepareContext(ctx, `INSERT INTO source_personal_evidence_music(music_key,play_key,record_id,position) VALUES(?,?,?,?) ON CONFLICT DO NOTHING`)
	if err != nil {
		return err
	}
	defer keys.Close()
	for key, values := range index.byMusic {
		for position, value := range values {
			if err = ctx.Err(); err != nil {
				return err
			}
			raw, err := json.Marshal(value.record)
			if err != nil {
				return err
			}
			if _, err = entries.ExecContext(ctx, value.play.Key, value.record.ID, string(raw)); err != nil {
				return err
			}
			if _, err = keys.ExecContext(ctx, key, value.play.Key, value.record.ID, position); err != nil {
				return err
			}
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO source_personal_evidence_state(id,version_id) VALUES(1,?) ON CONFLICT(id) DO UPDATE SET version_id=excluded.version_id`, versionID)
	return err
}

func inspectionEvidence(ctx context.Context, reader sqlReader, config RuleConfig, versionID int64, record matchingRecord) (*evidenceIndex, error) {
	var version int64
	err := reader.QueryRowContext(ctx, `SELECT version_id FROM source_personal_evidence_state WHERE id=1`).Scan(&version)
	if err == sql.ErrNoRows || err == nil && version != versionID {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	index := &evidenceIndex{byMusic: map[string][]personalEvidence{}, sources: map[int64]bool{}}
	sources, err := reader.QueryContext(ctx, `SELECT id FROM source_accounts`)
	if err != nil {
		return nil, err
	}
	for sources.Next() {
		var id int64
		if err = sources.Scan(&id); err != nil {
			sources.Close()
			return nil, err
		}
		index.sources[id] = true
	}
	err = sources.Err()
	sources.Close()
	if err != nil {
		return nil, err
	}
	music, _ := config.matchingRules()
	keys := evidenceMusicKeys(record.Values, music)
	if len(keys) == 0 {
		return index, nil
	}
	args, placeholders := []any{}, []string{}
	for _, key := range keys {
		args = append(args, key)
		placeholders = append(placeholders, "?")
	}
	args = append(args, record.UserID)
	rows, err := reader.QueryContext(ctx, `SELECT m.music_key,e.play_key,e.details_json FROM source_personal_evidence_music m JOIN source_personal_evidence e ON e.play_key=m.play_key AND e.record_id=m.record_id JOIN source_records r ON r.id=e.record_id JOIN source_accounts a ON a.id=r.source_id WHERE m.music_key IN (`+strings.Join(placeholders, ",")+`) AND a.user_id=? ORDER BY m.music_key,m.position`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var key, play, raw string
		if err = rows.Scan(&key, &play, &raw); err != nil {
			return nil, err
		}
		var r matchingRecord
		if err = json.Unmarshal([]byte(raw), &r); err != nil {
			return nil, err
		}
		// The query restricts evidence to this owner. User IDs are omitted from
		// matching records' public JSON.
		r.UserID = record.UserID
		index.byMusic[key] = append(index.byMusic[key], personalEvidence{plannedPlay{Key: play}, r})
	}
	return index, rows.Err()
}

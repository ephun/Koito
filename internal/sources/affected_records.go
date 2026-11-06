package sources

import (
	"context"
	"database/sql"
)

// Find the complete connected set before recalculating it. Range lookups alone
// miss native event identifiers, shifted exact copies, and supporting deliveries.
// All listening for connected music identities is included, preserving evidence
// ordering and repeat checks. Large components use the full rebuild instead.
func affectedRecordIDs(ctx context.Context, tx *sql.Tx, seeds []matchingRecord, config RuleConfig) ([]int64, bool, error) {
	if _, err := storeRecordDependencies(ctx, tx, seeds, config); err != nil {
		return nil, false, err
	}
	if _, err := tx.ExecContext(ctx, `CREATE TEMP TABLE live_affected_records(record_id INTEGER PRIMARY KEY)`); err != nil {
		return nil, false, err
	}
	defer tx.ExecContext(ctx, `DROP TABLE live_affected_records`)
	for _, record := range seeds {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO live_affected_records(record_id) VALUES(?)`, record.ID); err != nil {
			return nil, false, err
		}
	}
	queries := []string{
		`INSERT OR IGNORE INTO live_affected_records SELECT b.record_id FROM live_affected_records a CROSS JOIN source_record_dependencies d ON d.record_id=a.record_id CROSS JOIN source_record_music_keys x ON x.record_id=d.record_id CROSS JOIN source_record_music_keys y ON y.music_key=x.music_key CROSS JOIN source_record_dependencies b ON b.record_id=y.record_id WHERE d.eligible=1 AND b.eligible=1 AND d.user_id=b.user_id`,
		`INSERT OR IGNORE INTO live_affected_records SELECT b.record_id FROM live_affected_records a CROSS JOIN source_record_dependencies d ON d.record_id=a.record_id CROSS JOIN source_record_dependencies b ON b.exact_key=d.exact_key WHERE d.exact_key<>'' AND b.exact_key<>''`,
		`INSERT OR IGNORE INTO live_affected_records SELECT b.record_id FROM live_affected_records a CROSS JOIN source_record_dependencies d ON d.record_id=a.record_id CROSS JOIN source_record_dependencies b ON b.source_id=d.source_id AND b.event_id=d.event_id WHERE d.event_id<>'' AND b.event_id<>''`,
		`INSERT OR IGNORE INTO live_affected_records SELECT b.record_id FROM live_affected_records a CROSS JOIN source_record_dependencies d ON d.record_id=a.record_id CROSS JOIN source_record_dependencies b ON b.source_id=d.source_id AND b.support_id=d.event_id WHERE d.event_id<>'' AND b.support_id<>''`,
		`INSERT OR IGNORE INTO live_affected_records SELECT b.record_id FROM live_affected_records a CROSS JOIN source_record_dependencies d ON d.record_id=a.record_id CROSS JOIN source_record_dependencies b ON b.source_id=d.source_id AND b.event_id=d.support_id WHERE d.support_id<>'' AND b.event_id<>''`,
		`INSERT OR IGNORE INTO live_affected_records SELECT b.record_id FROM live_affected_records a CROSS JOIN source_interpretations d ON d.record_id=a.record_id AND d.policy='filters-v1' CROSS JOIN source_interpretations b ON b.play_key=d.play_key AND b.policy='filters-v1' WHERE d.play_key<>''`,
		`INSERT OR IGNORE INTO live_affected_records SELECT d.left_record FROM live_affected_records a CROSS JOIN match_decisions d ON d.right_record=a.record_id`,
		`INSERT OR IGNORE INTO live_affected_records SELECT d.right_record FROM live_affected_records a CROSS JOIN match_decisions d ON d.left_record=a.record_id`,
		`INSERT OR IGNORE INTO live_affected_records SELECT b.record_id FROM live_affected_records a CROSS JOIN source_personal_evidence_music d ON d.record_id=a.record_id CROSS JOIN source_personal_evidence_music b ON b.music_key=d.music_key`,
		`INSERT OR IGNORE INTO live_affected_records SELECT d.candidate_id FROM live_affected_records a CROSS JOIN source_match_candidates d ON d.record_id=a.record_id`,
		`INSERT OR IGNORE INTO live_affected_records SELECT d.record_id FROM live_affected_records a CROSS JOIN source_match_candidates d ON d.candidate_id=a.record_id`,
	}
	for {
		var added int64
		for _, query := range queries {
			result, err := tx.ExecContext(ctx, query)
			if err != nil {
				return nil, false, err
			}
			n, err := result.RowsAffected()
			if err != nil {
				return nil, false, err
			}
			added += n
		}
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM live_affected_records`).Scan(&count); err != nil {
			return nil, false, err
		}
		if count > 5000 {
			return nil, false, nil
		}
		if added == 0 {
			break
		}
	}
	rows, err := tx.QueryContext(ctx, `SELECT record_id FROM live_affected_records ORDER BY record_id`)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, false, err
		}
		ids = append(ids, id)
	}
	return ids, true, rows.Err()
}

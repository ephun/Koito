package sources

import (
	"context"
	"database/sql"
	"errors"
)

const recordDependencyRevision = 1

// WarmRecordDependencies indexes an already published view without replacing it.
// Pending native records stay beyond the indexed boundary. Historical imports
// and an existing incompatible index require full publication instead.
func (s *Store) WarmRecordDependencies(ctx context.Context) error {
	writeMu.Lock()
	defer writeMu.Unlock()
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	restoreCache, err := publicationCache(ctx, tx)
	if err != nil {
		return err
	}
	defer restoreCache()
	version, err := readRuleVersion(ctx, tx, 0)
	if err != nil {
		return err
	}
	var evidenceVersion int64
	var evidenceRevision int
	err = tx.QueryRowContext(ctx, `SELECT version_id,revision FROM source_personal_evidence_state WHERE id=1`).Scan(&evidenceVersion, &evidenceRevision)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if err == nil && evidenceVersion == version.ID && evidenceRevision != personalEvidenceRevision {
		if err = repairPersonalEvidenceNames(ctx, tx, version.Config); err != nil {
			return err
		}
	}
	var previous int
	err = tx.QueryRowContext(ctx, `SELECT revision FROM source_record_dependency_state WHERE id=1`).Scan(&previous)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if err == nil && previous != recordDependencyRevision {
		return errors.New("The dependency index requires a full rebuild")
	}
	if err == nil {
		var changed bool
		if err = tx.QueryRowContext(ctx, `SELECT max_snapshot_id<>(SELECT COALESCE(MAX(id),0) FROM source_snapshots) FROM source_record_dependency_state WHERE id=1`).Scan(&changed); err != nil {
			return err
		}
		if changed {
			return errors.New("Publish changed snapshots before warming the dependency index")
		}
	}
	var historical bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM source_records r WHERE r.parser<>? AND NOT EXISTS(SELECT 1 FROM source_interpretations i WHERE i.record_id=r.id AND i.policy=?))`, LiveListenBrainz, Policy).Scan(&historical); err != nil {
		return err
	}
	if historical {
		return errors.New("Publish pending historical imports before warming the dependency index")
	}
	var maximum int64
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE((SELECT MIN(r.id)-1 FROM source_records r WHERE NOT EXISTS(SELECT 1 FROM source_interpretations i WHERE i.record_id=r.id AND i.policy=?)),(SELECT MAX(id) FROM source_records),0)`, Policy).Scan(&maximum); err != nil {
		return err
	}
	records, err := loadMatchingRecordsWhere(ctx, tx, version.Config, `WHERE r.id<=?`, []any{maximum})
	if err != nil {
		return err
	}
	if err = publishRecordDependencies(ctx, tx, matchPlan{Records: records}, version.Config, version.ID); err != nil {
		return err
	}
	restoreCache()
	return tx.Commit()
}

// The index describes interpreted records before attribution removes other
// listeners' plays. A later record can change matching and attribution together.
// Original records remain the authority. Filter changes replace this index in
// the same transaction as the view built from those filters.
func publishRecordDependencies(ctx context.Context, tx *sql.Tx, plan matchPlan, config RuleConfig, version int64) error {
	for _, query := range []string{`DELETE FROM source_record_music_keys`, `DELETE FROM source_record_dependencies`} {
		if _, err := tx.ExecContext(ctx, query); err != nil {
			return err
		}
	}
	maxRecord, err := storeRecordDependencies(ctx, tx, plan.Records, config)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO source_record_dependency_state(id,version_id,max_record_id,max_source_id,max_snapshot_id,revision) VALUES(1,?,?,(SELECT COALESCE(MAX(id),0) FROM source_accounts),(SELECT COALESCE(MAX(id),0) FROM source_snapshots),?) ON CONFLICT(id) DO UPDATE SET version_id=excluded.version_id,max_record_id=excluded.max_record_id,max_source_id=excluded.max_source_id,max_snapshot_id=excluded.max_snapshot_id,revision=excluded.revision`, version, maxRecord, recordDependencyRevision)
	return err
}

func storeRecordDependencies(ctx context.Context, tx *sql.Tx, input []matchingRecord, config RuleConfig) (int64, error) {
	records, err := tx.PrepareContext(ctx, `INSERT INTO source_record_dependencies(record_id,source_id,user_id,start_ms,end_ms,event_id,support_id,exact_key,eligible) VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(record_id) DO UPDATE SET start_ms=excluded.start_ms,end_ms=excluded.end_ms,event_id=excluded.event_id,support_id=excluded.support_id,exact_key=excluded.exact_key,eligible=excluded.eligible`)
	if err != nil {
		return 0, err
	}
	defer records.Close()
	keys, err := tx.PrepareContext(ctx, `INSERT INTO source_record_music_keys(record_id,music_key) VALUES(?,?) ON CONFLICT DO NOTHING`)
	if err != nil {
		return 0, err
	}
	defer keys.Close()
	musicRule, _ := config.matchingRules()
	var maxRecord int64
	for _, r := range input {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		if r.ID > maxRecord {
			maxRecord = r.ID
		}
		eligible := r.Values.Reason == "counted"
		if !eligible && r.Values.SupportID == "" {
			continue
		}
		var start, end any
		if !r.Start.IsZero() {
			start = r.Start.UnixMilli()
			_, until := recordInterval(r)
			// Round outwards so the lookup cannot omit sub-millisecond overlap.
			endMS := until.UnixMilli()
			if until.Nanosecond()%1000000 != 0 {
				endMS++
			}
			end = endMS
		}
		exact := ""
		if eligible {
			exact = sourceExactKey(r)
		}
		if _, err = records.ExecContext(ctx, r.ID, r.SourceID, r.UserID, start, end, r.Values.EventID, r.Values.SupportID, exact, eligible); err != nil {
			return 0, err
		}
		if eligible {
			for _, key := range evidenceMusicKeys(r.Values, musicRule) {
				if _, err = keys.ExecContext(ctx, r.ID, key); err != nil {
					return 0, err
				}
			}
		}
	}
	return maxRecord, nil
}

package sources

import (
	"context"
	"database/sql"
	"encoding/json"
)

// Old forwarding records could have been indexed under an empty music name.
// Repair only cached name associations, retaining parent links and original data.
func repairPersonalEvidenceNames(ctx context.Context, tx *sql.Tx, config RuleConfig) error {
	rows, err := tx.QueryContext(ctx, `SELECT m.music_key,m.play_key,m.record_id,e.details_json FROM source_personal_evidence_music m JOIN source_personal_evidence e ON e.play_key=m.play_key AND e.record_id=m.record_id WHERE substr(m.music_key,1,5)='text:'`)
	if err != nil {
		return err
	}
	type entry struct {
		music, play string
		record      int64
	}
	var invalid []entry
	musicRule, _ := config.matchingRules()
	for rows.Next() {
		var e entry
		var raw string
		if err = rows.Scan(&e.music, &e.play, &e.record, &raw); err != nil {
			rows.Close()
			return err
		}
		var record matchingRecord
		if err = json.Unmarshal([]byte(raw), &record); err != nil {
			rows.Close()
			return err
		}
		valid := false
		for _, key := range evidenceMusicKeys(record.Values, musicRule) {
			if key == e.music {
				valid = true
				break
			}
		}
		if !valid {
			invalid = append(invalid, e)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	dirty := map[string]bool{}
	for _, e := range invalid {
		if _, err = tx.ExecContext(ctx, `DELETE FROM source_personal_evidence_music WHERE music_key=? AND play_key=? AND record_id=?`, e.music, e.play, e.record); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM source_personal_evidence WHERE play_key=? AND record_id=? AND NOT EXISTS(SELECT 1 FROM source_personal_evidence_music m WHERE m.play_key=source_personal_evidence.play_key AND m.record_id=source_personal_evidence.record_id)`, e.play, e.record); err != nil {
			return err
		}
		dirty[e.music] = true
	}
	for key := range dirty {
		rows, err = tx.QueryContext(ctx, `SELECT play_key,record_id,position FROM source_personal_evidence_music WHERE music_key=? ORDER BY position,play_key,record_id`, key)
		if err != nil {
			return err
		}
		var changed []entry
		var positions []int
		position := 0
		for rows.Next() {
			var play string
			var record int64
			var previous int
			if err = rows.Scan(&play, &record, &previous); err != nil {
				rows.Close()
				return err
			}
			if previous != position {
				changed = append(changed, entry{key, play, record})
				positions = append(positions, position)
			}
			position++
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for i, e := range changed {
			if _, err = tx.ExecContext(ctx, `UPDATE source_personal_evidence_music SET position=? WHERE music_key=? AND play_key=? AND record_id=?`, positions[i], e.music, e.play, e.record); err != nil {
				return err
			}
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE source_personal_evidence_state SET revision=? WHERE id=1`, personalEvidenceRevision)
	return err
}

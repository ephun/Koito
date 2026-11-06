package sources

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/gabehf/koito/internal/logger"
	"github.com/gabehf/koito/internal/music"
)

// The same calculation handles the complete connected music component. Changes
// to catalog metadata, library inference, or heuristic source universes still
// use full publication. A failed attempt rolls back before that fallback.
func (s *Store) tryIncrementalLive(ctx context.Context) (bool, error) {
	writeMu.Lock()
	defer writeMu.Unlock()
	started := time.Now()
	fallback := func(reason string) (bool, error) {
		logger.FromContext(ctx).Debug().Str("reason", reason).Msg("Live publication requires full rebuild")
		return false, nil
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	version, err := readRuleVersion(ctx, tx, 0)
	if err != nil {
		return false, err
	}
	for _, rule := range version.Config.Rules {
		if !rule.Enabled {
			continue
		}
		if rule.Type == "presentation" || rule.Type == "inference" && rule.Subtype == "count" || rule.Type == "attribution" && rule.Subtype == "heuristic" {
			return fallback("catalog naming, library inference, or heuristic attribution")
		}
	}
	var indexedVersion, maximum int64
	var revision int
	err = tx.QueryRowContext(ctx, `SELECT version_id,max_record_id,revision FROM source_record_dependency_state WHERE id=1`).Scan(&indexedVersion, &maximum, &revision)
	if errors.Is(err, sql.ErrNoRows) {
		return fallback("dependency index is not ready")
	}
	if err != nil {
		return false, err
	}
	if indexedVersion != version.ID || revision != recordDependencyRevision {
		return fallback("dependency version changed")
	}
	var snapshotsChanged bool
	if err = tx.QueryRowContext(ctx, `SELECT max_snapshot_id<>(SELECT COALESCE(MAX(id),0) FROM source_snapshots) FROM source_record_dependency_state WHERE id=1`).Scan(&snapshotsChanged); err != nil {
		return false, err
	}
	if snapshotsChanged {
		return fallback("snapshot membership changed")
	}
	var evidenceVersion int64
	var evidenceRevision int
	err = tx.QueryRowContext(ctx, `SELECT version_id,revision FROM source_personal_evidence_state WHERE id=1`).Scan(&evidenceVersion, &evidenceRevision)
	if errors.Is(err, sql.ErrNoRows) {
		return fallback("personal evidence cache is not ready")
	}
	if err != nil {
		return false, err
	}
	if evidenceVersion != version.ID {
		return fallback("personal evidence cache version changed")
	}
	if evidenceRevision != personalEvidenceRevision {
		if err = repairPersonalEvidenceNames(ctx, tx, version.Config); err != nil {
			return false, err
		}
	}
	var otherImports bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM source_records WHERE id>? AND parser<>?)`, maximum, LiveListenBrainz).Scan(&otherImports); err != nil {
		return false, err
	}
	if otherImports {
		return fallback("historical imports are pending")
	}
	seeds, err := loadMatchingRecordsWhere(ctx, tx, version.Config, `WHERE r.id>?`, []any{maximum})
	if err != nil {
		return false, err
	}
	if len(seeds) == 0 {
		return fallback("pending records precede the indexed boundary")
	}
	logger.FromContext(ctx).Debug().Dur("elapsed", time.Since(started)).Msg("Read pending listening records")
	ids, bounded, err := affectedRecordIDs(ctx, tx, seeds, version.Config)
	if err != nil || !bounded {
		if err == nil {
			return fallback("connected component exceeds incremental limit")
		}
		return false, err
	}
	logger.FromContext(ctx).Debug().Int("records", len(ids)).Dur("elapsed", time.Since(started)).Msg("Located connected listening component")
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	records, err := loadMatchingRecordsWhere(ctx, tx, version.Config, `WHERE r.id IN (`+strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")+`)`, args)
	if err != nil {
		return false, err
	}
	logger.FromContext(ctx).Debug().Dur("elapsed", time.Since(started)).Msg("Read connected listening records")
	for _, record := range records {
		if p := record.Values.Performance; p != nil && p.Kind == "concert" && p.SetlistID != "" && !p.Legacy {
			return fallback("structured concert revisions require complete context")
		}
	}
	decisions, err := loadDecisions(ctx, tx)
	if err != nil {
		return false, err
	}
	// Ignore decisions belonging entirely to another component.
	wanted := map[int64]bool{}
	for _, id := range ids {
		wanted[id] = true
	}
	local := decisions[:0]
	for _, decision := range decisions {
		if wanted[decision.Left] || wanted[decision.Right] {
			local = append(local, decision)
		}
	}
	plan, err := calculateView(ctx, tx, records, version.Config, local)
	if err != nil {
		return false, err
	}
	tracks := map[string]int64{}
	byID := recordIndex(records)
	for _, play := range plan.Plays {
		r := byID[play.Representative]
		track, found, e := existingProjectedTrack(ctx, tx, r)
		if e != nil {
			return false, e
		}
		if !found {
			logger.FromContext(ctx).Debug().Int64("record", r.ID).Msg("An appearance needs catalog refresh")
			return fallback("catalog appearance changed")
		}
		tracks[play.Key] = track
	}
	if err = publishIncrementalPlan(ctx, tx, plan, version, tracks); err != nil {
		return false, err
	}
	if _, err = storeRecordDependencies(ctx, tx, records, version.Config); err != nil {
		return false, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE source_record_dependency_state SET max_record_id=(SELECT COALESCE(MAX(id),0) FROM source_records),max_source_id=(SELECT COALESCE(MAX(id),0) FROM source_accounts) WHERE id=1`); err != nil {
		return false, err
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	logger.FromContext(ctx).Debug().Int("records", len(records)).Int("plays", len(plan.Plays)).Dur("elapsed", time.Since(started)).Msg("Published connected listening component")
	return true, nil
}

// Match the existing catalog lookup without creating or changing an appearance.
// New appearances and new duration information require normal catalog refresh.
func existingProjectedTrack(ctx context.Context, tx *sql.Tx, r matchingRecord) (int64, bool, error) {
	v := r.Values
	var artist, album, base, duration int64
	err := tx.QueryRowContext(ctx, `SELECT artist_id FROM artist_aliases WHERE alias=? ORDER BY is_primary DESC,artist_id LIMIT 1`, v.Artist).Scan(&artist)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	title := v.Album
	if strings.TrimSpace(title) == "" {
		title = "Unknown Album"
	}
	err = tx.QueryRowContext(ctx, `SELECT r.release_id FROM release_aliases r JOIN artist_releases a ON a.release_id=r.release_id WHERE r.alias=? AND a.artist_id=? ORDER BY r.release_id LIMIT 1`, title, artist).Scan(&album)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	identity := ""
	if v.RecordingID != "" || v.URI != "" {
		identity = music.RecordingKey(0, v.RecordingID, v.URI)
	}
	err = tx.QueryRowContext(ctx, `SELECT t.id,t.duration FROM tracks t JOIN track_aliases a ON a.track_id=t.id JOIN artist_tracks ar ON ar.track_id=t.id LEFT JOIN track_recordings tr ON tr.track_id=t.id WHERE t.release_id=? AND a.alias=? AND ar.artist_id=? AND ((?='' AND (tr.recording_key IS NULL OR tr.recording_key LIKE 'appearance:%')) OR tr.recording_key=?) ORDER BY t.id LIMIT 1`, album, v.Title, artist, identity, identity).Scan(&base, &duration)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	if duration == 0 && v.DurationMS/1000 > 0 {
		return 0, false, nil
	}
	key := historyComparisonKey(r)
	if key == "" {
		var overridden bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM track_history_overrides WHERE track_id=? AND comparison_key<>'')`, base).Scan(&overridden); err != nil {
			return 0, false, err
		}
		return base, !overridden, nil
	}
	var selected int64
	err = tx.QueryRowContext(ctx, `SELECT track_id FROM source_history_variants WHERE base_track_id=? AND comparison_key=?`, base, key).Scan(&selected)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return selected, true, nil
}

func publishIncrementalPlan(ctx context.Context, tx *sql.Tx, plan matchPlan, version RuleVersion, tracks map[string]int64) error {
	oldKeys := map[string]bool{}
	touched := map[int64]bool{}
	for _, r := range plan.Records {
		var key string
		err := tx.QueryRowContext(ctx, `SELECT play_key FROM source_interpretations WHERE record_id=? AND policy=?`, r.ID, Policy).Scan(&key)
		if err != nil && err != sql.ErrNoRows {
			return err
		}
		if key != "" {
			oldKeys[key] = true
		}
	}
	for key := range oldKeys {
		var track sql.NullInt64
		err := tx.QueryRowContext(ctx, `SELECT track_id FROM source_plays WHERE play_key=? AND policy=?`, key, Policy).Scan(&track)
		if err != nil && err != sql.ErrNoRows {
			return err
		}
		if track.Valid {
			touched[track.Int64] = true
		}
		for _, query := range []string{
			`DELETE FROM listens WHERE play_key=? AND client IN ('sources:spotify-v1','sources:filters-v1') AND EXISTS(SELECT 1 FROM source_projection p WHERE p.owned=1 AND p.play_key=listens.play_key AND p.track_id=listens.track_id AND p.listened_at=listens.listened_at AND p.user_id=listens.user_id)`,
			`DELETE FROM source_projection WHERE play_key=?`,
			`DELETE FROM source_plays WHERE play_key=? AND policy='filters-v1'`,
			`DELETE FROM source_play_tags WHERE play_key=?`,
			`DELETE FROM source_play_groupings WHERE play_key=?`,
			`DELETE FROM source_play_details WHERE play_key=?`,
			`DELETE FROM source_personal_evidence WHERE play_key=?`,
		} {
			if _, err = tx.ExecContext(ctx, query, key); err != nil {
				return err
			}
		}
	}
	for _, r := range plan.Records {
		if _, err := tx.ExecContext(ctx, `DELETE FROM source_interpretations WHERE record_id=? AND policy=?`, r.ID, Policy); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM source_match_candidates WHERE record_id=? OR candidate_id=?`, r.ID, r.ID); err != nil {
			return err
		}
	}
	for key, attribution := range plan.Attributions {
		raw, err := json.Marshal(attribution)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO source_play_details(play_key,evidence_level,attribution_json,version_id) VALUES(?,'observed',?,?)`, key, string(raw), version.ID); err != nil {
			return err
		}
	}
	for _, r := range plan.Records {
		if key := plan.Keys[r.ID]; key != "" {
			if _, err := tx.ExecContext(ctx, `INSERT INTO source_play_records(play_key,record_id) VALUES(?,?) ON CONFLICT DO NOTHING`, key, r.ID); err != nil {
				return err
			}
		}
	}
	if err := publishPlayContexts(ctx, tx, plan); err != nil {
		return err
	}
	byID := recordIndex(plan.Records)
	for _, play := range plan.Plays {
		r := byID[play.Representative]
		track := tracks[play.Key]
		at := play.At
		if at.IsZero() {
			at = r.Start
		}
		touched[track] = true
		var collision bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM listens l INDEXED BY source_listens_identity WHERE track_id=? AND listened_at=? AND user_id=? AND NOT EXISTS(SELECT 1 FROM source_projection p WHERE p.owned=1 AND p.track_id=l.track_id AND p.listened_at=l.listened_at AND p.user_id=l.user_id AND p.play_key=l.play_key))`, track, at.Unix(), r.UserID).Scan(&collision); err != nil {
			return err
		}
		owned, disposition := 1, "counted"
		if collision {
			owned, disposition = 0, "legacy display overlap"
		} else {
			if _, err := tx.ExecContext(ctx, `INSERT INTO listens(track_id,listened_at,user_id,client,play_key) VALUES(?,?,?,'sources:filters-v1',?)`, track, at.Unix(), r.UserID, play.Key); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO source_projection(track_id,listened_at,user_id,policy,owned,play_key) VALUES(?,?,?,?,?,?)`, track, at.Unix(), r.UserID, Policy, owned, play.Key); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO source_plays(policy,play_key,source_id,representative_record,track_id,listened_at,disposition,version_id) VALUES(?,?,?,?,?,?,?,?)`, Policy, play.Key, r.SourceID, r.ID, track, at.Unix(), disposition, version.ID); err != nil {
			return err
		}
	}
	for _, r := range plan.Records {
		if _, err := tx.ExecContext(ctx, `INSERT INTO source_interpretations(record_id,policy,play_key,reason) VALUES(?,?,?,?)`, r.ID, Policy, plan.Keys[r.ID], plan.Reasons[r.ID]); err != nil {
			return err
		}
	}
	for id, partners := range plan.Ambiguous {
		for _, partner := range partners {
			if _, err := tx.ExecContext(ctx, `INSERT INTO source_match_candidates(record_id,candidate_id) VALUES(?,?)`, id, partner); err != nil {
				return err
			}
		}
	}
	if err := insertPersonalEvidence(ctx, tx, plan, version.Config, version.ID); err != nil {
		return err
	}
	for track := range touched {
		if err := music.RefreshHistoryCanonicalForTrack(ctx, tx, track); err != nil {
			return err
		}
		if err := music.RefreshScope(ctx, tx, track); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE live_submissions SET status='projected',version_id=? WHERE status='stored'`, version.ID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO source_projection_runs(policy,created_at,records,plays,collisions) SELECT ?,?,(SELECT COUNT(*) FROM source_records),(SELECT COUNT(*) FROM source_plays WHERE policy=?),(SELECT COUNT(*) FROM source_projection WHERE owned=0)`, Policy, time.Now().Unix(), Policy)
	return err
}

package sources

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/gabehf/koito/internal/music"
)

func (s *Store) Rebuild(ctx context.Context) (ProjectionResult, error) {
	writeMu.Lock()
	defer writeMu.Unlock()
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return ProjectionResult{}, err
	}
	defer tx.Rollback()
	version, err := readRuleVersion(ctx, tx, 0)
	if err != nil {
		return ProjectionResult{}, err
	}
	records, err := loadMatchingRecords(ctx, tx, version.Config)
	if err != nil {
		return ProjectionResult{}, err
	}
	decisions, err := loadDecisions(ctx, tx)
	if err != nil {
		return ProjectionResult{}, err
	}
	plan, err := calculateView(ctx, tx, records, version.Config, decisions)
	if err != nil {
		return ProjectionResult{}, err
	}
	result, err := publishPlan(ctx, tx, plan, version.ID)
	if err != nil {
		return result, err
	}
	return result, tx.Commit()
}

// publishPlan replaces only owned display rows and derived links, atomically.
// It never updates source records, legacy aliases or unrelated live listens.
func publishPlan(ctx context.Context, tx *sql.Tx, plan matchPlan, versionID int64) (ProjectionResult, error) {
	result := ProjectionResult{Policy: Policy, Records: len(plan.Records), Plays: len(plan.Plays)}
	version, err := readRuleVersion(ctx, tx, versionID)
	if err != nil {
		return result, err
	}
	naming, err := compileNaming(version.Config)
	if err != nil {
		return result, err
	}
	for _, query := range []string{
		`DELETE FROM source_aggregate_records`,
		`DELETE FROM source_aggregates`,
		`DELETE FROM source_play_records`,
		`DELETE FROM source_play_details`,
		`DELETE FROM listens WHERE client IN ('sources:spotify-v1','sources:filters-v1') AND EXISTS(SELECT 1 FROM source_projection p WHERE p.owned=1 AND p.track_id=listens.track_id AND p.listened_at=listens.listened_at AND p.user_id=listens.user_id AND p.play_key=listens.play_key)`,
		`DELETE FROM source_projection`,
		`DELETE FROM source_match_candidates`,
		`DELETE FROM source_plays WHERE policy IN ('spotify-v1','filters-v1')`,
		`DELETE FROM source_interpretations WHERE policy IN ('spotify-v1','filters-v1')`,
	} {
		if _, err := tx.ExecContext(ctx, query); err != nil {
			return result, err
		}
	}
	// Read unowned display rows once. A per-play COUNT with a user condition
	// can make SQLite choose the user index and scan the entire single-user
	// history for every new play.
	legacyKeys := map[[3]int64]bool{}
	legacyRows, err := tx.QueryContext(ctx, `SELECT track_id,listened_at,user_id FROM listens`)
	if err != nil {
		return result, err
	}
	for legacyRows.Next() {
		var track, unix, user int64
		if err = legacyRows.Scan(&track, &unix, &user); err != nil {
			legacyRows.Close()
			return result, err
		}
		legacyKeys[[3]int64{track, unix, user}] = true
	}
	err = legacyRows.Err()
	legacyRows.Close()
	if err != nil {
		return result, err
	}
	byID := map[int64]*matchingRecord{}
	for i := range plan.Records {
		r := &plan.Records[i]
		byID[r.ID] = r
	}
	tracks := map[string]int64{}
	for key, attribution := range plan.Attributions {
		raw, err := json.Marshal(attribution)
		if err != nil {
			return result, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO source_play_details(play_key,evidence_level,attribution_json,version_id) VALUES(?,'observed',?,?)`, key, string(raw), versionID); err != nil {
			return result, err
		}
	}
	for _, r := range plan.Records {
		if key := plan.Keys[r.ID]; key != "" {
			if _, err := tx.ExecContext(ctx, `INSERT INTO source_play_records(play_key,record_id) VALUES(?,?) ON CONFLICT DO NOTHING`, key, r.ID); err != nil {
				return result, err
			}
		}
	}
	for _, play := range plan.Plays {
		r := byID[play.Representative]
		v := spotifyItem{Title: r.Values.Title, Artist: r.Values.Artist, Album: r.Values.Album, URI: r.Values.URI, RecordingID: r.Values.RecordingID}
		v, err = applyNaming(ctx, naming, *r, v)
		if err != nil {
			return result, err
		}
		raw, _ := json.Marshal([]string{v.Artist, v.Album, v.Title, v.URI, v.RecordingID})
		cacheKey := string(raw)
		track, ok := tracks[cacheKey]
		var err error
		if !ok {
			track, err = projectTrack(ctx, tx, v)
			if err != nil {
				return result, err
			}
			tracks[cacheKey] = track
		}
		at := play.At
		if at.IsZero() {
			at = r.Start
		}
		unix := at.Unix()
		if play.Evidence != "observed" {
			if _, err = tx.ExecContext(ctx, `UPDATE source_play_details SET evidence_level=?,interval_start=?,interval_end=? WHERE play_key=?`, play.Evidence, optionalTime(play.IntervalStart), optionalTime(play.IntervalEnd), play.Key); err != nil {
				return result, err
			}
			for _, id := range play.Records {
				if _, err = tx.ExecContext(ctx, `INSERT INTO source_play_records(play_key,record_id) VALUES(?,?) ON CONFLICT DO NOTHING`, play.Key, id); err != nil {
					return result, err
				}
			}
		}
		// An unowned legacy row remains available and is reported as a collision,
		// not silently deleted or claimed as proof that the source records match it.
		owned := 1
		disposition := "counted"
		if legacyKeys[[3]int64{track, unix, int64(r.UserID)}] {
			owned = 0
			result.Collisions++
			disposition = "legacy display overlap"
		} else {
			if _, err = tx.ExecContext(ctx, `INSERT INTO listens(track_id,listened_at,user_id,client,play_key) VALUES(?,?,?,'sources:filters-v1',?)`, track, unix, r.UserID, play.Key); err != nil {
				return result, err
			}
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO source_projection(track_id,listened_at,user_id,policy,owned,play_key) VALUES(?,?,?,?,?,?)`, track, unix, r.UserID, Policy, owned, play.Key); err != nil {
			return result, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO source_plays(policy,play_key,source_id,representative_record,track_id,listened_at,disposition,version_id) VALUES(?,?,?,?,?,?,?,?)`, Policy, play.Key, r.SourceID, r.ID, track, unix, disposition, versionID); err != nil {
			return result, err
		}
	}
	for _, aggregate := range plan.Aggregates {
		r := byID[aggregate.Representative]
		var track any
		if aggregate.Attributed > 0 {
			v := spotifyItem{Artist: r.Values.Artist, Title: r.Values.Title, Album: r.Values.Album, URI: r.Values.URI, RecordingID: r.Values.RecordingID}
			v, err = applyNaming(ctx, naming, *r, v)
			if err != nil {
				return result, err
			}
			id, err := projectTrack(ctx, tx, v)
			if err != nil {
				return result, err
			}
			track = id
		}
		raw, err := json.Marshal(aggregate)
		if err != nil {
			return result, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO source_aggregates(aggregate_key,library_key,track_key,representative_record,track_id,user_id,available_count,attributed_count,observed_count,overlap_count,last_played_count,estimated_count,undated_count,details_json,version_id) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, aggregate.Key, aggregate.LibraryKey, aggregate.TrackKey, aggregate.Representative, track, r.UserID, aggregate.Available, aggregate.Attributed, aggregate.Observed, aggregate.Overlap, aggregate.LastPlayed, aggregate.Estimated, aggregate.Undated, string(raw), versionID); err != nil {
			return result, err
		}
		result.Estimated += aggregate.Estimated
		result.LastPlayed += aggregate.LastPlayed
		if aggregate.Undated > math.MaxInt64-result.Undated {
			return result, fmt.Errorf("Undated counts exceed supported total.")
		}
		result.Undated += aggregate.Undated
		for _, id := range aggregate.Records {
			if _, err = tx.ExecContext(ctx, `INSERT INTO source_aggregate_records(aggregate_key,record_id) VALUES(?,?) ON CONFLICT DO NOTHING`, aggregate.Key, id); err != nil {
				return result, err
			}
		}
	}
	statement, err := tx.PrepareContext(ctx, `INSERT INTO source_interpretations(record_id,policy,play_key,reason) VALUES(?,?,?,?)`)
	if err != nil {
		return result, err
	}
	defer statement.Close()
	for _, r := range plan.Records {
		if _, err = statement.ExecContext(ctx, r.ID, Policy, plan.Keys[r.ID], plan.Reasons[r.ID]); err != nil {
			return result, err
		}
	}
	candidateStatement, err := tx.PrepareContext(ctx, `INSERT INTO source_match_candidates(record_id,candidate_id) VALUES(?,?)`)
	if err != nil {
		return result, err
	}
	defer candidateStatement.Close()
	for id, partners := range plan.Ambiguous {
		for _, partner := range partners {
			if _, err = candidateStatement.ExecContext(ctx, id, partner); err != nil {
				return result, err
			}
		}
	}
	// The version being published may differ from filter_state until commit.
	var rawConfig string
	if err = tx.QueryRowContext(ctx, `SELECT config_json FROM filter_versions WHERE id=?`, versionID).Scan(&rawConfig); err != nil {
		return result, err
	}
	var config RuleConfig
	if err = json.Unmarshal([]byte(rawConfig), &config); err != nil {
		return result, err
	}
	musicRule, _ := config.matchingRules()
	if err = music.RefreshHistories(ctx, tx, !musicRule.Enabled || musicRule.Settings.CaseSensitive); err != nil {
		return result, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO source_projection_runs(policy,created_at,records,plays,collisions) VALUES(?,?,?,?,?)`, Policy, time.Now().Unix(), result.Records, result.Plays, result.Collisions)
	return result, err
}

func optionalTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value.UTC().Format(time.RFC3339Nano)
}

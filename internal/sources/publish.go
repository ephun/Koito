package sources

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
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
	plan, err := calculateMatches(ctx, records, version.Config, decisions)
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
	for _, query := range []string{
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
	for _, play := range plan.Plays {
		r := byID[play.Representative]
		v := spotifyItem{Title: r.Values.Title, Artist: r.Values.Artist, Album: r.Values.Album}
		raw, _ := json.Marshal([]string{v.Artist, v.Album, v.Title})
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
		unix := r.Start.Unix()
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
	_, err = tx.ExecContext(ctx, `INSERT INTO source_projection_runs(policy,created_at,records,plays,collisions) VALUES(?,?,?,?,?)`, Policy, time.Now().Unix(), result.Records, result.Plays, result.Collisions)
	return result, err
}

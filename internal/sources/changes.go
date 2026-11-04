package sources

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"
)

var ErrStalePreview = errors.New("Data changed. Preview again.")

type DecisionChange struct {
	Left   int64  `json:"left"`
	Right  int64  `json:"right"`
	Action string `json:"action"`
	UndoID int64  `json:"undo_id,omitempty"`
}
type FilterChange struct {
	BaseVersion    int64           `json:"base_version"`
	Config         RuleConfig      `json:"config"`
	RestoreVersion int64           `json:"restore_version,omitempty"`
	Decision       *DecisionChange `json:"decision,omitempty"`
	Token          string          `json:"token,omitempty"`
}
type PreviewExample struct {
	Before [][]int64 `json:"before"`
	After  []int64   `json:"after"`
	Title  string    `json:"title"`
	Artist string    `json:"artist"`
}
type FilterPreview struct {
	GroupingExamples []NamingExample  `json:"grouping_examples"`
	MusicExamples    []NamingExample  `json:"music_examples"`
	NamingExamples   []NamingExample  `json:"naming_examples"`
	Token            string           `json:"token"`
	BaseVersion      int64            `json:"base_version"`
	Before           int              `json:"before"`
	After            int              `json:"after"`
	Records          int              `json:"records"`
	Unresolved       int              `json:"unresolved"`
	Ambiguous        int              `json:"ambiguous"`
	Changed          int              `json:"changed"`
	Examples         []PreviewExample `json:"examples"`
}

func resolveChange(ctx context.Context, tx *sql.Tx, change FilterChange) (RuleVersion, RuleConfig, []MatchDecision, *MatchDecision, error) {
	current, err := readRuleVersion(ctx, tx, 0)
	if err != nil {
		return current, RuleConfig{}, nil, nil, err
	}
	if change.BaseVersion != current.ID {
		return current, RuleConfig{}, nil, nil, ErrStalePreview
	}
	config := change.Config
	if change.RestoreVersion != 0 {
		v, e := readRuleVersion(ctx, tx, change.RestoreVersion)
		if e != nil {
			return current, config, nil, nil, fmt.Errorf("Unknown rule version.")
		}
		config = v.Config
	}
	if config.Rules == nil {
		config = current.Config
	}
	if err = validateRules(config); err != nil {
		return current, config, nil, nil, err
	}
	decisions, err := loadDecisions(ctx, tx)
	if err != nil {
		return current, config, nil, nil, err
	}
	var added *MatchDecision
	if d := change.Decision; d != nil {
		added = &MatchDecision{Left: d.Left, Right: d.Right, Action: d.Action}
		if d.UndoID != 0 {
			found := false
			for _, prior := range decisions {
				if prior.ID == d.UndoID {
					added.Left = prior.Left
					added.Right = prior.Right
					found = true
					break
				}
			}
			if !found {
				return current, config, nil, nil, fmt.Errorf("Unknown manual change.")
			}
			// Undo only the current decision for this pair; never erase a newer edit.
			added.Action = "clear"
			for _, prior := range decisions {
				if pairKey(prior.Left, prior.Right) == pairKey(added.Left, added.Right) {
					if prior.ID > d.UndoID {
						return current, config, nil, nil, ErrStalePreview
					}
					if prior.ID < d.UndoID {
						added.Action = prior.Action
					}
				}
			}
		}
		if added.Left > added.Right {
			added.Left, added.Right = added.Right, added.Left
		}
		if added.Left <= 0 || added.Left == added.Right || (added.Action != "match" && added.Action != "split" && added.Action != "clear") {
			return current, config, nil, nil, fmt.Errorf("Choose two different records and a valid action.")
		}
		var n int
		if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM source_records WHERE id IN (?,?)`, added.Left, added.Right).Scan(&n); err != nil {
			return current, config, nil, nil, err
		}
		if n != 2 {
			return current, config, nil, nil, fmt.Errorf("Unknown record.")
		}
		added.ID = 1
		if len(decisions) > 0 {
			added.ID = decisions[len(decisions)-1].ID + 1
		}
		decisions = append(decisions, *added)
	}
	return current, config, decisions, added, nil
}

func validateNewMatch(records []matchingRecord, decision *MatchDecision) error {
	if decision == nil || decision.Action != "match" {
		return nil
	}
	var selected []matchingRecord
	for _, record := range records {
		if record.ID == decision.Left || record.ID == decision.Right {
			selected = append(selected, record)
		}
	}
	if len(selected) != 2 || selected[0].UserID != selected[1].UserID {
		return fmt.Errorf("Records must belong to the same user.")
	}
	if selected[0].Values.Reason != "counted" || selected[1].Values.Reason != "counted" {
		return fmt.Errorf("Both records need usable music and time before matching.")
	}
	return nil
}

func previewChange(ctx context.Context, tx *sql.Tx, change FilterChange) (FilterPreview, matchPlan, RuleConfig, *MatchDecision, error) {
	var preview FilterPreview
	current, config, decisions, added, err := resolveChange(ctx, tx, change)
	if err != nil {
		return preview, matchPlan{}, config, added, err
	}
	records, err := loadMatchingRecords(ctx, tx, config, current.Config)
	if err != nil {
		return preview, matchPlan{}, config, added, err
	}
	if err = validateNewMatch(records, added); err != nil {
		return preview, matchPlan{}, config, added, err
	}
	plan, err := calculateView(ctx, tx, records, config, decisions)
	if err != nil {
		return preview, plan, config, added, err
	}
	preview.NamingExamples, err = previewNaming(ctx, plan, current.Config, config)
	if err != nil {
		return preview, plan, config, added, err
	}
	preview.MusicExamples, err = previewMusic(ctx, tx, plan, current.Config, config)
	if err != nil {
		return preview, plan, config, added, err
	}
	preview.GroupingExamples, err = previewGrouping(ctx, tx, plan, current.Config, config)
	if err != nil {
		return preview, plan, config, added, err
	}
	preview.BaseVersion = current.ID
	preview.After = len(plan.Plays)
	preview.Records = len(records)
	preview.Ambiguous = len(plan.Ambiguous)
	preview.Examples = []PreviewExample{}
	beforeKeys := map[int64]string{}
	beforeGroups := map[string][]int64{}
	rows, err := tx.QueryContext(ctx, `SELECT i.record_id,i.play_key FROM source_interpretations i JOIN source_plays p ON p.play_key=i.play_key AND p.policy=i.policy WHERE i.policy=? ORDER BY i.record_id`, Policy)
	if err != nil {
		return preview, plan, config, added, err
	}
	for rows.Next() {
		var id int64
		var key string
		if err = rows.Scan(&id, &key); err != nil {
			rows.Close()
			return preview, plan, config, added, err
		}
		beforeKeys[id] = key
		beforeGroups[key] = append(beforeGroups[key], id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return preview, plan, config, added, err
	}
	// Inferred events can share one supporting record. Their own play keys
	// identify them independently from the record-to-observed-play relation.
	rows, err = tx.QueryContext(ctx, `SELECT r.play_key,r.record_id FROM source_play_records r JOIN source_play_details d ON d.play_key=r.play_key WHERE d.evidence_level<>'observed' ORDER BY r.play_key,r.record_id`)
	if err != nil {
		return preview, plan, config, added, err
	}
	for rows.Next() {
		var key string
		var id int64
		if err = rows.Scan(&key, &id); err != nil {
			rows.Close()
			return preview, plan, config, added, err
		}
		beforeGroups[key] = append(beforeGroups[key], id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return preview, plan, config, added, err
	}
	// The first migration preview also includes the existing Phase 1 view.
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM source_plays`).Scan(&preview.Before); err != nil {
		return preview, plan, config, added, err
	}
	type previousPlay struct{ representative, unix int64 }
	previous := map[string]previousPlay{}
	rows, err = tx.QueryContext(ctx, `SELECT play_key,representative_record,listened_at FROM source_plays WHERE policy=?`, Policy)
	if err != nil {
		return preview, plan, config, added, err
	}
	for rows.Next() {
		var key string
		var old previousPlay
		if err = rows.Scan(&key, &old.representative, &old.unix); err != nil {
			rows.Close()
			return preview, plan, config, added, err
		}
		previous[key] = old
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return preview, plan, config, added, err
	}
	byID := map[int64]*matchingRecord{}
	for i := range records {
		r := &records[i]
		byID[r.ID] = r
		if r.Values.Reason != "counted" {
			preview.Unresolved++
		}
	}
	touched := map[string]bool{}
	for _, play := range plan.Plays {
		if play.Evidence != "observed" {
			prior, exists := previous[play.Key]
			same := exists && prior.representative == play.Representative && prior.unix == play.At.Unix() && equalIDs(beforeGroups[play.Key], play.Records)
			old := [][]int64{}
			if exists {
				touched[play.Key] = true
				old = append(old, beforeGroups[play.Key])
			}
			if !same {
				preview.Changed++
				if len(preview.Examples) < 12 {
					r := byID[play.Representative]
					preview.Examples = append(preview.Examples, PreviewExample{Before: old, After: play.Records, Title: r.Values.Title, Artist: r.Values.Artist})
				}
			}
			continue
		}
		same := len(play.Records) > 0
		oldKeys := map[string]bool{}
		old := [][]int64{}
		for _, id := range play.Records {
			key := beforeKeys[id]
			if key == "" {
				same = false
			}
			if !oldKeys[key] && key != "" {
				oldKeys[key] = true
				old = append(old, beforeGroups[key])
				touched[key] = true
			}
		}
		if len(old) != 1 || !equalIDs(old[0], play.Records) {
			same = false
		}
		// Also preview changes to representative fields and timestamps, even when
		// the records supporting a play are unchanged.
		if same {
			old, ok := previous[beforeKeys[play.Records[0]]]
			if !ok || old.representative != play.Representative || old.unix != byID[play.Representative].Start.Unix() {
				same = false
			}
		}
		if !same {
			preview.Changed++
			if len(preview.Examples) < 12 {
				r := byID[play.Representative]
				preview.Examples = append(preview.Examples, PreviewExample{Before: old, After: play.Records, Title: r.Values.Title, Artist: r.Values.Artist})
			}
		}
	}
	oldKeys := make([]string, 0, len(beforeGroups))
	for key := range beforeGroups {
		oldKeys = append(oldKeys, key)
	}
	sort.Strings(oldKeys)
	for _, key := range oldKeys {
		ids := beforeGroups[key]
		if !touched[key] {
			preview.Changed++
			if len(preview.Examples) < 12 {
				r := byID[ids[0]]
				preview.Examples = append(preview.Examples, PreviewExample{Before: [][]int64{ids}, After: []int64{}, Title: r.Values.Title, Artist: r.Values.Artist})
			}
		}
	}
	var maxRecord, maxDecision int64
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(id),0) FROM source_records`).Scan(&maxRecord); err != nil {
		return preview, plan, config, added, err
	}
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(id),0) FROM match_decisions`).Scan(&maxDecision); err != nil {
		return preview, plan, config, added, err
	}
	var maxRun int64
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(id),0) FROM source_projection_runs`).Scan(&maxRun); err != nil {
		return preview, plan, config, added, err
	}
	raw, _ := json.Marshal([]any{current.ID, maxRecord, len(records), maxDecision, maxRun, config, added})
	preview.Token = digest(raw)
	return preview, plan, config, added, nil
}

func equalIDs(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (s *Store) Preview(ctx context.Context, change FilterChange) (FilterPreview, error) {
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return FilterPreview{}, err
	}
	defer tx.Rollback()
	preview, _, _, _, err := previewChange(ctx, tx, change)
	return preview, err
}

func (s *Store) Apply(ctx context.Context, change FilterChange) (RuleVersion, error) {
	writeMu.Lock()
	defer writeMu.Unlock()
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return RuleVersion{}, err
	}
	defer tx.Rollback()
	preview, plan, config, decision, err := previewChange(ctx, tx, change)
	if err != nil {
		return RuleVersion{}, err
	}
	if change.Token == "" || change.Token != preview.Token {
		return RuleVersion{}, ErrStalePreview
	}
	return savePlan(ctx, tx, preview.BaseVersion, config, plan, decision, change)
}

// Save calculates and publishes one change in a transaction. The explicit
// preview/apply API remains available to callers that need a separate preview.
func (s *Store) Save(ctx context.Context, change FilterChange) (RuleVersion, error) {
	writeMu.Lock()
	defer writeMu.Unlock()
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return RuleVersion{}, err
	}
	defer tx.Rollback()
	current, config, decisions, decision, err := resolveChange(ctx, tx, change)
	if err != nil {
		return RuleVersion{}, err
	}
	records, err := loadMatchingRecords(ctx, tx, config)
	if err != nil {
		return RuleVersion{}, err
	}
	if err = validateNewMatch(records, decision); err != nil {
		return RuleVersion{}, err
	}
	plan, err := calculateView(ctx, tx, records, config, decisions)
	if err != nil {
		return RuleVersion{}, err
	}
	return savePlan(ctx, tx, current.ID, config, plan, decision, change)
}

func savePlan(ctx context.Context, tx *sql.Tx, baseVersion int64, config RuleConfig, plan matchPlan, decision *MatchDecision, change FilterChange) (RuleVersion, error) {
	var err error
	label := "Edit rules"
	if change.RestoreVersion != 0 {
		label = fmt.Sprintf("Restore rules %d", change.RestoreVersion)
	}
	if decision != nil {
		label = "Manual " + decision.Action
		if change.Decision.UndoID != 0 {
			label = "Undo manual change"
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO match_decisions(left_record,right_record,action,created_at) VALUES(?,?,?,?)`, decision.Left, decision.Right, decision.Action, time.Now().Unix()); err != nil {
			return RuleVersion{}, err
		}
	}
	raw, err := json.Marshal(config)
	if err != nil {
		return RuleVersion{}, err
	}
	id, err := insertID(ctx, tx, `INSERT INTO filter_versions(parent_id,config_json,label,created_at) VALUES(?,?,?,?)`, baseVersion, string(raw), label, time.Now().Unix())
	if err != nil {
		return RuleVersion{}, err
	}
	if _, err = publishPlan(ctx, tx, plan, id); err != nil {
		return RuleVersion{}, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE filter_state SET version_id=? WHERE id=1`, id); err != nil {
		return RuleVersion{}, err
	}
	version, err := readRuleVersion(ctx, tx, id)
	if err != nil {
		return version, err
	}
	return version, tx.Commit()
}

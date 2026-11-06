package music

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/gabehf/koito/internal/filters"
)

func scopeExprReadsPlay(expr filters.Expr) bool {
	if expr.Op == "field" && (strings.HasPrefix(expr.Field, "play.") || strings.HasPrefix(expr.Field, "event.") || strings.HasPrefix(expr.Field, "game.") || strings.HasPrefix(expr.Field, "concert.")) {
		return true
	}
	for _, arg := range expr.Args {
		if scopeExprReadsPlay(arg) {
			return true
		}
	}
	return false
}
func scopeReadsPlay(steps []filters.Statement) bool {
	for _, step := range steps {
		if step.If != nil && scopeExprReadsPlay(*step.If) || step.Value != nil && scopeExprReadsPlay(*step.Value) {
			return true
		}
		if scopeReadsPlay(step.Then) || scopeReadsPlay(step.Else) {
			return true
		}
		for _, branch := range step.ElseIf {
			if scopeExprReadsPlay(branch.If) || scopeReadsPlay(branch.Then) {
				return true
			}
		}
	}
	return false
}
func (p ScopePrograms) UsesPlays() bool {
	for _, program := range p.programs {
		if program.perPlay {
			return true
		}
	}
	return false
}

type ScopePlay struct {
	TrackID, Time                        int64
	Key, Client                          string
	Records                              []filters.Value
	Performance                          *Performance
	Evidence, Listener, AttributionBasis string
}

func (play ScopePlay) Environment(base filters.Env) filters.Env {
	env := filters.Env{}
	for key, value := range base {
		env[key] = value
	}
	env["play.key"], env["play.client"] = filters.String(play.Key), filters.String(play.Client)
	env["play.time"] = filters.Timestamp(time.Unix(play.Time, 0).UTC())
	env["play.record_ids"] = filters.Values(play.Records...)
	evidence := play.Evidence
	if evidence == "" {
		evidence = "observed"
	}
	listener := play.Listener
	if listener == "" {
		listener = "self"
	}
	env["play.evidence_level"] = filters.String(evidence)
	env["play.listener"] = filters.String(listener)
	env["play.attribution_basis"] = filters.String(play.AttributionBasis)
	for field, value := range play.Performance.Environment() {
		env[field] = value
	}
	return env
}
func ScopePlays(ctx context.Context, db Database, trackID int64) ([]ScopePlay, error) {
	query := `SELECT l.track_id,l.listened_at,l.play_key,l.client,COALESCE(d.evidence_level,'observed'),COALESCE(json_extract(d.attribution_json,'$.listener'),'self'),COALESCE(json_extract(d.attribution_json,'$.basis'),'existing history') FROM listens l LEFT JOIN source_play_details d ON d.play_key=l.play_key`
	args := []any{}
	if trackID != 0 {
		query += ` WHERE l.track_id=?`
		args = append(args, trackID)
	}
	query += ` ORDER BY l.track_id,l.listened_at,l.play_key`
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	plays := []ScopePlay{}
	wanted := map[string]bool{}
	for rows.Next() {
		var play ScopePlay
		if err = rows.Scan(&play.TrackID, &play.Time, &play.Key, &play.Client, &play.Evidence, &play.Listener, &play.AttributionBasis); err != nil {
			rows.Close()
			return nil, err
		}
		plays = append(plays, play)
		if play.Key != "" {
			wanted[play.Key] = true
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if len(wanted) == 0 {
		return plays, nil
	}
	rows, err = db.QueryContext(ctx, `SELECT play_key,record_id FROM source_interpretations WHERE play_key<>'' AND policy='filters-v1' ORDER BY play_key,record_id`)
	if err != nil {
		return nil, err
	}
	records := map[string][]filters.Value{}
	for rows.Next() {
		var key string
		var id int64
		if err = rows.Scan(&key, &id); err != nil {
			rows.Close()
			return nil, err
		}
		if wanted[key] {
			records[key] = append(records[key], filters.Integer(id))
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	keys := []string{}
	for key := range wanted {
		keys = append(keys, key)
	}
	contexts, err := LoadPerformances(ctx, db, keys)
	if err != nil {
		return nil, err
	}
	for i := range plays {
		plays[i].Records = records[plays[i].Key]
		plays[i].Performance = contexts[plays[i].Key]
	}
	return plays, nil
}
func saveListenScope(ctx context.Context, db Database, programs ScopePrograms, base filters.Env, play ScopePlay, catalog bool) error {
	included, issues, err := programs.Resolve(ctx, play.Environment(base))
	if err != nil {
		return err
	}
	if included == catalog && len(issues) == 0 {
		return nil
	}
	raw, err := json.Marshal(issues)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `INSERT INTO listen_scope(track_id,listened_at,play_key,included,issues_json) VALUES(?,?,?,?,?)`, play.TrackID, play.Time, play.Key, included, string(raw))
	return err
}
func refreshListenScopes(ctx context.Context, db Database, programs ScopePrograms, items []ScopeItem, trackID int64) error {
	if !programs.UsesPlays() {
		return nil
	}
	envs := map[int64]filters.Env{}
	catalog := map[int64]bool{}
	for _, item := range items {
		envs[item.ID] = item.Env
		included, _, err := programs.ResolveCatalog(ctx, item.Env)
		if err != nil {
			return err
		}
		catalog[item.ID] = included
	}
	plays, err := ScopePlays(ctx, db, trackID)
	if err != nil {
		return err
	}
	for _, play := range plays {
		if err = saveListenScope(ctx, db, programs, envs[play.TrackID], play, catalog[play.TrackID]); err != nil {
			return err
		}
	}
	return nil
}

// Native capture inserts and evaluates one listen in its caller's transaction.
func RefreshListenScope(ctx context.Context, db Database, trackID, listenedAt int64) error {
	var raw string
	if err := db.QueryRowContext(ctx, `SELECT v.config_json FROM filter_versions v JOIN filter_state s ON s.version_id=v.id WHERE s.id=1`).Scan(&raw); err != nil {
		return err
	}
	var config struct {
		Rules []IdentityFilter `json:"rules"`
	}
	if err := json.Unmarshal([]byte(raw), &config); err != nil {
		return err
	}
	programs, err := CompileScopePrograms(config.Rules)
	if err != nil {
		return err
	}
	if !programs.UsesPlays() {
		return nil
	}
	items, err := ScopeItems(ctx, db, trackID)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return nil
	}
	play := ScopePlay{TrackID: trackID, Time: listenedAt}
	if err = db.QueryRowContext(ctx, `SELECT client FROM listens WHERE track_id=? AND listened_at=? AND play_key=''`, trackID, listenedAt).Scan(&play.Client); err != nil {
		return err
	}
	if _, err = db.ExecContext(ctx, `DELETE FROM listen_scope WHERE track_id=? AND listened_at=? AND play_key=''`, trackID, listenedAt); err != nil {
		return err
	}
	catalog, _, err := programs.ResolveCatalog(ctx, items[0].Env)
	if err != nil {
		return err
	}
	return saveListenScope(ctx, db, programs, items[0].Env, play, catalog)
}

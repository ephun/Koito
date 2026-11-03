package sources

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
)

type RuleScope struct {
	Service  string `json:"service,omitempty"`
	SourceID int64  `json:"source_id,omitempty"`
	FileHash string `json:"file_hash,omitempty"`
}
type RuleSettings struct {
	CaseSensitive     bool   `json:"case_sensitive"`
	MatchMissingAlbum bool   `json:"match_missing_album"`
	ToleranceSeconds  int    `json:"tolerance_seconds"`
	SpotifyTime       string `json:"spotify_time,omitempty"`
	PreferredSource   string `json:"preferred_source,omitempty"`
	Timezone          string `json:"timezone"`
}
type Rule struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Subtype  string       `json:"subtype"`
	Enabled  bool         `json:"enabled"`
	Scope    RuleScope    `json:"scope"`
	Settings RuleSettings `json:"settings"`
}
type RuleConfig struct {
	Rules []Rule `json:"rules"`
}
type RuleVersion struct {
	ID        int64      `json:"id"`
	ParentID  int64      `json:"parent_id"`
	Config    RuleConfig `json:"config"`
	Label     string     `json:"label"`
	CreatedAt int64      `json:"created_at"`
}
type TimeFile struct {
	Hash     string `json:"hash"`
	Filename string `json:"filename"`
	SourceID int64  `json:"source_id"`
	Records  int    `json:"records"`
}
type FilterState struct {
	Current   RuleVersion     `json:"current"`
	History   []RuleVersion   `json:"history"`
	TimeFiles []TimeFile      `json:"time_files"`
	Decisions []MatchDecision `json:"decisions"`
}

type sqlReader interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func readRuleVersion(ctx context.Context, reader sqlReader, id int64) (RuleVersion, error) {
	var v RuleVersion
	var raw string
	query := `SELECT id,COALESCE(parent_id,0),config_json,label,created_at FROM filter_versions WHERE id=?`
	if id == 0 {
		query = `SELECT v.id,COALESCE(v.parent_id,0),v.config_json,v.label,v.created_at FROM filter_versions v JOIN filter_state s ON s.version_id=v.id WHERE s.id=1`
		err := reader.QueryRowContext(ctx, query).Scan(&v.ID, &v.ParentID, &raw, &v.Label, &v.CreatedAt)
		if err != nil {
			return v, err
		}
	} else {
		if err := reader.QueryRowContext(ctx, query, id).Scan(&v.ID, &v.ParentID, &raw, &v.Label, &v.CreatedAt); err != nil {
			return v, err
		}
	}
	err := json.Unmarshal([]byte(raw), &v.Config)
	return v, err
}

func (s *Store) Filters(ctx context.Context) (FilterState, error) {
	var state FilterState
	var err error
	state.Current, err = readRuleVersion(ctx, s.DB, 0)
	if err != nil {
		return state, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT id,COALESCE(parent_id,0),config_json,label,created_at FROM filter_versions ORDER BY id DESC LIMIT 50`)
	if err != nil {
		return state, err
	}
	state.History = []RuleVersion{}
	for rows.Next() {
		var v RuleVersion
		var raw string
		if err = rows.Scan(&v.ID, &v.ParentID, &raw, &v.Label, &v.CreatedAt); err != nil {
			rows.Close()
			return state, err
		}
		if err = json.Unmarshal([]byte(raw), &v.Config); err != nil {
			rows.Close()
			return state, err
		}
		state.History = append(state.History, v)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return state, err
	}
	rows.Close()
	// These files contain wall times. Native epoch times keep their explicit UTC
	// interpretation and need no timezone control.
	rows, err = s.DB.QueryContext(ctx, `SELECT r.file_hash,r.source_id,COALESCE((SELECT name FROM source_members m WHERE m.file_hash=r.file_hash ORDER BY parent_hash,position LIMIT 1),(SELECT filename FROM import_receipts i WHERE i.file_hash=r.file_hash ORDER BY id LIMIT 1),substr(r.file_hash,1,12)),COUNT(*) FROM source_records r JOIN source_accounts a ON a.id=r.source_id WHERE a.service='lastfm' AND (
	 r.parser=? OR json_extract(r.payload,'$.source_type') LIKE 'lastfm_csv%' OR
	 (json_extract(r.payload,'$.date.uts') IS NULL AND json_extract(r.payload,'$.date."#text"') IS NOT NULL)
	) GROUP BY r.file_hash,r.source_id ORDER BY r.file_hash,r.source_id`, LastfmCSV)
	if err != nil {
		return state, err
	}
	defer rows.Close()
	state.TimeFiles = []TimeFile{}
	for rows.Next() {
		var f TimeFile
		if err = rows.Scan(&f.Hash, &f.SourceID, &f.Filename, &f.Records); err != nil {
			return state, err
		}
		state.TimeFiles = append(state.TimeFiles, f)
	}
	if err = rows.Err(); err != nil {
		return state, err
	}
	rows.Close()
	rows, err = s.DB.QueryContext(ctx, `SELECT id,left_record,right_record,action,created_at FROM match_decisions ORDER BY id DESC LIMIT 100`)
	if err != nil {
		return state, err
	}
	defer rows.Close()
	state.Decisions = []MatchDecision{}
	for rows.Next() {
		var d MatchDecision
		if err = rows.Scan(&d.ID, &d.Left, &d.Right, &d.Action, &d.CreatedAt); err != nil {
			return state, err
		}
		state.Decisions = append(state.Decisions, d)
	}
	return state, rows.Err()
}

func validateRules(config RuleConfig) error {
	if len(config.Rules) < 2 || len(config.Rules) > 64 {
		return errors.New("Expected 2–64 rules.")
	}
	ids := map[string]bool{}
	music, plays := 0, 0
	timeScopes := map[string]bool{}
	for _, rule := range config.Rules {
		if rule.ID == "" || len(rule.ID) > 120 || ids[rule.ID] {
			return errors.New("Rule IDs must be unique.")
		}
		ids[rule.ID] = true
		if rule.Scope.SourceID < 0 {
			return errors.New("Invalid source scope.")
		}
		if rule.Scope.Service != "" && rule.Scope.Service != "spotify" && rule.Scope.Service != "lastfm" {
			return errors.New("Invalid service scope.")
		}
		if rule.Scope.FileHash != "" {
			b, e := hex.DecodeString(rule.Scope.FileHash)
			if e != nil || len(b) != 32 {
				return errors.New("Invalid file scope.")
			}
		}
		switch {
		case rule.Type == "matching" && rule.Subtype == "music":
			music++
			if rule.Scope != (RuleScope{}) {
				return errors.New("Music matching currently applies to all imports.")
			}
		case rule.Type == "matching" && rule.Subtype == "play":
			plays++
			if rule.Scope != (RuleScope{}) {
				return errors.New("Play matching currently applies to all imports.")
			}
			if rule.Settings.ToleranceSeconds < 0 || rule.Settings.ToleranceSeconds > 3600 {
				return errors.New("Time tolerance must be between 0 and 3600 seconds.")
			}
			if rule.Settings.SpotifyTime != "estimated_start" && rule.Settings.SpotifyTime != "end" {
				return errors.New("Invalid Spotify time setting.")
			}
			if rule.Settings.PreferredSource != "spotify" && rule.Settings.PreferredSource != "lastfm" {
				return errors.New("Invalid displayed source.")
			}
		case rule.Type == "inference" && rule.Subtype == "time":
			if rule.Scope.Service == "spotify" {
				return errors.New("CSV timezone rules apply to Last.fm.")
			}
			if err := requireZone(rule.Settings.Timezone); err != nil {
				return err
			}
			key := fmt.Sprintf("%s:%d:%s", rule.Scope.Service, rule.Scope.SourceID, rule.Scope.FileHash)
			if timeScopes[key] {
				return errors.New("Only one timezone rule may use a scope.")
			}
			timeScopes[key] = true
		default:
			return errors.New("Unsupported rule type.")
		}
	}
	if music != 1 || plays != 1 {
		return errors.New("Exactly one music matching and one play matching rule are required.")
	}
	return nil
}

func (config RuleConfig) matchingRules() (Rule, Rule) {
	var music, plays Rule
	for _, rule := range config.Rules {
		if rule.Type == "matching" {
			if rule.Subtype == "music" {
				music = rule
			} else if rule.Subtype == "play" {
				plays = rule
			}
		}
	}
	return music, plays
}

func (config RuleConfig) zoneFor(sourceID int64, fileHash, service string) string {
	best := -1
	zone := ""
	for _, rule := range config.Rules {
		if !rule.Enabled || rule.Type != "inference" || rule.Subtype != "time" {
			continue
		}
		if rule.Scope.Service != "" && rule.Scope.Service != service {
			continue
		}
		if rule.Scope.SourceID != 0 && rule.Scope.SourceID != sourceID {
			continue
		}
		if rule.Scope.FileHash != "" && rule.Scope.FileHash != fileHash {
			continue
		}
		score := 0
		if rule.Scope.Service != "" {
			score++
		}
		if rule.Scope.SourceID != 0 {
			score += 2
		}
		if rule.Scope.FileHash != "" {
			score += 4
		}
		if score > best {
			best = score
			zone = rule.Settings.Timezone
		}
	}
	return zone
}

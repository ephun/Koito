package sources

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/gabehf/koito/internal/filters"
)

type RuleScope struct {
	Service   string `json:"service,omitempty"`
	SourceID  int64  `json:"source_id,omitempty"`
	FileHash  string `json:"file_hash,omitempty"`
	LibraryID string `json:"library_id,omitempty"`
	TrackID   string `json:"track_id,omitempty"`
	RecordID  int64  `json:"record_id,omitempty"`
	Artist    string `json:"artist,omitempty"`
	Device    string `json:"device,omitempty"`
	From      string `json:"from,omitempty"`
	Until     string `json:"until,omitempty"`
}
type RuleSettings struct {
	CaseSensitive     bool   `json:"case_sensitive"`
	MatchMissingAlbum bool   `json:"match_missing_album"`
	ToleranceSeconds  int    `json:"tolerance_seconds"`
	SpotifyTime       string `json:"spotify_time,omitempty"`
	PreferredSource   string `json:"preferred_source,omitempty"`
	Timezone          string `json:"timezone"`
	Listener          string `json:"listener,omitempty"`
	SharePercent      *int   `json:"share_percent,omitempty"`
	EvidenceSourceID  int64  `json:"evidence_source_id,omitempty"`
	MinimumMatches    int    `json:"minimum_matches,omitempty"`
	CountMode         string `json:"count_mode,omitempty"`
	UseLastPlayed     bool   `json:"use_last_played,omitempty"`
	EstimateTimes     bool   `json:"estimate_times,omitempty"`
}
type Rule struct {
	TimeMigrated     bool `json:"time_migrated,omitempty"`
	timeTargets      map[string]bool
	Migration        *ProgramMigration `json:"migration,omitempty"`
	legacyComparison string
	evidenceFields   []string
	Priority         *int `json:"priority,omitempty"`
	compiled         *filters.CompiledProgram
	Name             string           `json:"name,omitempty"`
	Program          *filters.Program `json:"program,omitempty"`
	ID               string           `json:"id"`
	Type             string           `json:"type"`
	Subtype          string           `json:"subtype"`
	Enabled          bool             `json:"enabled"`
	Scope            RuleScope        `json:"scope"`
	Settings         RuleSettings     `json:"settings"`
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
	Blocks    ProgramSchema   `json:"blocks"`
	Current   RuleVersion     `json:"current"`
	History   []RuleVersion   `json:"history"`
	TimeFiles []TimeFile      `json:"time_files"`
	Decisions []MatchDecision `json:"decisions"`
	Sources   []FilterSource  `json:"sources"`
	Files     []FilterFile    `json:"files"`
	Libraries []FilterLibrary `json:"libraries"`
}

type FilterSource struct {
	ID      int64  `json:"id"`
	Service string `json:"service"`
	Records int    `json:"records"`
}

type FilterFile struct {
	Hash     string `json:"hash"`
	SourceID int64  `json:"source_id"`
	Service  string `json:"service"`
	Filename string `json:"filename"`
}
type FilterLibrary struct {
	ID       string `json:"id"`
	SourceID int64  `json:"source_id"`
	Filename string `json:"filename"`
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
	state.Blocks = ProgramSchema{Operators: filters.Operators, Schemas: map[string]filters.Schema{"presentation:naming": namingSchema(), "attribution:direct": directSchema(), "attribution:aggregate": aggregateSchema(), "attribution:heuristic": heuristicSchema(), "inference:count": countSchema(), "inference:time": timeSchema()}}
	state.Blocks.Templates = map[string][]ProgramTemplate{"inference:count": {{"maximum", "Largest reported count", countProgram("maximum", RuleScope{})}, {"latest", "Latest reported count", countProgram("latest", RuleScope{})}, {"reset_total", "Assume decreases restart the counter", countProgram("reset_total", RuleScope{})}}}
	state.Blocks.Templates["inference:time"] = []ProgramTemplate{{"lastfm", "Last.fm timezone", legacyTimeProgram(Rule{Scope: RuleScope{Service: "lastfm"}, Settings: RuleSettings{Timezone: "UTC"}})}, {"spotify", "Spotify timestamps", spotifyTimeProgram("estimated_start")}, {"itunes", "iTunes play dates", legacyTimeProgram(Rule{Scope: RuleScope{Service: "itunes"}})}}
	var err error
	state.Current, err = readRuleVersion(ctx, s.DB, 0)
	if err != nil {
		return state, err
	}
	migrated := migrateTimePrograms(migrateCountPrograms(migrateHeuristicPrograms(migrateAggregatePrograms(migrateDirectPrograms(state.Current.Config)))))
	state.Blocks.Legacy = map[string]Rule{}
	for i, rule := range migrated.Rules {
		if i >= len(state.Current.Config.Rules) {
			state.Blocks.Additions = append(state.Blocks.Additions, rule)
			continue
		}
		if state.Current.Config.Rules[i].Program == nil && rule.Program != nil || state.Current.Config.Rules[i].TimeMigrated != rule.TimeMigrated {
			state.Blocks.Legacy[rule.ID] = rule
		}
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
	err = rows.Err()
	rows.Close()
	if err != nil {
		return state, err
	}
	// Rule editing needs source identity and available records, not the full
	// contribution summaries used by Imports.
	state.Sources = []FilterSource{}
	rows, err = s.DB.QueryContext(ctx, `SELECT a.id,a.service,(SELECT COUNT(*) FROM source_records r WHERE r.source_id=a.id) FROM source_accounts a ORDER BY a.id`)
	if err != nil {
		return state, err
	}
	for rows.Next() {
		var source FilterSource
		if err = rows.Scan(&source.ID, &source.Service, &source.Records); err != nil {
			rows.Close()
			return state, err
		}
		state.Sources = append(state.Sources, source)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return state, err
	}
	state.Files = []FilterFile{}
	state.Libraries = []FilterLibrary{}
	rows, err = s.DB.QueryContext(ctx, `SELECT DISTINCT e.file_hash,e.source_id,a.service,COALESCE((SELECT name FROM source_members m WHERE m.file_hash=e.file_hash ORDER BY parent_hash,position LIMIT 1),(SELECT filename FROM import_receipts i WHERE i.file_hash=e.file_hash ORDER BY id LIMIT 1),substr(e.file_hash,1,12)) FROM source_extractions e JOIN source_accounts a ON a.id=e.source_id WHERE e.parser<>? ORDER BY e.source_id,e.file_hash`, ITunesExpanded)
	if err != nil {
		return state, err
	}
	for rows.Next() {
		var f FilterFile
		if err = rows.Scan(&f.Hash, &f.SourceID, &f.Service, &f.Filename); err != nil {
			rows.Close()
			return state, err
		}
		state.Files = append(state.Files, f)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return state, err
	}
	rows, err = s.DB.QueryContext(ctx, `SELECT DISTINCT json_extract(payload,'$.library_id'),source_id,file_hash FROM source_records WHERE kind='library' AND json_extract(payload,'$.library_id')<>'' ORDER BY source_id,id`)
	if err != nil {
		return state, err
	}
	defer rows.Close()
	seen := map[string]bool{}
	for rows.Next() {
		var library FilterLibrary
		var hash string
		if err = rows.Scan(&library.ID, &library.SourceID, &hash); err != nil {
			return state, err
		}
		key := fmt.Sprintf("%d:%s", library.SourceID, library.ID)
		if seen[key] {
			continue
		}
		seen[key] = true
		library.Filename = library.ID
		for _, f := range state.Files {
			if f.Hash == hash && f.SourceID == library.SourceID {
				library.Filename = f.Filename
				break
			}
		}
		state.Libraries = append(state.Libraries, library)
	}
	if err = rows.Err(); err != nil {
		return state, err
	}
	rows.Close()
	schema := state.Blocks.Schemas["presentation:naming"]
	schema.Choices = map[string][]filters.Choice{}
	for _, source := range state.Sources {
		if source.Records > 0 {
			name := map[string]string{"spotify": "Spotify", "lastfm": "Last.fm", "itunes": "iTunes"}[source.Service]
			if name == "" {
				name = source.Service
			}
			schema.Choices["source.service"] = append(schema.Choices["source.service"], filters.Choice{Label: name, Value: source.Service})
		}
	}
	for _, file := range state.Files {
		schema.Choices["file.hash"] = append(schema.Choices["file.hash"], filters.Choice{Label: file.Filename, Value: file.Hash})
	}
	state.Blocks.Schemas["presentation:naming"] = schema
	heuristic := state.Blocks.Schemas["attribution:heuristic"]
	heuristic.Labels = map[string]string{}
	for _, source := range state.Sources {
		if source.Records > 0 && source.Service != "itunes" {
			name := map[string]string{"spotify": "Spotify", "lastfm": "Last.fm"}[source.Service]
			if name == "" {
				name = source.Service
			}
			field := fmt.Sprintf("evidence:%d", source.ID)
			heuristic.Fields[field] = filters.List
			heuristic.Labels[field] = "Personal plays from " + name
		}
	}
	state.Blocks.Schemas["attribution:heuristic"] = heuristic
	for key, other := range state.Blocks.Schemas {
		if key == "presentation:naming" {
			continue
		}
		if other.Choices == nil {
			other.Choices = map[string][]filters.Choice{}
		}
		other.Choices["source.service"] = schema.Choices["source.service"]
		other.Choices["file.hash"] = schema.Choices["file.hash"]
		if other.SnapshotFields {
			other.Choices["file.hash"] = nil
			for _, file := range state.Files {
				if file.Service == "itunes" {
					other.Choices["file.hash"] = append(other.Choices["file.hash"], filters.Choice{Label: file.Filename, Value: file.Hash})
				}
			}
		}
		state.Blocks.Schemas[key] = other
	}
	return state, nil
}

func validateRules(config RuleConfig) error {
	if len(config.Rules) < 2 || len(config.Rules) > 64 {
		return errors.New("Expected 2–64 rules.")
	}
	ids := map[string]bool{}
	music, plays := 0, 0
	timeScopes := map[string]bool{}
	for _, rule := range config.Rules {
		if rule.Migration != nil {
			for _, value := range []string{rule.Migration.ProgramHash, rule.Migration.SettingsHash} {
				decoded, err := hex.DecodeString(value)
				if err != nil || len(decoded) != 32 {
					return errors.New("Invalid filter migration information.")
				}
			}
		}
		if rule.Priority != nil && (*rule.Priority < 0 || *rule.Priority > 100000) {
			return errors.New("Filter priority must be between 0 and 100000.")
		}
		if rule.Program != nil && !(rule.Type == "presentation" && rule.Subtype == "naming" || rule.Type == "attribution" && (rule.Subtype == "direct" || rule.Subtype == "aggregate" || rule.Subtype == "heuristic") || rule.Type == "inference" && (rule.Subtype == "count" || rule.Subtype == "time")) {
			return errors.New("This filter type does not support blocks yet.")
		}
		if rule.ID == "" || len(rule.ID) > 120 || ids[rule.ID] {
			return errors.New("Rule IDs must be unique.")
		}
		ids[rule.ID] = true
		if rule.Scope.SourceID < 0 || rule.Scope.RecordID < 0 {
			return errors.New("Invalid source scope.")
		}
		if rule.Scope.Service != "" && rule.Scope.Service != "spotify" && rule.Scope.Service != "lastfm" && rule.Scope.Service != "itunes" {
			return errors.New("Invalid service scope.")
		}
		if rule.Scope.FileHash != "" {
			b, e := hex.DecodeString(rule.Scope.FileHash)
			if e != nil || len(b) != 32 {
				return errors.New("Invalid file scope.")
			}
		}
		var from, until time.Time
		for name, value := range map[string]string{"from": rule.Scope.From, "until": rule.Scope.Until} {
			if value == "" {
				continue
			}
			date, err := time.Parse(time.RFC3339Nano, value)
			if err != nil {
				return errors.New("Date scopes need an explicit timezone.")
			}
			if name == "from" {
				from = date
			} else {
				until = date
			}
		}
		if !from.IsZero() && !until.IsZero() && !until.After(from) {
			return errors.New("The end must follow the start.")
		}
		if len(rule.Scope.LibraryID) > 200 || len(rule.Scope.TrackID) > 200 || len(rule.Scope.Artist) > 1000 || len(rule.Scope.Device) > 1000 {
			return errors.New("Rule scope is too long.")
		}
		if rule.Settings.SharePercent != nil && (*rule.Settings.SharePercent < 0 || *rule.Settings.SharePercent > 100) {
			return errors.New("The share must be between 0 and 100 percent.")
		}
		switch {
		case rule.Type == "presentation" && rule.Subtype == "naming":
			if rule.Program == nil {
				return errors.New("Naming filters need IF and SET blocks.")
			}
			if _, err := filters.CompileProgram(*rule.Program, namingSchema()); err != nil {
				return fmt.Errorf("Filter %s: %w", rule.ID, err)
			}
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
		case rule.Type == "attribution" && rule.Subtype == "direct":
			if rule.Program != nil {
				if rule.Scope != (RuleScope{}) {
					return errors.New("Move direct attribution conditions into IF blocks.")
				}
				if _, err := filters.CompileProgram(*rule.Program, directSchema()); err != nil {
					return err
				}
				continue
			}
			if rule.Settings.Listener != "self" && rule.Settings.Listener != "other" && rule.Settings.Listener != "unknown" {
				return errors.New("Choose a listener.")
			}
		case rule.Type == "attribution" && rule.Subtype == "aggregate":
			if rule.Program != nil {
				if rule.Scope != (RuleScope{}) {
					return errors.New("Move aggregate attribution conditions into IF blocks.")
				}
				if _, err := filters.CompileProgram(*rule.Program, aggregateSchema()); err != nil {
					return err
				}
				continue
			}
			if rule.Scope.Service != "itunes" || rule.Settings.SharePercent == nil {
				return errors.New("Aggregate attribution needs an iTunes scope and share.")
			}
			if rule.Scope.Device != "" || rule.Scope.From != "" || rule.Scope.Until != "" {
				return errors.New("Aggregate shares apply to whole counts.")
			}
		case rule.Type == "attribution" && rule.Subtype == "heuristic":
			if rule.Program != nil {
				if rule.Scope != (RuleScope{}) {
					return errors.New("Move heuristic attribution conditions into IF blocks.")
				}
				if _, err := filters.CompileProgram(*rule.Program, heuristicSchema()); err != nil {
					return err
				}
				continue
			}
			if rule.Scope.Service == "" {
				return errors.New("Choose a source for heuristic attribution.")
			}
			if rule.Settings.EvidenceSourceID <= 0 || rule.Settings.MinimumMatches < 1 || rule.Settings.MinimumMatches > 100000 {
				return errors.New("Choose an evidence source and minimum matches.")
			}
			if rule.Scope.Service == "itunes" && rule.Settings.SharePercent == nil {
				return errors.New("Library heuristics need an explicit share.")
			}
		case rule.Type == "inference" && rule.Subtype == "count":
			if rule.Program != nil {
				if rule.Scope != (RuleScope{}) {
					return errors.New("Move Count conditions into snapshot selection and IF blocks.")
				}
				if _, err := filters.CompileProgram(*rule.Program, countSchema()); err != nil {
					return err
				}
				continue
			}
			if rule.Scope.Service != "itunes" {
				return errors.New("Count inference needs an iTunes scope.")
			}
			if rule.Settings.CountMode != "maximum" && rule.Settings.CountMode != "latest" && rule.Settings.CountMode != "reset_total" {
				return errors.New("Choose how to interpret cumulative counts.")
			}
			if rule.Scope.Device != "" || rule.Scope.From != "" || rule.Scope.Until != "" {
				return errors.New("Count inference applies to whole counts.")
			}
		case rule.Type == "inference" && rule.Subtype == "time" && rule.Program != nil:
			if rule.Scope != (RuleScope{}) {
				return errors.New("Move Time conditions into IF blocks.")
			}
			if _, err := filters.CompileProgram(*rule.Program, timeSchema()); err != nil {
				return err
			}
		case rule.Type == "inference" && rule.Subtype == "time" && rule.Scope.Service == "itunes":
		case rule.Type == "inference" && rule.Subtype == "time":
			if rule.Scope.LibraryID != "" || rule.Scope.TrackID != "" || rule.Scope.RecordID != 0 || rule.Scope.Artist != "" || rule.Scope.Device != "" || rule.Scope.From != "" || rule.Scope.Until != "" {
				return errors.New("Timezone rules use source and file scopes.")
			}
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
		if !rule.Enabled || rule.Type != "inference" || rule.Subtype != "time" || rule.Program != nil {
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

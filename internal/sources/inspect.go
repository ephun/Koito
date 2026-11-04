package sources

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gabehf/koito/internal/filters"
)

type InspectedRecord struct {
	matchingRecord
	Reason  string `json:"reason"`
	PlayKey string `json:"play_key"`
}

type RecordInspection struct {
	Record        InspectedRecord         `json:"record"`
	Contributors  []InspectedRecord       `json:"contributors"`
	Candidates    []InspectedRecord       `json:"candidates"`
	Attribution   *Attribution            `json:"attribution,omitempty"`
	Contributions []AggregateContribution `json:"contributions"`
	FilterIssues  []FilterIssue           `json:"filter_issues"`
}

type FilterIssue struct {
	FilterID string `json:"filter_id"`
	Name     string `json:"name"`
	filters.Issue
}

// Diagnostic checks evaluate enabled programs against this record. Naming
// filters see earlier naming changes. Direct attribution reads original fields.
func inspectFilterIssues(ctx context.Context, r matchingRecord, config RuleConfig, evidence *evidenceIndex) ([]FilterIssue, error) {
	issues := []FilterIssue{}
	namingEnv := recordEnvironment(r)
	from, until := recordInterval(r)
	for _, rule := range config.Rules {
		if !rule.Enabled || rule.Program == nil {
			continue
		}
		var schema filters.Schema
		var env filters.Env
		switch {
		case rule.Type == "presentation" && rule.Subtype == "naming":
			if !scopeContains(rule.Scope, r, from, until) {
				continue
			}
			schema, env = namingSchema(), namingEnv
		case rule.Type == "attribution" && rule.Subtype == "direct":
			schema, env = directSchema(), attributionEnvironment(r, from, until)
		case rule.Type == "attribution" && rule.Subtype == "aggregate":
			if r.Service != "itunes" {
				continue
			}
			schema, env = aggregateSchema(), attributionEnvironment(r, from, until)
		case rule.Type == "attribution" && rule.Subtype == "heuristic":
			schema, env = heuristicSchema(), heuristicEnvironment(rule, r, config, evidence, from, until, time.Time{}, time.Time{})
		default:
			continue
		}
		compiled, err := filters.CompileProgram(*rule.Program, schema)
		if err != nil {
			return nil, err
		}
		result, err := compiled.Run(ctx, env)
		if err != nil {
			return nil, err
		}
		if rule.Type == "attribution" && rule.Subtype == "heuristic" {
			if issue := heuristicDomainIssue(r, result); issue != nil {
				result.Issues = append(result.Issues, *issue)
			}
		}
		for _, issue := range result.Issues {
			issues = append(issues, FilterIssue{rule.ID, rule.Name, issue})
		}
		if rule.Type == "presentation" && len(result.Issues) == 0 {
			for key, value := range result.Values {
				namingEnv[key] = value
			}
		}
	}
	return issues, nil
}

const inspectedColumns = `r.id,r.source_id,a.service,r.file_hash,r.ordinal,r.parser,r.payload,a.user_id,COALESCE(i.reason,'not projected'),COALESCE(i.play_key,'')`
const inspectedTables = ` FROM source_records r JOIN source_accounts a ON a.id=r.source_id LEFT JOIN source_interpretations i ON i.record_id=r.id AND i.policy='filters-v1' `

func inspectedRecords(ctx context.Context, reader sqlReader, config RuleConfig, where string, args ...any) ([]InspectedRecord, error) {
	rows, err := reader.QueryContext(ctx, `SELECT `+inspectedColumns+inspectedTables+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	rawFields := requestedRawFields(config)
	out := []InspectedRecord{}
	for rows.Next() {
		var r InspectedRecord
		if err = rows.Scan(&r.ID, &r.SourceID, &r.Service, &r.FileHash, &r.Ordinal, &r.Parser, &r.Payload, &r.UserID, &r.Reason, &r.PlayKey); err != nil {
			return nil, err
		}
		interpretMatchingRecord(&r.matchingRecord, config)
		if err = retainRawFields(&r.matchingRecord, rawFields); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) Inspect(ctx context.Context, id int64) (RecordInspection, error) {
	var result RecordInspection
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	version, err := readRuleVersion(ctx, tx, 0)
	if err != nil {
		return result, err
	}
	rows, err := inspectedRecords(ctx, tx, version.Config, `WHERE r.id=?`, id)
	if err != nil {
		return result, err
	}
	if len(rows) == 0 {
		return result, fmt.Errorf("Unknown record.")
	}
	result.Record = rows[0]
	var personal *evidenceIndex
	for _, rule := range version.Config.Rules {
		if rule.Enabled && rule.Type == "attribution" && rule.Subtype == "heuristic" && rule.Program != nil {
			personal, err = inspectionEvidence(ctx, tx, version.Config, version.ID, result.Record.matchingRecord)
			if err != nil {
				return result, err
			}
			break
		}
	}
	result.FilterIssues, err = inspectFilterIssues(ctx, result.Record.matchingRecord, version.Config, personal)
	if err != nil {
		return result, err
	}
	result.Contributions = []AggregateContribution{}
	if result.Record.PlayKey != "" {
		var raw string
		err = tx.QueryRowContext(ctx, `SELECT attribution_json FROM source_play_details WHERE play_key=?`, result.Record.PlayKey).Scan(&raw)
		if err != nil && err != sql.ErrNoRows {
			return result, err
		}
		if err == nil {
			var value Attribution
			if err = json.Unmarshal([]byte(raw), &value); err != nil {
				return result, err
			}
			result.Attribution = &value
		}
	}
	contributions, err := tx.QueryContext(ctx, `SELECT a.details_json FROM source_aggregates a JOIN source_aggregate_records r ON r.aggregate_key=a.aggregate_key WHERE r.record_id=? ORDER BY a.aggregate_key`, id)
	if err != nil {
		return result, err
	}
	for contributions.Next() {
		var raw string
		var value AggregateContribution
		if err = contributions.Scan(&raw); err != nil {
			contributions.Close()
			return result, err
		}
		if err = json.Unmarshal([]byte(raw), &value); err != nil {
			contributions.Close()
			return result, err
		}
		result.Contributions = append(result.Contributions, value)
	}
	err = contributions.Err()
	contributions.Close()
	if err != nil {
		return result, err
	}
	result.Contributors = []InspectedRecord{}
	result.Candidates = []InspectedRecord{}
	if result.Record.PlayKey != "" {
		result.Contributors, err = inspectedRecords(ctx, tx, version.Config, `WHERE i.play_key=? ORDER BY r.id`, result.Record.PlayKey)
		if err != nil {
			return result, err
		}
	}
	result.Candidates, err = inspectedRecords(ctx, tx, version.Config, `WHERE r.id IN (SELECT candidate_id FROM source_match_candidates WHERE record_id=?) ORDER BY r.id`, id)
	return result, err
}

type PlayListItem struct {
	Key         string       `json:"key"`
	RecordID    int64        `json:"record_id"`
	Title       string       `json:"title"`
	Artist      string       `json:"artist"`
	Album       string       `json:"album"`
	Time        string       `json:"time"`
	Records     int          `json:"records"`
	Reason      string       `json:"reason"`
	Evidence    string       `json:"evidence"`
	Attribution *Attribution `json:"attribution,omitempty"`
	Cursor      string       `json:"cursor"`
}

func (s *Store) Plays(ctx context.Context, after int64, query string, ambiguous bool) ([]PlayListItem, error) {
	return s.PlaysAfter(ctx, after, "", query, ambiguous)
}

func (s *Store) PlaysAfter(ctx context.Context, after int64, afterKey, query string, ambiguous bool) ([]PlayListItem, error) {
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	version, err := readRuleVersion(ctx, tx, 0)
	if err != nil {
		return nil, err
	}
	where := `WHERE (r.id>? OR (r.id=? AND p.play_key>? AND ?<>''))`
	args := []any{after, after, afterKey, afterKey}
	if ambiguous {
		where += ` AND EXISTS(SELECT 1 FROM source_match_candidates c JOIN source_interpretations other ON other.record_id=c.record_id AND other.policy='filters-v1' WHERE other.play_key=i.play_key)`
	}
	if query != "" {
		where += ` AND (CAST(r.id AS TEXT)=? OR r.payload LIKE ? ESCAPE '\')`
		args = append(args, query, "%"+strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(query)+"%")
	}
	rows, err := tx.QueryContext(ctx, `SELECT `+inspectedColumns+`,p.play_key,p.listened_at,COALESCE(d.evidence_level,'observed'),d.attribution_json,(SELECT COUNT(*) FROM source_play_records pr WHERE pr.play_key=p.play_key)`+inspectedTables+` JOIN source_plays p ON p.representative_record=r.id AND p.policy='filters-v1' LEFT JOIN source_play_details d ON d.play_key=p.play_key `+where+` ORDER BY r.id,p.play_key LIMIT 100`, args...)
	if err != nil {
		return nil, err
	}
	out := []PlayListItem{}
	defer rows.Close()
	for rows.Next() {
		var r InspectedRecord
		var key, level string
		var attribution sql.NullString
		var unix int64
		var n int
		if err = rows.Scan(&r.ID, &r.SourceID, &r.Service, &r.FileHash, &r.Ordinal, &r.Parser, &r.Payload, &r.UserID, &r.Reason, &r.PlayKey, &key, &unix, &level, &attribution, &n); err != nil {
			return nil, err
		}
		interpretMatchingRecord(&r.matchingRecord, version.Config)
		item := PlayListItem{Key: key, RecordID: r.ID, Title: r.Values.Title, Artist: r.Values.Artist, Album: r.Values.Album, Time: time.Unix(unix, 0).UTC().Format(time.RFC3339Nano), Records: n, Reason: r.Reason, Evidence: level, Cursor: fmt.Sprintf("%d:%s", r.ID, key)}
		if level != "observed" {
			item.Reason = level
		}
		if attribution.Valid {
			var value Attribution
			if err = json.Unmarshal([]byte(attribution.String), &value); err != nil {
				return nil, err
			}
			item.Attribution = &value
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

package sources

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/gabehf/koito/internal/filters"
)

func playSchema() filters.Schema {
	s := filters.Schema{PairFields: true, Fields: map[string]filters.Type{}, Targets: map[string]filters.Type{"match.play": filters.Boolean}, Labels: map[string]string{"match.play": "Same play", "pair.exact": "Exact source duplicate", "pair.same_time_basis": "Comparable timestamp meaning", "pair.overlap": "Timestamp ranges overlap", "pair.gap": "Gap between timestamp ranges"}, Help: map[string]string{"match.play": "Treat these two records as one play when the condition is TRUE.", "pair.exact": "Both records share source, reported time, precision, and music identity.", "pair.gap": "Duration between non-overlapping timestamp ranges. Overlapping ranges have zero gap."}}
	base := namingSchema()
	for _, side := range []string{"left", "right"} {
		for field, kind := range base.Fields {
			s.Fields[side+"."+field] = kind
		}
		s.Fields[side+".time_basis"] = filters.Text
		s.Fields[side+".time_end"] = filters.Date
		s.Labels[side+".time_basis"] = strings.ToUpper(side[:1]) + side[1:] + " timestamp meaning"
		s.Labels[side+".time_end"] = strings.ToUpper(side[:1]) + side[1:] + " timestamp range end"
	}
	s.Fields["pair.exact"] = filters.Boolean
	s.Fields["pair.same_time_basis"] = filters.Boolean
	s.Fields["pair.overlap"] = filters.Boolean
	s.Fields["pair.gap"] = filters.Duration
	return s
}

func defaultPlayProgram(seconds int) *filters.Program {
	timing := fieldExpr("pair.overlap")
	if seconds > 0 {
		timing = operation("or", timing, operation("le", fieldExpr("pair.gap"), operation("seconds", literalExpr(seconds))))
	}
	value := operation("or", fieldExpr("pair.exact"), operation("and", fieldExpr("pair.same_time_basis"), timing))
	condition := literalExpr(true)
	return &filters.Program{Steps: []filters.Statement{{If: &condition, Then: []filters.Statement{{Set: "match.play", Value: &value}}}}}
}

func migratePlayPrograms(config RuleConfig) RuleConfig {
	config.Rules = append([]Rule(nil), config.Rules...)
	for i := range config.Rules {
		r := &config.Rules[i]
		if r.Type != "matching" || r.Subtype != "play" || r.Program != nil {
			continue
		}
		r.Program = defaultPlayProgram(r.Settings.ToleranceSeconds)
		r.Migration = migratedProgram(*r)
		if r.Name == "" {
			r.Name = "Play matching"
		}
	}
	return config
}

func preparePlayPrograms(config RuleConfig) error {
	for i := range config.Rules {
		r := &config.Rules[i]
		if r.Type != "matching" || r.Subtype != "play" || r.Program == nil {
			continue
		}
		compiled, err := filters.CompileProgram(*r.Program, playSchema())
		if err != nil {
			return fmt.Errorf("Filter %s: %w", r.ID, err)
		}
		r.compiled = compiled
	}
	return nil
}

func exactSourcePair(a, b matchingRecord) bool {
	if a.SourceID == b.SourceID && a.Values.EventID != "" && a.Values.EventID == b.Values.EventID {
		return true
	}
	if a.SourceID != b.SourceID || !a.Values.Time.Equal(b.Values.Time) || a.Values.Precision != b.Values.Precision {
		return false
	}
	if a.Values.URI != "" || b.Values.URI != "" {
		return a.Values.URI == b.Values.URI && a.Values.RecordingID == b.Values.RecordingID
	}
	return a.Values.Artist == b.Values.Artist && a.Values.Title == b.Values.Title && a.Values.Album == b.Values.Album && a.Values.RecordingID == b.Values.RecordingID
}

func pairEnvironment(a, b matchingRecord) filters.Env {
	if a.ID > b.ID {
		a, b = b, a
	}
	env := filters.Env{}
	for _, item := range []struct {
		side   string
		record matchingRecord
	}{{"left", a}, {"right", b}} {
		for field, value := range recordEnvironment(item.record) {
			if !strings.HasPrefix(field, "display.") {
				if value.Type == filters.Missing {
					value = filters.Absent(item.side + "." + field)
				}
				env[item.side+"."+field] = value
			}
		}
		env[item.side+".time_basis"] = filters.String(item.record.Basis)
		if !item.record.Start.IsZero() {
			env[item.side+".time_end"] = filters.Timestamp(item.record.Start.Add(time.Duration(item.record.Values.Precision)))
		}
	}
	aEnd, bEnd := a.Start.Add(time.Duration(a.Values.Precision)), b.Start.Add(time.Duration(b.Values.Precision))
	gap := time.Duration(0)
	if b.Start.After(aEnd) {
		gap = b.Start.Sub(aEnd)
	} else if a.Start.After(bEnd) {
		gap = a.Start.Sub(bEnd)
	}
	span, _ := filters.From(gap)
	env["pair.gap"] = span
	env["pair.exact"] = filters.Bool(exactSourcePair(a, b))
	env["pair.same_time_basis"] = filters.Bool(comparableTimeBasis(a.Basis, b.Basis))
	env["pair.overlap"] = filters.Bool(a.Start.Before(bEnd) && b.Start.Before(aEnd))
	return env
}

type pairResult struct {
	Match    bool
	Applied  bool
	Conflict bool
	Issues   []FilterIssue
}

type PairInspection struct {
	Left   int64         `json:"left"`
	Right  int64         `json:"right"`
	Music  bool          `json:"music"`
	Play   bool          `json:"play"`
	Issues []FilterIssue `json:"issues"`
}

func (s *Store) InspectPair(ctx context.Context, left, right int64) (PairInspection, error) {
	out := PairInspection{Left: left, Right: right, Issues: []FilterIssue{}}
	if left <= 0 || right <= 0 || left == right {
		return out, fmt.Errorf("Choose two different records.")
	}
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	version, err := readRuleVersion(ctx, tx, 0)
	if err != nil {
		return out, err
	}
	records, err := inspectedRecords(ctx, tx, version.Config, `WHERE r.id IN (?,?) ORDER BY r.id`, left, right)
	if err != nil {
		return out, err
	}
	if len(records) != 2 {
		return out, fmt.Errorf("Unknown record.")
	}
	if records[0].UserID != records[1].UserID {
		return out, fmt.Errorf("Records must belong to the same owner.")
	}
	music, _ := version.Config.matchingRules()
	a, b := records[0].matchingRecord, records[1].matchingRecord
	out.Left, out.Right = a.ID, b.ID
	out.Music = musicCompatible(a.Values, b.Values, music)
	result := resolvePlayPair(ctx, version.Config, a, b)
	out.Play = result.Match
	out.Issues = append(out.Issues, result.Issues...)
	return out, nil
}

func resolvePlayPair(ctx context.Context, config RuleConfig, a, b matchingRecord) pairResult {
	out := pairResult{Issues: []FilterIssue{}}
	best := -1
	var winner Rule
	var env filters.Env
	for _, original := range config.Rules {
		if !original.Enabled || original.Type != "matching" || original.Subtype != "play" {
			continue
		}
		candidate := original
		candidate.legacyComparison = legacyComparison(original)
		match := false
		applied := true
		if original.Program == nil {
			match = exactSourcePair(a, b) || timeCompatible(a, b, time.Duration(original.Settings.ToleranceSeconds)*time.Second)
		} else {
			compiled := original.compiled
			if compiled == nil {
				var err error
				compiled, err = filters.CompileProgram(*original.Program, playSchema())
				if err != nil {
					out.Issues = append(out.Issues, FilterIssue{original.ID, original.Name, filters.Issue{Path: "steps", Reason: err.Error()}})
					continue
				}
			}
			if env == nil {
				env = pairEnvironment(a, b)
			}
			result, err := compiled.Run(ctx, env)
			if err != nil {
				out.Issues = append(out.Issues, FilterIssue{original.ID, original.Name, filters.Issue{Path: "steps", Reason: err.Error()}})
				continue
			}
			for _, issue := range result.Issues {
				out.Issues = append(out.Issues, FilterIssue{original.ID, original.Name, issue})
			}
			if len(result.Issues) > 0 {
				match = false
			} else {
				value, exists := result.Values["match.play"]
				applied = exists
				if exists {
					match, _ = value.Bool()
				}
			}
		}
		if !applied {
			continue
		}
		rank := rulePriority(original)
		if rank > best {
			best = rank
			winner = candidate
			out.Match = match
			out.Applied = true
			out.Conflict = false
		} else if rank == best {
			different := out.Match != match
			if winner.legacyComparison != "" && candidate.legacyComparison != "" {
				different = winner.legacyComparison != candidate.legacyComparison
			}
			if different {
				out.Conflict = true
			}
			if candidate.ID < winner.ID {
				winner = candidate
				out.Match = match
			}
		}
	}
	if out.Conflict {
		out.Match = false
		out.Issues = append(out.Issues, FilterIssue{winner.ID, winner.Name, filters.Issue{Path: "match.play", Reason: "Equal-priority Play matching filters disagree."}})
	}
	return out
}

const unlimitedPairGap time.Duration = math.MaxInt64

func constantDuration(e filters.Expr) (time.Duration, bool) {
	x, err := filters.Compile(e, filters.Schema{})
	if err != nil {
		return 0, false
	}
	return x.Eval(context.Background(), nil).Span()
}

// This is only an index optimization. An unrecognized or unbounded expression
// compares the entire music block instead of silently omitting possible pairs.
func trueGapBound(e filters.Expr) time.Duration {
	if e.Op == "field" && (e.Field == "pair.overlap" || e.Field == "pair.exact") {
		return 0
	}
	if e.Op == "literal" {
		var value bool
		if json.Unmarshal(e.Value, &value) == nil && !value {
			return -1
		}
		return unlimitedPairGap
	}
	if (e.Op == "le" || e.Op == "lt") && len(e.Args) == 2 && e.Args[0].Op == "field" && e.Args[0].Field == "pair.gap" {
		if bound, ok := constantDuration(e.Args[1]); ok {
			return bound
		}
	}
	if e.Op == "and" {
		bound := unlimitedPairGap
		for _, child := range e.Args {
			if value := trueGapBound(child); value < bound {
				bound = value
			}
		}
		return bound
	}
	if e.Op == "or" {
		bound := time.Duration(-1)
		for _, child := range e.Args {
			if value := trueGapBound(child); value > bound {
				bound = value
			}
		}
		return bound
	}
	return unlimitedPairGap
}

func playSearchGap(config RuleConfig) time.Duration {
	maximum := time.Duration(0)
	for _, r := range config.Rules {
		if !r.Enabled || r.Type != "matching" || r.Subtype != "play" {
			continue
		}
		if r.Program == nil {
			if span := time.Duration(r.Settings.ToleranceSeconds) * time.Second; span > maximum {
				maximum = span
			}
			continue
		}
		var visit func([]filters.Statement, time.Duration)
		visit = func(steps []filters.Statement, condition time.Duration) {
			for _, step := range steps {
				if step.If != nil {
					bound := trueGapBound(*step.If)
					if condition < bound {
						bound = condition
					}
					visit(step.Then, bound)
					for _, branch := range step.ElseIf {
						bound := trueGapBound(branch.If)
						if condition < bound {
							bound = condition
						}
						visit(branch.Then, bound)
					}
					visit(step.Else, condition)
				} else if step.Set == "match.play" && step.Value != nil {
					bound := trueGapBound(*step.Value)
					if condition < bound {
						bound = condition
					}
					if bound > maximum {
						maximum = bound
					}
				}
			}
		}
		visit(r.Program.Steps, unlimitedPairGap)
	}
	return maximum
}

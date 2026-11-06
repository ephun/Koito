package sources

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gabehf/koito/internal/filters"
)

func heuristicSchema() filters.Schema {
	s := directSchema()
	s.EvidenceFields = true
	s.Targets["share.percent"] = filters.Number
	s.Bounds = aggregateSchema().Bounds
	return s
}

func referencedFields(program *filters.Program) []string {
	if program == nil {
		return nil
	}
	seen := map[string]bool{}
	var expression func(filters.Expr)
	expression = func(e filters.Expr) {
		if e.Op == "field" {
			seen[e.Field] = true
		}
		for _, child := range e.Args {
			expression(child)
		}
	}
	var statements func([]filters.Statement)
	statements = func(steps []filters.Statement) {
		for _, step := range steps {
			if step.If != nil {
				expression(*step.If)
			}
			if step.Value != nil {
				expression(*step.Value)
			}
			statements(step.Then)
			statements(step.Else)
			for _, branch := range step.ElseIf {
				expression(branch.If)
				statements(branch.Then)
			}
		}
	}
	statements(program.Steps)
	out := []string{}
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func heuristicCollection(rule Rule) filters.Expr {
	collection := fieldExpr("evidence:" + strconv.FormatInt(rule.Settings.EvidenceSourceID, 10))
	conditions := []filters.Expr{}
	for _, bound := range []struct{ value, op string }{{rule.Scope.From, "ge"}, {rule.Scope.Until, "le"}} {
		if bound.value != "" {
			conditions = append(conditions, operation(bound.op, operation("get", fieldExpr("item"), literalExpr("time")), operation("date", literalExpr(bound.value))))
		}
	}
	if len(conditions) == 1 {
		collection = operation("where", collection, conditions[0])
	} else if len(conditions) > 1 {
		collection = operation("where", collection, operation("and", conditions...))
	}
	return collection
}

func migrateHeuristicPrograms(config RuleConfig) RuleConfig {
	config.Rules = append([]Rule(nil), config.Rules...)
	for i := range config.Rules {
		rule := &config.Rules[i]
		if rule.Type != "attribution" || rule.Subtype != "heuristic" || rule.Program != nil {
			continue
		}
		condition := scopeExpression(rule.Scope)
		minimum := operation("ge", operation("count", heuristicCollection(*rule)), literalExpr(rule.Settings.MinimumMatches))
		value, target := literalExpr("self"), "listener"
		if rule.Scope.Service == "itunes" {
			target = "share.percent"
			if rule.Settings.SharePercent != nil {
				value = literalExpr(*rule.Settings.SharePercent)
			} else {
				value = fieldExpr("share.percent")
			}
		}
		unknown := literalExpr("unknown")
		priority := scopeRank(rule.Scope)
		rule.Priority = &priority
		rule.Program = &filters.Program{Steps: []filters.Statement{{If: &condition, Then: []filters.Statement{{If: &minimum, Then: []filters.Statement{{Set: target, Value: &value}}, Else: []filters.Statement{{Set: "listener", Value: &unknown}}}}}}}
		rule.Migration = migratedProgram(*rule)
		rule.Scope = RuleScope{}
		if rule.Name == "" {
			rule.Name = "Heuristic attribution"
		}
	}
	return config
}

func prepareHeuristicPrograms(config RuleConfig) error {
	for i := range config.Rules {
		rule := &config.Rules[i]
		if rule.Program == nil || rule.Type != "attribution" || rule.Subtype != "heuristic" {
			continue
		}
		compiled, err := filters.CompileProgram(*rule.Program, heuristicSchema())
		if err != nil {
			return fmt.Errorf("Filter %s: %w", rule.ID, err)
		}
		rule.compiled = compiled
		rule.evidenceFields = nil
		for _, field := range referencedFields(rule.Program) {
			if filters.ValidEvidenceField(field) {
				rule.evidenceFields = append(rule.evidenceFields, field)
			}
		}
	}
	return nil
}

func matchingPersonalPlays(index *evidenceIndex, r matchingRecord, source int64, config RuleConfig, from, until time.Time) []personalEvidence {
	out := []personalEvidence{}
	if index == nil {
		return out
	}
	music, _ := config.matchingRules()
	seen := map[string]bool{}
	for _, key := range evidenceMusicKeys(r.Values, music) {
		for _, item := range index.byMusic[key] {
			if seen[item.play.Key] || item.record.UserID != r.UserID || item.record.SourceID != source || !musicCompatible(r.Values, item.record.Values, music) {
				continue
			}
			if !from.IsZero() && item.record.Start.Before(from) || !until.IsZero() && item.record.Start.After(until) {
				continue
			}
			seen[item.play.Key] = true
			out = append(out, item)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].record.Start.Equal(out[j].record.Start) {
			return out[i].record.Start.Before(out[j].record.Start)
		}
		return out[i].play.Key < out[j].play.Key
	})
	return out
}

func heuristicEnvironment(rule Rule, r matchingRecord, config RuleConfig, index *evidenceIndex, from, until, evidenceFrom, evidenceUntil time.Time) filters.Env {
	env := attributionEnvironment(r, from, until)
	fields := rule.evidenceFields
	if fields == nil {
		for _, field := range referencedFields(rule.Program) {
			if filters.ValidEvidenceField(field) {
				fields = append(fields, field)
			}
		}
	}
	for _, field := range fields {
		if index == nil {
			env[field] = filters.Unknown("Personal evidence is awaiting Direct attribution.")
			continue
		}
		source, _ := strconv.ParseInt(strings.TrimPrefix(field, "evidence:"), 10, 64)
		if !index.sources[source] {
			env[field] = filters.Unknown("The personal evidence source is unavailable.")
			continue
		}
		items := []filters.Value{}
		for _, item := range matchingPersonalPlays(index, r, source, config, evidenceFrom, evidenceUntil) {
			value, _ := filters.From(map[string]any{"time": item.record.Start, "record_id": item.record.ID, "source_id": item.record.SourceID, "play_key": item.play.Key, "artist": item.record.Values.Artist, "album": item.record.Values.Album, "title": item.record.Values.Title, "device": item.record.Device})
			items = append(items, value)
		}
		env[field] = filters.Values(items...)
	}
	return env
}

func heuristicDomainIssue(r matchingRecord, result filters.Outcome) *filters.Issue {
	if r.Service != "itunes" {
		if _, exists := result.Values["share.percent"]; exists {
			return &filters.Issue{Path: "share.percent", Reason: "Percentages require cumulative listening counts."}
		}
	}
	return nil
}

func chooseHeuristicRule(config RuleConfig, r matchingRecord, from, until time.Time, index *evidenceIndex, evidenceFrom, evidenceUntil time.Time) (*Rule, bool) {
	var selected *Rule
	best, conflict := -1, false
	for _, original := range config.Rules {
		if !original.Enabled || original.Type != "attribution" || original.Subtype != "heuristic" {
			continue
		}
		candidate := original
		candidate.legacyComparison = legacyComparison(original)
		if candidate.Program == nil {
			if !scopeContains(candidate.Scope, r, from, until) {
				continue
			}
			candidate.Settings.Listener = "unknown"
			if index != nil {
				lower, upper := evidenceFrom, evidenceUntil
				if r.Service != "itunes" {
					if original.Scope.From != "" {
						lower, _ = time.Parse(time.RFC3339Nano, original.Scope.From)
					}
					if original.Scope.Until != "" {
						upper, _ = time.Parse(time.RFC3339Nano, original.Scope.Until)
					}
				}
				if evidenceMatches(index, r, candidate.Settings.EvidenceSourceID, candidate.Settings.MinimumMatches, config, lower, upper) {
					candidate.Settings.Listener = "self"
				}
			}
		} else {
			compiled := candidate.compiled
			if compiled == nil {
				var err error
				compiled, err = filters.CompileProgram(*candidate.Program, heuristicSchema())
				if err != nil {
					continue
				}
			}
			outcome, err := compiled.Run(context.Background(), heuristicEnvironment(candidate, r, config, index, from, until, evidenceFrom, evidenceUntil))
			if err != nil {
				continue
			}
			if issue := heuristicDomainIssue(r, outcome); issue != nil {
				outcome.Values = filters.Env{}
				outcome.Issues = append(outcome.Issues, *issue)
			}
			candidate.Settings.Listener, candidate.Settings.SharePercent = "unknown", nil
			if len(outcome.Issues) == 0 {
				listener, hasListener := outcome.Values["listener"]
				share, hasShare := outcome.Values["share.percent"]
				if !hasListener && !hasShare {
					continue
				}
				if hasListener {
					candidate.Settings.Listener, _ = listener.Text()
				}
				if hasShare {
					if n, valid := share.Int64(); valid && n >= 0 && n <= 100 {
						amount := int(n)
						candidate.Settings.SharePercent = &amount
						if !hasListener {
							candidate.Settings.Listener = "self"
						}
					}
				}
			}
		}
		rank := rulePriority(candidate)
		if rank > best {
			copy := candidate
			selected, best, conflict = &copy, rank, false
		} else if rank == best {
			if attributionConflict(*selected, candidate) {
				conflict = true
			}
			if candidate.ID < selected.ID {
				copy := candidate
				selected = &copy
			}
		}
	}
	return selected, conflict
}

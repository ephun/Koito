package sources

import (
	"context"
	"fmt"
	"time"

	"github.com/gabehf/koito/internal/filters"
)

func aggregateSchema() filters.Schema {
	s := directSchema()
	s.Choices = nil
	s.Targets = map[string]filters.Type{"share.percent": filters.Number}
	minimum, maximum := int64(0), int64(100)
	s.Bounds = map[string]filters.NumberBounds{"share.percent": {Min: &minimum, Max: &maximum, Whole: true}}
	return s
}

func migrateAggregatePrograms(config RuleConfig) RuleConfig {
	config.Rules = append([]Rule(nil), config.Rules...)
	for i := range config.Rules {
		r := &config.Rules[i]
		if r.Type != "attribution" || r.Subtype != "aggregate" || r.Program != nil {
			continue
		}
		condition := scopeExpression(r.Scope)
		var value filters.Expr
		if r.Settings.SharePercent == nil {
			value = fieldExpr("share.percent")
		} else {
			value = literalExpr(*r.Settings.SharePercent)
		}
		priority := scopeRank(r.Scope)
		r.Priority = &priority
		r.Program = &filters.Program{Steps: []filters.Statement{{If: &condition, Then: []filters.Statement{{Set: "share.percent", Value: &value}}}}}
		r.Migration = migratedProgram(*r)
		r.Scope = RuleScope{}
		if r.Name == "" {
			r.Name = "Aggregate attribution"
		}
	}
	return config
}

func prepareAggregatePrograms(config RuleConfig) error {
	for i := range config.Rules {
		r := &config.Rules[i]
		if r.Program == nil || r.Type != "attribution" || r.Subtype != "aggregate" {
			continue
		}
		program, err := filters.CompileProgram(*r.Program, aggregateSchema())
		if err != nil {
			return fmt.Errorf("Filter %s: %w", r.ID, err)
		}
		r.compiled = program
	}
	return nil
}

func attributionEnvironment(r matchingRecord, from, until time.Time) filters.Env {
	env := recordEnvironment(r)
	if !from.IsZero() {
		env["range.from"] = filters.Timestamp(from)
	}
	if !until.IsZero() {
		env["range.until"] = filters.Timestamp(until)
	}
	return env
}

func chooseAggregateRule(config RuleConfig, r matchingRecord, from, until time.Time) (*Rule, bool) {
	// Shares operate on the cumulative library evidence. Future source adapters
	// can supply that evidence without turning event counts into percentages.
	if r.Service != "itunes" {
		return nil, false
	}
	var selected *Rule
	best, conflict := -1, false
	for _, original := range config.Rules {
		if !original.Enabled || original.Type != "attribution" || original.Subtype != "aggregate" {
			continue
		}
		candidate := original
		candidate.legacyComparison = legacyComparison(original)
		if candidate.Program == nil {
			if !scopeContains(candidate.Scope, r, from, until) {
				continue
			}
		} else {
			program := candidate.compiled
			if program == nil {
				var err error
				program, err = filters.CompileProgram(*candidate.Program, aggregateSchema())
				if err != nil {
					continue
				}
			}
			outcome, err := program.Run(context.Background(), attributionEnvironment(r, from, until))
			if err != nil {
				continue
			}
			candidate.Settings.SharePercent = nil
			if len(outcome.Issues) == 0 {
				value, exists := outcome.Values["share.percent"]
				if !exists {
					continue
				}
				n, valid := value.Int64()
				if valid && n >= 0 && n <= 100 {
					share := int(n)
					candidate.Settings.SharePercent = &share
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

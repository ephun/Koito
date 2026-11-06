package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/gabehf/koito/internal/filters"
)

func repetitionSchema() filters.Schema {
	s := namingSchema()
	s.Fields["repeat.window"] = filters.Duration
	s.Fields["repeat.plays"] = filters.List
	s.Fields["play.time"] = filters.Date
	s.Fields["play.evidence_level"] = filters.Text
	s.Fields["play.attribution_basis"] = filters.Text
	s.Targets = map[string]filters.Type{"repeat.window": filters.Duration, "repetition.count": filters.Boolean}
	s.Labels["repeat.window"] = "Lookback duration"
	s.Labels["repeat.plays"] = "Previous plays of this music"
	s.Labels["repetition.count"] = "Count this play"
	s.Labels["play.time"] = "Play time"
	s.Labels["play.evidence_level"] = "Evidence level"
	s.Labels["play.attribution_basis"] = "Attribution basis"
	s.Help["repeat.window"] = "Set the lookback duration before reading previous plays."
	s.Help["repeat.plays"] = "Distinct earlier plays after music matching and attribution, including plays excluded by earlier repetition decisions."
	s.Help["repetition.count"] = "Choose whether this separate play contributes to the view."
	s.ItemFields = map[string]filters.Type{"item:/time": filters.Date, "item:/key": filters.Text, "item:/device": filters.Text, "item:/source_id": filters.Number, "item:/source_service": filters.Text, "item:/evidence_level": filters.Text, "item:/attribution_basis": filters.Text}
	s.ItemLabels = map[string]string{"item:/time": "Previous play time", "item:/key": "Previous play identifier", "item:/device": "Previous playback device", "item:/source_id": "Previous source identifier", "item:/source_service": "Previous source", "item:/evidence_level": "Previous evidence level", "item:/attribution_basis": "Previous attribution basis"}
	return s
}

func repetitionProgram() *filters.Program {
	window := operation("minutes", literalExpr(30))
	observed := operation("where", fieldExpr("repeat.plays"), operation("eq", fieldExpr("item:/evidence_level"), literalExpr("observed")))
	condition := operation("and", operation("eq", fieldExpr("play.evidence_level"), literalExpr("observed")), operation("ge", operation("count", observed), literalExpr(10)))
	program := assignWhen(condition, "repetition.count", literalExpr(false))
	program.Steps = append([]filters.Statement{{Set: "repeat.window", Value: &window}}, program.Steps...)
	return program
}

// Repetition evaluates distinct attributed plays after count inference has
// reserved observed listening. Excluded loops must not return as count estimates.
func applyRepetition(ctx context.Context, plan *matchPlan, config RuleConfig) error {
	type executable struct {
		rule     Rule
		compiled *filters.CompiledProgram
	}
	programs := []executable{}
	for _, rule := range config.Rules {
		if !rule.Enabled || rule.Type != "qualification" || rule.Subtype != "repetition" || rule.Program == nil {
			continue
		}
		compiled, err := filters.CompileProgram(*rule.Program, repetitionSchema())
		if err != nil {
			return fmt.Errorf("Filter %s: %w", rule.ID, err)
		}
		programs = append(programs, executable{rule, compiled})
	}
	if len(programs) == 0 {
		return nil
	}
	byID := recordIndex(plan.Records)
	positions := map[int64]int{}
	for i, r := range plan.Records {
		positions[r.ID] = i
	}
	ordered := append([]plannedPlay(nil), plan.Plays...)
	at := func(play plannedPlay) time.Time {
		if !play.At.IsZero() {
			return play.At
		}
		return byID[play.Representative].Start
	}
	sort.Slice(ordered, func(i, j int) bool {
		a, b := at(ordered[i]), at(ordered[j])
		if !a.Equal(b) {
			return a.Before(b)
		}
		return ordered[i].Key < ordered[j].Key
	})
	musicRule, _ := config.matchingRules()
	history := map[string][]plannedPlay{}
	excluded := map[string]bool{}
	for _, play := range ordered {
		if err := ctx.Err(); err != nil {
			return err
		}
		r := byID[play.Representative]
		identity := comparisonMusic(r.Values, musicRule)
		raw, _ := json.Marshal([]any{r.UserID, identity.Artist, identity.Title})
		group := string(raw)
		previous := history[group]
		currentTime := at(play)
		env := recordEnvironment(r)
		env["play.time"] = filters.Timestamp(currentTime)
		evidence := play.Evidence
		if evidence == "" {
			evidence = "observed"
		}
		env["play.evidence_level"] = filters.String(evidence)
		env["play.attribution_basis"] = filters.String(play.Attribution.Basis)
		env["repetition.count"] = filters.Bool(true)
		env["repeat.plays"] = filters.Deferred(func(_ context.Context, values filters.Env) filters.Value {
			window, known := values["repeat.window"].Span()
			if !known || window < 0 {
				return filters.Unknown("Set a nonnegative lookback duration before reading previous plays.")
			}
			from := currentTime.Add(-window)
			first := sort.Search(len(previous), func(i int) bool { return !at(previous[i]).Before(from) })
			items := make([]filters.Value, 0, len(previous)-first)
			for _, p := range previous[first:] {
				item := byID[p.Representative]
				evidence := p.Evidence
				if evidence == "" {
					evidence = "observed"
				}
				fields := map[string]any{"time": at(p), "key": p.Key, "source_id": item.SourceID, "source_service": item.Service, "evidence_level": evidence, "attribution_basis": p.Attribution.Basis}
				if item.Device != "" {
					fields["device"] = item.Device
				}
				value, err := filters.From(fields)
				if err != nil {
					return filters.Unknown(err.Error())
				}
				items = append(items, value)
			}
			return filters.Values(items...)
		})
		for _, program := range programs {
			result, err := program.compiled.Run(ctx, env)
			if err != nil {
				return err
			}
			for _, issue := range result.Issues {
				for _, id := range play.Records {
					i := positions[id]
					plan.Records[i].RepetitionIssues = append(plan.Records[i].RepetitionIssues, FilterIssue{program.rule.ID, program.rule.Name, issue})
				}
			}
			if len(result.Issues) > 0 {
				continue
			}
			for key, value := range result.Values {
				env[key] = value
			}
		}
		counted, _ := env["repetition.count"].Bool()
		if !counted {
			excluded[play.Key] = true
			for _, id := range play.Records {
				plan.Reasons[id] = "excluded by repetition"
			}
		}
		history[group] = append(previous, play)
	}
	kept := plan.Plays[:0]
	for _, play := range plan.Plays {
		if !excluded[play.Key] {
			kept = append(kept, play)
		}
	}
	plan.Plays = kept
	return nil
}

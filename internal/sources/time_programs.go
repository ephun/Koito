package sources

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/gabehf/koito/internal/filters"
)

func timeSchema() filters.Schema {
	s := directSchema()
	s.Fields["original.time_text"] = filters.Text
	s.Fields["record.played_ms"] = filters.Number
	s.Targets = map[string]filters.Type{"time.zone": filters.Text, "time.spotify_basis": filters.Text, "time.offset": filters.Duration, "time.last_played": filters.Boolean, "time.estimate_dates": filters.Boolean}
	s.Choices = map[string][]filters.Choice{"time.spotify_basis": {{Label: "Reported end", Value: "end"}, {Label: "Estimated start", Value: "estimated_start"}}}
	s.Labels = map[string]string{"original.time_text": "Original timestamp text", "record.played_ms": "Played milliseconds", "time.zone": "Last.fm timezone", "time.spotify_basis": "Spotify timestamps", "time.offset": "Time adjustment", "time.last_played": "Include last known play", "time.estimate_dates": "Estimate other play dates"}
	s.Help = map[string]string{"time.zone": "Interpret local Last.fm timestamps using this timezone.", "time.spotify_basis": "Estimate starts using the reported end and played duration.", "time.offset": "Add this duration to the interpreted play time.", "time.last_played": "Use the last-played date recorded in the iTunes library export.", "time.estimate_dates": "Estimate missing play dates within the time range supported by the iTunes library export."}
	s.Targets["time.meaning"] = filters.Text
	s.Choices["time.meaning"] = []filters.Choice{{Label: "Playback start", Value: "start"}, {Label: "Playback end", Value: "end"}, {Label: "Submission", Value: "submission"}, {Label: "Unknown", Value: "unknown"}}
	s.Labels["time.meaning"] = "Timestamp meaning"
	s.Help["time.meaning"] = "Choose what this timestamp represents before comparing it with another source."
	return s
}

func legacyTimeProgram(rule Rule) *filters.Program {
	condition := scopeExpression(rule.Scope)
	actions := []filters.Statement{}
	if rule.Scope.Service == "itunes" {
		last, estimate := literalExpr(rule.Settings.UseLastPlayed), literalExpr(rule.Settings.EstimateTimes)
		actions = append(actions, filters.Statement{Set: "time.last_played", Value: &last}, filters.Statement{Set: "time.estimate_dates", Value: &estimate})
	} else {
		zone := literalExpr(rule.Settings.Timezone)
		actions = append(actions, filters.Statement{Set: "time.zone", Value: &zone})
	}
	return &filters.Program{Steps: []filters.Statement{{If: &condition, Then: actions}}}
}

func spotifyTimeProgram(basis string) *filters.Program {
	condition := operation("eq", fieldExpr("source.service"), literalExpr("spotify"))
	value := literalExpr(basis)
	return &filters.Program{Steps: []filters.Statement{{If: &condition, Then: []filters.Statement{{Set: "time.spotify_basis", Value: &value}}}}}
}

func migrateTimePrograms(config RuleConfig) RuleConfig {
	config.Rules = append([]Rule(nil), config.Rules...)
	for i := range config.Rules {
		r := &config.Rules[i]
		if r.Type != "inference" || r.Subtype != "time" || r.Program != nil {
			continue
		}
		priority := scopeRank(r.Scope)
		r.Priority = &priority
		r.Program = legacyTimeProgram(*r)
		r.Migration = migratedProgram(*r)
		if r.Name == "" {
			if r.Scope.Service == "itunes" {
				r.Name = "iTunes play dates"
			} else {
				r.Name = "Last.fm timezone"
			}
		}
		r.Scope = RuleScope{}
	}
	ids := map[string]bool{}
	for _, r := range config.Rules {
		ids[r.ID] = true
	}
	basisDefined := false
	for _, r := range config.Rules {
		if r.Type == "inference" && r.Subtype == "time" && r.Program != nil && programTargets(r.Program)["time.spotify_basis"] {
			basisDefined = true
		}
	}
	for i := range config.Rules {
		r := &config.Rules[i]
		if r.Type != "matching" || r.Subtype != "play" || r.TimeMigrated {
			continue
		}
		basis := r.Settings.SpotifyTime
		if basis == "" {
			basis = "end"
		}
		r.TimeMigrated = true
		if basisDefined {
			break
		}
		id := "spotify-time"
		for suffix := 2; ids[id]; suffix++ {
			id = fmt.Sprintf("spotify-time-%d", suffix)
		}
		config.Rules = append(config.Rules, Rule{ID: id, Name: "Spotify timestamps", Type: "inference", Subtype: "time", Enabled: true, Program: spotifyTimeProgram(basis)})
		break
	}
	return config
}

func programTargets(program *filters.Program) map[string]bool {
	out := map[string]bool{}
	if program == nil {
		return out
	}
	var visit func([]filters.Statement)
	visit = func(steps []filters.Statement) {
		for _, step := range steps {
			if step.Set != "" {
				out[step.Set] = true
			}
			visit(step.Then)
			visit(step.Else)
			for _, branch := range step.ElseIf {
				visit(branch.Then)
			}
		}
	}
	visit(program.Steps)
	return out
}

func prepareTimePrograms(config RuleConfig) error {
	for i := range config.Rules {
		r := &config.Rules[i]
		if r.Type != "inference" || r.Subtype != "time" {
			continue
		}
		program := r.Program
		if program == nil {
			program = legacyTimeProgram(*r)
		}
		compiled, err := filters.CompileProgram(*program, timeSchema())
		if err != nil {
			return fmt.Errorf("Filter %s: %w", r.ID, err)
		}
		r.compiled = compiled
		r.timeTargets = programTargets(program)
	}
	return nil
}

type resolvedTimes struct {
	Values    filters.Env
	Rules     []string
	Issues    []FilterIssue
	Conflicts map[string]bool
}

func resolveTimePrograms(ctx context.Context, config RuleConfig, r matchingRecord, from, until time.Time, library bool) resolvedTimes {
	out := resolvedTimes{Values: filters.Env{}, Rules: []string{}, Issues: []FilterIssue{}, Conflicts: map[string]bool{}}
	owners := map[string]Rule{}
	ranks := map[string]int{}
	used := map[string]bool{}
	for _, original := range config.Rules {
		if !original.Enabled || original.Type != "inference" || original.Subtype != "time" {
			continue
		}
		program := original.Program
		if program == nil {
			program = legacyTimeProgram(original)
		}
		targets := original.timeTargets
		if targets == nil {
			targets = programTargets(program)
		}
		relevant := targets["time.zone"] || targets["time.spotify_basis"] || targets["time.offset"] || targets["time.meaning"]
		if library {
			relevant = targets["time.last_played"] || targets["time.estimate_dates"]
		}
		if !relevant {
			continue
		}
		compiled := original.compiled
		if compiled == nil {
			var err error
			compiled, err = filters.CompileProgram(*program, timeSchema())
			if err != nil {
				out.Issues = append(out.Issues, FilterIssue{original.ID, original.Name, filters.Issue{Path: "steps", Reason: err.Error()}})
				continue
			}
		}
		env := attributionEnvironment(r, from, until)
		env["original.time_text"] = filters.String(r.Values.TimeText)
		env["record.played_ms"] = filters.Integer(r.Values.PlayedMS)
		result, err := compiled.Run(ctx, env)
		// Retained service payloads use that service's existing time filters.
		// A filter that explicitly addresses the wrapper still evaluates first.
		if err == nil && len(result.Values) == 0 && len(result.Issues) == 0 && r.Values.OriginalFormat != "" && r.Values.OriginalService != "" && r.Values.OriginalService != r.Service {
			env["source.service"] = filters.String(r.Values.OriginalService)
			result, err = compiled.Run(ctx, env)
		}
		if err != nil {
			out.Issues = append(out.Issues, FilterIssue{original.ID, original.Name, filters.Issue{Path: "steps", Reason: err.Error()}})
			continue
		}
		if value, exists := result.Values["time.zone"]; exists {
			zone, _ := value.Text()
			if err := requireZone(zone); err != nil {
				result.Issues = append(result.Issues, filters.Issue{Path: "time.zone", Reason: "This timezone is unavailable."})
			}
		}
		if value, exists := result.Values["time.offset"]; exists {
			if _, valid := value.Span(); !valid {
				result.Issues = append(result.Issues, filters.Issue{Path: "time.offset", Reason: "This duration exceeds timestamp precision or range."})
			}
		}
		if len(result.Issues) > 0 {
			for _, issue := range result.Issues {
				out.Issues = append(out.Issues, FilterIssue{original.ID, original.Name, issue})
			}
			continue
		}
		candidate := original
		candidate.legacyComparison = legacyComparison(original)
		for field, value := range result.Values {
			isLibrary := field == "time.last_played" || field == "time.estimate_dates"
			if isLibrary != library {
				continue
			}
			rank := rulePriority(original)
			previous, exists := owners[field]
			if !exists || rank > ranks[field] {
				out.Values[field] = value
				owners[field] = candidate
				ranks[field] = rank
				out.Conflicts[field] = false
				continue
			}
			if rank == ranks[field] {
				left, _ := out.Values[field].MarshalJSON()
				right, _ := value.MarshalJSON()
				different := string(left) != string(right)
				if previous.legacyComparison != "" && candidate.legacyComparison != "" {
					different = previous.legacyComparison != candidate.legacyComparison
				}
				if different {
					out.Conflicts[field] = true
					out.Issues = append(out.Issues, FilterIssue{original.ID, original.Name, filters.Issue{Path: field, Reason: "Equal-priority Time filters disagree."}})
				}
				if original.ID < previous.ID {
					out.Values[field] = value
					owners[field] = candidate
				}
			}
		}
	}
	for field, rule := range owners {
		if out.Conflicts[field] {
			delete(out.Values, field)
		} else if !used[rule.ID] {
			used[rule.ID] = true
			out.Rules = append(out.Rules, rule.ID)
		}
	}
	sort.Strings(out.Rules)
	return out
}

func hasObservedTimePrograms(config RuleConfig) bool {
	for _, r := range config.Rules {
		if r.Type == "inference" && r.Subtype == "time" && r.Program != nil {
			targets := r.timeTargets
			if targets == nil {
				targets = programTargets(r.Program)
			}
			if targets["time.zone"] || targets["time.spotify_basis"] || targets["time.offset"] || targets["time.meaning"] {
				return true
			}
		}
	}
	return false
}

func applyObservedTime(r *matchingRecord, config RuleConfig) string {
	_, play := config.matchingRules()
	basis := play.Settings.SpotifyTime
	if play.TimeMigrated {
		basis = "end"
	}
	if !hasObservedTimePrograms(config) {
		return basis
	}
	from, until := recordInterval(*r)
	resolved := resolveTimePrograms(context.Background(), config, *r, from, until, false)
	r.TimeIssues = resolved.Issues
	if value, exists := resolved.Values["time.zone"]; exists {
		zone, _ := value.Text()
		r.Values = normalizeRecord([]byte(r.Payload), r.Parser, zone)
		r.Start = r.Values.Time
		r.Basis = r.Values.TimeMeaning
	}
	if value, exists := resolved.Values["time.spotify_basis"]; exists {
		basis, _ = value.Text()
	}
	if resolved.Conflicts["time.zone"] {
		r.Values = normalizeRecord([]byte(r.Payload), r.Parser, "")
		r.Start = r.Values.Time
		r.Basis = r.Values.TimeMeaning
	}
	if value, exists := resolved.Values["time.offset"]; exists {
		if span, valid := value.Span(); valid {
			r.timeOffset = &span
		}
	}
	if value, exists := resolved.Values["time.meaning"]; exists {
		if meaning, ok := value.Text(); ok {
			switch meaning {
			case "start", "end", "submission", "unknown":
				r.Basis = meaning
			}
		}
	}
	return basis
}

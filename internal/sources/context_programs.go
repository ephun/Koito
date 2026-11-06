package sources

import (
	"context"
	"fmt"

	"github.com/gabehf/koito/internal/filters"
)

func contextSchema() filters.Schema {
	s := namingSchema()
	s.Targets = map[string]filters.Type{"context.kind": filters.Text}
	s.Choices = map[string][]filters.Choice{"context.kind": {{Label: "Gameplay", Value: "gameplay"}, {Label: "Concert", Value: "concert"}, {Label: "Unspecified", Value: "unknown"}}}
	s.Labels["context.kind"] = "Listening context"
	s.Labels["record.source_input"] = "Named source input"
	s.Help["context.kind"] = "Identify listening context using the record's source information."
	return s
}

func prepareContextPrograms(config RuleConfig) error {
	for i := range config.Rules {
		r := &config.Rules[i]
		if r.Type != "inference" || r.Subtype != "context" || r.Program == nil {
			continue
		}
		var err error
		r.compiled, err = filters.CompileProgram(*r.Program, contextSchema())
		if err != nil {
			return fmt.Errorf("Filter %s: %w", r.ID, err)
		}
	}
	return nil
}

func applyContextPrograms(r *matchingRecord, config RuleConfig) {
	r.ContextIssues = nil
	var env filters.Env
	for _, rule := range config.Rules {
		if !rule.Enabled || rule.Type != "inference" || rule.Subtype != "context" || rule.Program == nil {
			continue
		}
		if env == nil {
			env = recordEnvironment(*r)
		}
		program := rule.compiled
		if program == nil {
			var err error
			program, err = filters.CompileProgram(*rule.Program, contextSchema())
			if err != nil {
				r.ContextIssues = append(r.ContextIssues, FilterIssue{rule.ID, rule.Name, filters.Issue{Path: "steps", Reason: err.Error()}})
				continue
			}
		}
		result, err := program.Run(context.Background(), env)
		if err != nil {
			r.ContextIssues = append(r.ContextIssues, FilterIssue{rule.ID, rule.Name, filters.Issue{Path: "steps", Reason: err.Error()}})
			continue
		}
		for _, issue := range result.Issues {
			r.ContextIssues = append(r.ContextIssues, FilterIssue{rule.ID, rule.Name, issue})
		}
		if len(result.Issues) != 0 {
			continue
		}
		if value, exists := result.Values["context.kind"]; exists {
			kind, valid := value.Text()
			if !valid || kind != "gameplay" && kind != "concert" && kind != "unknown" {
				r.ContextIssues = append(r.ContextIssues, FilterIssue{rule.ID, rule.Name, filters.Issue{Path: "context.kind", Reason: "Choose Gameplay, Concert, or Unspecified."}})
				continue
			}
			if r.Values.Performance == nil {
				r.Values.Performance = &performanceValues{Kind: kind}
			} else {
				r.Values.Performance.Kind = kind
			}
			env["event.kind"], env["context.kind"] = value, value
		}
	}
}

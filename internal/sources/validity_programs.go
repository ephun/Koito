package sources

import (
	"context"
	"fmt"

	"github.com/gabehf/koito/internal/filters"
)

func validitySchema() filters.Schema {
	s := namingSchema()
	s.Targets = map[string]filters.Type{"validity.count": filters.Boolean}
	s.Labels["validity.count"] = "Allow this record to contribute a play"
	s.Help["validity.count"] = "Choose which usable records can contribute plays."
	return s
}

func prepareValidityPrograms(config RuleConfig) error {
	for i := range config.Rules {
		rule := &config.Rules[i]
		if rule.Type != "qualification" || rule.Subtype != "validity" || rule.Program == nil {
			continue
		}
		compiled, err := filters.CompileProgram(*rule.Program, validitySchema())
		if err != nil {
			return fmt.Errorf("Filter %s: %w", rule.ID, err)
		}
		rule.compiled = compiled
	}
	return nil
}

func applyValidity(r *matchingRecord, config RuleConfig) {
	r.ValidityIssues = nil
	if r.Values.Reason != "counted" && r.Values.Reason != "not reported complete" && r.Values.Reason != "source duplicate" {
		return
	}
	env := recordEnvironment(*r)
	env["validity.count"] = filters.Bool(r.Values.Reason != "source duplicate")
	assigned := false
	for _, rule := range config.Rules {
		if !rule.Enabled || rule.Type != "qualification" || rule.Subtype != "validity" || rule.Program == nil {
			continue
		}
		compiled := rule.compiled
		if compiled == nil {
			var err error
			compiled, err = filters.CompileProgram(*rule.Program, validitySchema())
			if err != nil {
				r.ValidityIssues = append(r.ValidityIssues, FilterIssue{rule.ID, rule.Name, filters.Issue{Path: "steps", Reason: err.Error()}})
				continue
			}
		}
		result, err := compiled.Run(context.Background(), env)
		if err != nil {
			r.ValidityIssues = append(r.ValidityIssues, FilterIssue{rule.ID, rule.Name, filters.Issue{Path: "steps", Reason: err.Error()}})
			continue
		}
		for _, issue := range result.Issues {
			r.ValidityIssues = append(r.ValidityIssues, FilterIssue{rule.ID, rule.Name, issue})
		}
		if len(result.Issues) > 0 {
			continue
		}
		for name, value := range result.Values {
			env[name] = value
			if name == "validity.count" {
				assigned = true
			}
		}
	}
	if !assigned {
		return
	}
	allowed, _ := env["validity.count"].Bool()
	if !allowed {
		r.Values.Reason = "excluded by validity"
	} else if r.Values.Reason == "source duplicate" {
		r.Values.Reason = "counted"
	}
}

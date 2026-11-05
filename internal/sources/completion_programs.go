package sources

import (
	"context"
	"fmt"

	"github.com/gabehf/koito/internal/filters"
)

func completionSchema() filters.Schema {
	s := namingSchema()
	s.Targets = map[string]filters.Type{"completion.count": filters.Boolean}
	s.Labels["completion.count"] = "Count as a play"
	s.Help["completion.count"] = "Choose whether this music history record contributes a play."
	return s
}

func standardCompletionProgram() *filters.Program {
	condition := operation("eq", fieldExpr("source.format"), literalExpr(SpotifyStandard))
	value := operation("ge", fieldExpr("record.played_ms"), literalExpr(30000))
	return &filters.Program{Steps: []filters.Statement{{If: &condition, Then: []filters.Statement{{Set: "completion.count", Value: &value}}}}}
}

func extendedCompletionProgram() *filters.Program {
	condition := operation("eq", fieldExpr("source.format"), literalExpr(Parser))
	value := operation("and", operation("present", fieldExpr("record.completed")), fieldExpr("record.completed"))
	return &filters.Program{Steps: []filters.Statement{{If: &condition, Then: []filters.Statement{{Set: "completion.count", Value: &value}}}}}
}

func prepareCompletionPrograms(config RuleConfig) error {
	for i := range config.Rules {
		r := &config.Rules[i]
		if r.Type != "qualification" || r.Subtype != "completion" || r.Program == nil {
			continue
		}
		var err error
		r.compiled, err = filters.CompileProgram(*r.Program, completionSchema())
		if err != nil {
			return fmt.Errorf("Filter %s: %w", r.ID, err)
		}
	}
	return nil
}

func applyCompletion(r *matchingRecord, config RuleConfig) {
	r.CompletionIssues = nil
	// Qualification changes the counting decision for valid music history.
	// Structural errors and non-play evidence remain outside this domain.
	if r.Values.Reason != "counted" && r.Values.Reason != "not reported complete" {
		return
	}
	env := recordEnvironment(*r)
	env["completion.count"] = filters.Bool(r.Values.Reason == "counted")
	for _, rule := range config.Rules {
		if !rule.Enabled || rule.Type != "qualification" || rule.Subtype != "completion" || rule.Program == nil {
			continue
		}
		compiled := rule.compiled
		if compiled == nil {
			var err error
			compiled, err = filters.CompileProgram(*rule.Program, completionSchema())
			if err != nil {
				r.CompletionIssues = append(r.CompletionIssues, FilterIssue{rule.ID, rule.Name, filters.Issue{Path: "steps", Reason: err.Error()}})
				continue
			}
		}
		result, err := compiled.Run(context.Background(), env)
		if err != nil {
			r.CompletionIssues = append(r.CompletionIssues, FilterIssue{rule.ID, rule.Name, filters.Issue{Path: "steps", Reason: err.Error()}})
			continue
		}
		for _, issue := range result.Issues {
			r.CompletionIssues = append(r.CompletionIssues, FilterIssue{rule.ID, rule.Name, issue})
		}
		if len(result.Issues) != 0 {
			continue
		}
		if value, ok := result.Values["completion.count"]; ok {
			env["completion.count"] = value
			counted, _ := value.Bool()
			if counted {
				r.Values.Reason = "counted"
			} else {
				r.Values.Reason = "excluded by completion"
			}
		}
	}
}

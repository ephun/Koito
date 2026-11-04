package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/gabehf/koito/internal/filters"
)

func directSchema() filters.Schema {
	s := namingSchema()
	s.Fields["range.from"] = filters.Date
	s.Fields["range.until"] = filters.Date
	s.Targets = map[string]filters.Type{"listener": filters.Text}
	s.Choices = map[string][]filters.Choice{"listener": {{Label: "Me", Value: "self"}, {Label: "Someone else", Value: "other"}, {Label: "Unknown", Value: "unknown"}}}
	return s
}

func literalExpr(value any) filters.Expr {
	raw, _ := json.Marshal(value)
	return filters.Expr{Op: "literal", Value: raw}
}
func fieldExpr(name string) filters.Expr { return filters.Expr{Op: "field", Field: name} }
func operation(name string, args ...filters.Expr) filters.Expr {
	return filters.Expr{Op: name, Args: args}
}

func scopeExpression(s RuleScope) filters.Expr {
	conditions := []filters.Expr{}
	for _, item := range [][2]string{{"source.service", s.Service}, {"file.hash", s.FileHash}, {"library.id", s.LibraryID}, {"track.persistent_id", s.TrackID}, {"original.artist", s.Artist}, {"record.device", s.Device}} {
		if item[1] != "" {
			conditions = append(conditions, operation("and", operation("present", fieldExpr(item[0])), operation("eq", fieldExpr(item[0]), literalExpr(item[1]))))
		}
	}
	for _, item := range []struct {
		field string
		value int64
	}{{"source.id", s.SourceID}, {"record.id", s.RecordID}} {
		if item.value != 0 {
			conditions = append(conditions, operation("eq", fieldExpr(item.field), operation("number", literalExpr(strconv.FormatInt(item.value, 10)))))
		}
	}
	for _, item := range []struct{ field, value, op string }{{"range.from", s.From, "ge"}, {"range.until", s.Until, "le"}} {
		if item.value != "" {
			conditions = append(conditions, operation("and", operation("present", fieldExpr(item.field)), operation(item.op, fieldExpr(item.field), operation("date", literalExpr(item.value)))))
		}
	}
	if len(conditions) == 0 {
		return literalExpr(true)
	}
	if len(conditions) == 1 {
		return conditions[0]
	}
	return operation("and", conditions...)
}

// Migration happens in the returned edit configuration. Saved history stays
// immutable until the user saves. Existing precedence becomes explicit priority.
func migrateDirectPrograms(config RuleConfig) RuleConfig {
	config.Rules = append([]Rule(nil), config.Rules...)
	for i := range config.Rules {
		r := &config.Rules[i]
		if r.Type != "attribution" || r.Subtype != "direct" || r.Program != nil {
			continue
		}
		condition := scopeExpression(r.Scope)
		value := literalExpr(r.Settings.Listener)
		priority := scopeRank(r.Scope)
		r.Priority = &priority
		r.Program = &filters.Program{Steps: []filters.Statement{{If: &condition, Then: []filters.Statement{{Set: "listener", Value: &value}}}}}
		r.Scope = RuleScope{}
		if r.Name == "" {
			r.Name = "Direct attribution"
		}
	}
	return config
}

func prepareDirectPrograms(config RuleConfig) error {
	for i := range config.Rules {
		r := &config.Rules[i]
		if r.Program == nil || r.Type != "attribution" || r.Subtype != "direct" {
			continue
		}
		p, err := filters.CompileProgram(*r.Program, directSchema())
		if err != nil {
			return fmt.Errorf("Filter %s: %w", r.ID, err)
		}
		r.compiled = p
	}
	return nil
}

func rulePriority(r Rule) int {
	if r.Priority != nil {
		return *r.Priority
	}
	return scopeRank(r.Scope)
}

func chooseDirectRule(config RuleConfig, r matchingRecord, from, until time.Time) (*Rule, bool) {
	var selected *Rule
	best := -1
	conflict := false
	for _, original := range config.Rules {
		if !original.Enabled || original.Type != "attribution" || original.Subtype != "direct" {
			continue
		}
		candidate := original
		if candidate.Program == nil {
			if !scopeContains(candidate.Scope, r, from, until) {
				continue
			}
		} else {
			compiled := candidate.compiled
			if compiled == nil {
				var err error
				compiled, err = filters.CompileProgram(*candidate.Program, directSchema())
				if err != nil {
					continue
				}
			}
			env := recordEnvironment(r)
			if !from.IsZero() {
				env["range.from"] = filters.Timestamp(from)
			}
			if !until.IsZero() {
				env["range.until"] = filters.Timestamp(until)
			}
			result, err := compiled.Run(context.Background(), env)
			if err != nil {
				continue
			}
			if len(result.Issues) > 0 {
				candidate.Settings.Listener = "unknown"
			} else {
				value, exists := result.Values["listener"]
				if !exists {
					continue
				}
				listener, valid := value.Text()
				if !valid || listener != "self" && listener != "other" && listener != "unknown" {
					listener = "unknown"
				}
				candidate.Settings.Listener = listener
			}
		}
		rank := rulePriority(candidate)
		if rank > best {
			best = rank
			copy := candidate
			selected = &copy
			conflict = false
			continue
		}
		if rank == best {
			a, _ := json.Marshal(selected.Settings)
			b, _ := json.Marshal(candidate.Settings)
			if string(a) != string(b) {
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

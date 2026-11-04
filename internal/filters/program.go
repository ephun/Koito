package filters

import (
	"context"
	"fmt"
	"math/big"
)

// Statements contain either a conditional or one SET action. ELSE IF and ELSE
// branches can contain the same statements, including nested conditionals.
type Program struct {
	Steps []Statement `json:"steps"`
}
type Statement struct {
	If     *Expr       `json:"if,omitempty"`
	Then   []Statement `json:"then,omitempty"`
	ElseIf []Branch    `json:"else_if,omitempty"`
	Else   []Statement `json:"else,omitempty"`
	Set    string      `json:"set,omitempty"`
	Value  *Expr       `json:"value,omitempty"`
}
type Branch struct {
	If   Expr        `json:"if"`
	Then []Statement `json:"then"`
}
type compiledStatement struct {
	condition       *Expression
	then, otherwise []compiledStatement
	branches        []compiledBranch
	target          string
	expected        Type
	choices         []Choice
	bounds          *NumberBounds
	value           *Expression
}
type compiledBranch struct {
	condition *Expression
	steps     []compiledStatement
}
type CompiledProgram struct{ steps []compiledStatement }
type Issue struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}
type Outcome struct {
	Values Env     `json:"values"`
	Issues []Issue `json:"issues"`
}

func CompileProgram(p Program, s Schema) (*CompiledProgram, error) {
	n := 0
	steps, err := compileStatements(p.Steps, s, 0, &n)
	if err != nil {
		return nil, err
	}
	return &CompiledProgram{steps: steps}, nil
}

func compileStatements(steps []Statement, s Schema, depth int, n *int) ([]compiledStatement, error) {
	if depth > 64 {
		return nil, fmt.Errorf("Conditional nesting is too deep.")
	}
	var out []compiledStatement
	for i, step := range steps {
		*n++
		if *n > 4096 {
			return nil, fmt.Errorf("Filter is too complex.")
		}
		var c compiledStatement
		if step.If != nil {
			if step.Set != "" || step.Value != nil {
				return nil, fmt.Errorf("A block contains either IF or SET.")
			}
			x, err := compile(*step.If, s, depth, n)
			if err != nil {
				return nil, fmt.Errorf("IF %d: %w", i+1, err)
			}
			if x.kind != Boolean && x.kind != Any {
				return nil, fmt.Errorf("IF requires a Boolean condition.")
			}
			c.condition = x
			c.then, err = compileStatements(step.Then, s, depth+1, n)
			if err != nil {
				return nil, err
			}
			for _, branch := range step.ElseIf {
				x, err := compile(branch.If, s, depth+1, n)
				if err != nil {
					return nil, err
				}
				if x.kind != Boolean && x.kind != Any {
					return nil, fmt.Errorf("ELSE IF requires a Boolean condition.")
				}
				body, err := compileStatements(branch.Then, s, depth+1, n)
				if err != nil {
					return nil, err
				}
				c.branches = append(c.branches, compiledBranch{x, body})
			}
			c.otherwise, err = compileStatements(step.Else, s, depth+1, n)
			if err != nil {
				return nil, err
			}
		} else {
			if step.Set == "" || step.Value == nil || len(step.Then) > 0 || len(step.ElseIf) > 0 || len(step.Else) > 0 {
				return nil, fmt.Errorf("SET needs a target and a value.")
			}
			kind, ok := s.Targets[step.Set]
			if !ok {
				return nil, fmt.Errorf("Field %q cannot be changed here.", step.Set)
			}
			x, err := compile(*step.Value, s, depth, n)
			if err != nil {
				return nil, err
			}
			if x.kind != kind && x.kind != Any && kind != Any {
				return nil, fmt.Errorf("SET %s requires a %s value.", step.Set, kind)
			}
			c.target, c.value, c.expected = step.Set, x, kind
			c.choices = s.Choices[step.Set]
			if bounds, exists := s.Bounds[step.Set]; exists {
				if kind != Number {
					return nil, fmt.Errorf("Numeric bounds require a number target.")
				}
				if bounds.Min != nil && bounds.Max != nil && *bounds.Min > *bounds.Max {
					return nil, fmt.Errorf("Invalid numeric bounds.")
				}
				c.bounds = &bounds
				if x.op == "literal" && !withinBounds(x.value, c.bounds) {
					return nil, fmt.Errorf("SET %s requires a number within its allowed range.", step.Set)
				}
			}
			if x.op == "literal" && !allowedChoice(x.value, c.choices) {
				return nil, fmt.Errorf("SET %s requires an available choice.", step.Set)
			}
		}
		out = append(out, c)
	}
	return out, nil
}

// Run uses a private environment. Unknown conditions or action values discard
// this filter's entire result, including preceding SET actions. Later filters
// can still interpret the record independently.
func (p *CompiledProgram) Run(ctx context.Context, input Env) (Outcome, error) {
	env := make(Env, len(input))
	for key, value := range input {
		env[key] = value
	}
	changes := Env{}
	issue, err := runStatements(ctx, p.steps, env, changes, "steps")
	if err != nil {
		return Outcome{}, err
	}
	if issue != nil {
		return Outcome{Values: Env{}, Issues: []Issue{*issue}}, nil
	}
	return Outcome{Values: changes, Issues: []Issue{}}, nil
}

func runStatements(ctx context.Context, steps []compiledStatement, env, changes Env, path string) (*Issue, error) {
	for i, step := range steps {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		position := fmt.Sprintf("%s.%d", path, i)
		if step.condition != nil {
			value := step.condition.Eval(ctx, env)
			truth, known := value.Bool()
			if !known {
				return &Issue{position + ".if", reason(value)}, nil
			}
			body, label := step.otherwise, ".else"
			if truth {
				body, label = step.then, ".then"
			} else {
				for j, branch := range step.branches {
					value = branch.condition.Eval(ctx, env)
					truth, known = value.Bool()
					if !known {
						return &Issue{fmt.Sprintf("%s.else_if.%d", position, j), reason(value)}, nil
					}
					if truth {
						body, label = branch.steps, fmt.Sprintf(".else_if.%d.then", j)
						break
					}
				}
			}
			if issue, err := runStatements(ctx, body, env, changes, position+label); issue != nil || err != nil {
				return issue, err
			}
		} else {
			value := step.value.Eval(ctx, env)
			if !value.Known() {
				return &Issue{position + ".value", reason(value)}, nil
			}
			if step.expected != Any && value.Type != step.expected {
				return &Issue{position + ".value", "Value has an incompatible type."}, nil
			}
			if !allowedChoice(value, step.choices) {
				return &Issue{position + ".value", "Value is not an available choice."}, nil
			}
			if !withinBounds(value, step.bounds) {
				return &Issue{position + ".value", "Number is outside the allowed range or requires a whole value."}, nil
			}
			env[step.target] = value
			changes[step.target] = value
		}
	}
	return nil, nil
}

func withinBounds(value Value, bounds *NumberBounds) bool {
	if bounds == nil {
		return true
	}
	n, ok := value.data.(*big.Rat)
	if !ok || value.Type != Number || bounds.Whole && !n.IsInt() {
		return false
	}
	return (bounds.Min == nil || n.Cmp(big.NewRat(*bounds.Min, 1)) >= 0) && (bounds.Max == nil || n.Cmp(big.NewRat(*bounds.Max, 1)) <= 0)
}

func allowedChoice(value Value, choices []Choice) bool {
	if len(choices) == 0 {
		return true
	}
	text, ok := value.Text()
	if !ok {
		return false
	}
	for _, choice := range choices {
		if text == choice.Value {
			return true
		}
	}
	return false
}

func reason(v Value) string {
	if v.Reason != "" {
		return v.Reason
	}
	return "Condition or value is unknown."
}

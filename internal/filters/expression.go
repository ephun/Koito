package filters

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"time"
	"unicode"
)

type Expr struct {
	Op    string          `json:"op"`
	Field string          `json:"field,omitempty"`
	Value json.RawMessage `json:"value,omitempty"`
	Args  []Expr          `json:"args,omitempty"`
}

type Operator struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Min   int    `json:"min_args"`
	Max   int    `json:"max_args"`
}

var Operators = []Operator{
	{"and", "AND", 2, 64}, {"or", "OR", 2, 64}, {"not", "NOT", 1, 1}, {"nor", "NOR", 2, 64}, {"nand", "NAND", 2, 64}, {"xor", "XOR", 2, 2}, {"xnor", "XNOR", 2, 2},
	{"eq", "=", 2, 2}, {"ne", "!=", 2, 2}, {"lt", "<", 2, 2}, {"le", "<=", 2, 2}, {"gt", ">", 2, 2}, {"ge", ">=", 2, 2},
	{"missing", "IS MISSING", 1, 1}, {"present", "IS PRESENT", 1, 1}, {"null", "IS NULL", 1, 1}, {"empty", "IS EMPTY", 1, 1},
	{"contains", "CONTAINS", 2, 2}, {"starts", "STARTS WITH", 2, 2}, {"ends", "ENDS WITH", 2, 2}, {"in", "IN", 2, 2}, {"regex", "MATCHES REGEX", 2, 2},
	{"trim", "TRIM", 1, 1}, {"lower", "LOWERCASE", 1, 1}, {"upper", "UPPERCASE", 1, 1}, {"casefold", "NORMALIZE CASE", 1, 1},
	{"replace", "REPLACE", 3, 3}, {"regex_replace", "REGEX REPLACE", 3, 3}, {"join", "JOIN TEXT", 2, 64},
	{"add", "+", 2, 2}, {"subtract", "-", 2, 2}, {"multiply", "*", 2, 2}, {"divide", "/", 2, 2}, {"floor", "FLOOR", 1, 1}, {"ceil", "CEILING", 1, 1}, {"absolute", "ABSOLUTE", 1, 1},
	{"number", "TO NUMBER", 1, 1}, {"text", "TO TEXT", 1, 1}, {"date", "DATE", 1, 1}, {"milliseconds", "MILLISECONDS", 1, 1}, {"seconds", "SECONDS", 1, 1}, {"minutes", "MINUTES", 1, 1}, {"hours", "HOURS", 1, 1}, {"days", "DAYS", 1, 1},
	{"list", "LIST", 0, 256}, {"count", "COUNT", 1, 1}, {"sum", "SUM", 1, 1}, {"min", "MIN", 1, 1}, {"max", "MAX", 1, 1},
}

type Schema struct {
	Choices map[string][]Choice `json:"choices,omitempty"`
	Fields  map[string]Type     `json:"fields"`
	Targets map[string]Type     `json:"targets"`
}

type Choice struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type Expression struct {
	op      string
	field   string
	value   Value
	kind    Type
	args    []*Expression
	pattern *regexp.Regexp
}

func Compile(expr Expr, schema Schema) (*Expression, error) {
	n := 0
	return compile(expr, schema, 0, &n)
}

func compile(e Expr, s Schema, depth int, n *int) (*Expression, error) {
	*n++
	if depth > 64 || *n > 4096 {
		return nil, fmt.Errorf("Expression is too complex.")
	}
	x := &Expression{op: e.Op, field: e.Field}
	if e.Op == "field" {
		kind, ok := s.Fields[e.Field]
		if !ok {
			kind, ok = s.Targets[e.Field]
		}
		if !ok {
			return nil, fmt.Errorf("Unknown field %q.", e.Field)
		}
		if len(e.Args) > 0 || len(e.Value) > 0 {
			return nil, fmt.Errorf("Field blocks only select a field.")
		}
		x.kind = kind
		return x, nil
	}
	if e.Op == "literal" {
		if len(e.Args) > 0 || e.Field != "" {
			return nil, fmt.Errorf("Literal blocks only contain a value.")
		}
		v, err := FromJSON(e.Value)
		if err != nil {
			return nil, fmt.Errorf("Invalid literal: %w", err)
		}
		x.value = v
		x.kind = v.Type
		return x, nil
	}
	var operator *Operator
	for i := range Operators {
		if Operators[i].ID == e.Op {
			operator = &Operators[i]
			break
		}
	}
	if operator == nil {
		return nil, fmt.Errorf("Unknown operator %q.", e.Op)
	}
	if len(e.Value) > 0 || e.Field != "" {
		return nil, fmt.Errorf("Operator inputs belong in its argument blocks.")
	}
	if len(e.Args) < operator.Min || len(e.Args) > operator.Max {
		return nil, fmt.Errorf("%s needs %d–%d inputs.", operator.Label, operator.Min, operator.Max)
	}
	for _, a := range e.Args {
		child, err := compile(a, s, depth+1, n)
		if err != nil {
			return nil, err
		}
		x.args = append(x.args, child)
	}
	var err error
	x.kind, err = x.resultType()
	if err != nil {
		return nil, err
	}
	if (e.Op == "regex" || e.Op == "regex_replace") && x.args[1].op == "literal" {
		pattern, _ := x.args[1].value.Text()
		if len(pattern) > 4096 {
			return nil, fmt.Errorf("Regular expression is too long.")
		}
		x.pattern, err = regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("Invalid regular expression: %w", err)
		}
	}
	return x, nil
}

func (x *Expression) resultType() (Type, error) {
	accept := func(kinds ...Type) error {
		for _, a := range x.args {
			ok := a.kind == Any || a.kind == Missing
			for _, k := range kinds {
				ok = ok || a.kind == k
			}
			if !ok {
				return fmt.Errorf("%s cannot use %s inputs.", x.op, a.kind)
			}
		}
		return nil
	}
	switch x.op {
	case "and", "or", "not", "nor", "nand", "xor", "xnor":
		return Boolean, accept(Boolean)
	case "eq", "ne", "lt", "le", "gt", "ge":
		a, b := x.args[0].kind, x.args[1].kind
		if a != b && a != Any && b != Any && a != Missing && b != Missing {
			return "", fmt.Errorf("Comparison inputs must have compatible types.")
		}
		if x.op != "eq" && x.op != "ne" && a != Number && a != Date && a != Duration && a != Text && a != Any {
			return "", fmt.Errorf("This comparison requires text, numbers, dates, or durations.")
		}
		return Boolean, nil
	case "missing", "present", "null":
		return Boolean, nil
	case "empty":
		return Boolean, accept(Text, List)
	case "contains", "starts", "ends", "regex":
		return Boolean, accept(Text)
	case "in":
		if x.args[1].kind != List && x.args[1].kind != Any {
			return "", fmt.Errorf("IN needs a list as its second input.")
		}
		return Boolean, nil
	case "trim", "lower", "upper", "casefold", "replace", "regex_replace", "join":
		return Text, accept(Text)
	case "number":
		return Number, accept(Text, Number)
	case "text":
		return Text, accept(Text, Number, Boolean, Date, Duration)
	case "date":
		return Date, accept(Text, Date)
	case "milliseconds", "seconds", "minutes", "hours", "days":
		return Duration, accept(Number)
	case "list":
		return List, nil
	case "count":
		return Number, accept(List)
	case "sum", "min", "max":
		return Any, accept(List)
	case "floor", "ceil", "absolute":
		return Number, accept(Number)
	case "add", "subtract":
		a, b := x.args[0].kind, x.args[1].kind
		if a == Date && b == Duration {
			return Date, nil
		}
		if a == Date && b == Date && x.op == "subtract" {
			return Duration, nil
		}
		if a == b && (a == Number || a == Duration) {
			return a, nil
		}
		if a == Any || b == Any {
			return Any, nil
		}
		return "", fmt.Errorf("Arithmetic inputs need compatible types and units.")
	case "multiply":
		a, b := x.args[0].kind, x.args[1].kind
		if a == Duration && b == Number || a == Number && b == Duration {
			return Duration, nil
		}
		return Number, accept(Number)
	case "divide":
		a, b := x.args[0].kind, x.args[1].kind
		if a == Duration && b == Number {
			return Duration, nil
		}
		if a == Duration && b == Duration {
			return Number, nil
		}
		return Number, accept(Number)
	}
	return "", fmt.Errorf("Unsupported operator.")
}

func (x *Expression) Type() Type { return x.kind }

func (x *Expression) Eval(ctx context.Context, env Env) Value {
	if err := ctx.Err(); err != nil {
		return Unknown(err.Error())
	}
	if x.op == "literal" {
		return x.value
	}
	if x.op == "field" {
		if v, ok := env[x.field]; ok {
			return v
		}
		return Absent(x.field)
	}
	v := make([]Value, len(x.args))
	for i, a := range x.args {
		v[i] = a.Eval(ctx, env)
	}
	if x.op == "and" || x.op == "or" || x.op == "not" || x.op == "nor" || x.op == "nand" || x.op == "xor" || x.op == "xnor" {
		return logical(x.op, v)
	}
	if x.op == "missing" {
		if v[0].Type == Unresolved || v[0].Type == "" {
			return v[0]
		}
		return Bool(v[0].Type == Missing)
	}
	if x.op == "present" {
		if v[0].Type == Unresolved || v[0].Type == "" {
			return v[0]
		}
		return Bool(v[0].Known())
	}
	if x.op == "null" {
		return Bool(v[0].Type == Null)
	}
	if x.op == "list" {
		return Values(v...)
	}
	for _, a := range v {
		if !a.Known() {
			return a
		}
	}
	switch x.op {
	case "eq", "ne", "lt", "le", "gt", "ge":
		return compare(x.op, v[0], v[1])
	case "in":
		items, ok := v[1].Items()
		if !ok {
			return Unknown("IN requires a list.")
		}
		unresolved := Value{}
		for _, item := range items {
			r := compare("eq", v[0], item)
			if b, ok := r.Bool(); ok && b {
				return Bool(true)
			}
			if !r.Known() {
				unresolved = r
			}
		}
		if !unresolved.Known() && unresolved.Reason != "" {
			return unresolved
		}
		return Bool(false)
	case "empty":
		if text, ok := v[0].Text(); ok {
			return Bool(text == "")
		}
		if items, ok := v[0].Items(); ok {
			return Bool(len(items) == 0)
		}
	case "contains", "starts", "ends", "regex", "trim", "lower", "upper", "casefold", "replace", "regex_replace", "join":
		return x.textOperation(v)
	case "number":
		if v[0].Type == Number {
			return v[0]
		}
		s, ok := v[0].Text()
		if !ok {
			return Unknown("TO NUMBER requires text or a number.")
		}
		n, err := From(json.Number(s))
		if err != nil {
			return Unknown("Text is not a valid number.")
		}
		return n
	case "text":
		return stringify(v[0])
	case "date":
		if v[0].Type == Date {
			return v[0]
		}
		s, ok := v[0].Text()
		if !ok {
			return Unknown("DATE requires timestamp text.")
		}
		date, err := time.Parse(time.RFC3339Nano, s)
		if err != nil {
			return Unknown("Dates need an explicit timezone.")
		}
		return Timestamp(date)
	case "milliseconds", "seconds", "minutes", "hours", "days":
		n, ok := v[0].data.(*big.Rat)
		if !ok || v[0].Type != Number {
			return Unknown("A duration requires a number.")
		}
		factor := map[string]int64{"milliseconds": 1, "seconds": 1000, "minutes": 60000, "hours": 3600000, "days": 86400000}[x.op]
		return numeric(new(big.Rat).Mul(n, big.NewRat(factor, 1)), Duration)
	case "count", "sum", "min", "max":
		return aggregate(x.op, v[0])
	case "add", "subtract", "multiply", "divide", "floor", "ceil", "absolute":
		return arithmetic(x.op, v)
	}
	return Unknown("Operation received an incompatible value.")
}

func logical(op string, values []Value) Value {
	if op == "not" {
		b, ok := values[0].Bool()
		if !ok {
			return Unknown("Condition is unknown.")
		}
		return Bool(!b)
	}
	anyTrue, anyFalse, unknown := false, false, false
	for _, v := range values {
		b, ok := v.Bool()
		if !ok {
			unknown = true
		} else if b {
			anyTrue = true
		} else {
			anyFalse = true
		}
	}
	var result bool
	switch op {
	case "and", "nand":
		if anyFalse {
			result = false
		} else if unknown {
			return Unknown("Condition is unknown.")
		} else {
			result = true
		}
		if op == "nand" {
			result = !result
		}
	case "or", "nor":
		if anyTrue {
			result = true
		} else if unknown {
			return Unknown("Condition is unknown.")
		} else {
			result = false
		}
		if op == "nor" {
			result = !result
		}
	case "xor", "xnor":
		if unknown {
			return Unknown("Condition is unknown.")
		}
		result = anyTrue && anyFalse
		if op == "xnor" {
			result = !result
		}
	}
	return Bool(result)
}

func compare(op string, a, b Value) Value {
	if !a.Known() {
		return a
	}
	if !b.Known() {
		return b
	}
	if a.Type != b.Type {
		return Unknown("Comparison inputs have different types.")
	}
	order := 0
	switch a.Type {
	case Null:
	case Number, Duration:
		order = a.data.(*big.Rat).Cmp(b.data.(*big.Rat))
	case Text:
		order = strings.Compare(a.data.(string), b.data.(string))
	case Boolean:
		if a.data.(bool) != b.data.(bool) {
			order = 1
		}
	case Date:
		order = a.data.(time.Time).Compare(b.data.(time.Time))
	case List:
		left, _ := a.Items()
		right, _ := b.Items()
		if len(left) != len(right) {
			order = 1
		} else {
			for i := range left {
				r := compare("eq", left[i], right[i])
				equal, known := r.Bool()
				if !known {
					return r
				}
				if !equal {
					order = 1
					break
				}
			}
		}
	default:
		return Unknown("Values cannot be compared.")
	}
	result := map[string]bool{"eq": order == 0, "ne": order != 0, "lt": order < 0, "le": order <= 0, "gt": order > 0, "ge": order >= 0}[op]
	return Bool(result)
}

func (x *Expression) textOperation(v []Value) Value {
	texts := make([]string, len(v))
	for i, a := range v {
		s, ok := a.Text()
		if !ok {
			return Unknown("Text operation requires text inputs.")
		}
		texts[i] = s
	}
	s := texts[0]
	switch x.op {
	case "contains":
		return Bool(strings.Contains(s, texts[1]))
	case "starts":
		return Bool(strings.HasPrefix(s, texts[1]))
	case "ends":
		return Bool(strings.HasSuffix(s, texts[1]))
	case "trim":
		return String(strings.TrimSpace(s))
	case "lower":
		return String(strings.ToLower(s))
	case "upper":
		return String(strings.ToUpper(s))
	case "casefold":
		return String(strings.Map(func(r rune) rune {
			minimum := r
			for next := unicode.SimpleFold(r); next != r; next = unicode.SimpleFold(next) {
				if next < minimum {
					minimum = next
				}
			}
			return minimum
		}, s))
	case "join":
		return String(strings.Join(texts, ""))
	case "replace":
		return String(strings.ReplaceAll(s, texts[1], texts[2]))
	case "regex", "regex_replace":
		pattern := x.pattern
		if pattern == nil {
			if len(texts[1]) > 4096 {
				return Unknown("Regular expression is too long.")
			}
			var err error
			pattern, err = regexp.Compile(texts[1])
			if err != nil {
				return Unknown("Invalid regular expression.")
			}
		}
		if x.op == "regex" {
			return Bool(pattern.MatchString(s))
		}
		return String(pattern.ReplaceAllString(s, texts[2]))
	}
	return Unknown("Unsupported text operation.")
}

func stringify(v Value) Value {
	switch v.Type {
	case Text:
		return v
	case Number, Duration:
		return String(v.data.(*big.Rat).RatString())
	case Boolean:
		if v.data.(bool) {
			return String("true")
		}
		return String("false")
	case Date:
		return String(v.data.(time.Time).Format(time.RFC3339Nano))
	}
	return Unknown("Value cannot be converted to text.")
}

func arithmetic(op string, v []Value) Value {
	if v[0].Type == Date {
		a := v[0].data.(time.Time)
		if op == "subtract" && v[1].Type == Date {
			b := v[1].data.(time.Time)
			seconds := new(big.Int).Sub(big.NewInt(a.Unix()), big.NewInt(b.Unix()))
			nanos := new(big.Int).Mul(seconds, big.NewInt(int64(time.Second)))
			nanos.Add(nanos, big.NewInt(int64(a.Nanosecond()-b.Nanosecond())))
			return numeric(new(big.Rat).SetFrac(nanos, big.NewInt(int64(time.Millisecond))), Duration)
		}
		if v[1].Type == Duration && (op == "add" || op == "subtract") {
			nanos := new(big.Rat).Mul(v[1].data.(*big.Rat), big.NewRat(int64(time.Millisecond), 1))
			if !nanos.IsInt() || !nanos.Num().IsInt64() {
				return Unknown("Duration exceeds timestamp precision or range.")
			}
			d := nanos.Num().Int64()
			if op == "subtract" {
				if d == -1<<63 {
					return Unknown("Duration exceeds the supported range.")
				}
				d = -d
			}
			return Timestamp(a.Add(time.Duration(d)))
		}
		return Unknown("Timestamp arithmetic requires a duration.")
	}
	a, ok := v[0].data.(*big.Rat)
	if !ok {
		return Unknown("Arithmetic requires numbers or durations.")
	}
	if op == "floor" || op == "ceil" || op == "absolute" {
		if op == "absolute" {
			return numeric(new(big.Rat).Abs(a), Number)
		}
		q, r := new(big.Int), new(big.Int)
		q.QuoRem(a.Num(), a.Denom(), r)
		if r.Sign() != 0 && op == "floor" && a.Sign() < 0 {
			q.Sub(q, big.NewInt(1))
		}
		if r.Sign() != 0 && op == "ceil" && a.Sign() > 0 {
			q.Add(q, big.NewInt(1))
		}
		return numeric(new(big.Rat).SetInt(q), Number)
	}
	b, ok := v[1].data.(*big.Rat)
	if !ok {
		return Unknown("Arithmetic requires compatible inputs.")
	}
	kind := v[0].Type
	out := new(big.Rat)
	switch op {
	case "add", "subtract":
		if v[0].Type != v[1].Type {
			return Unknown("Arithmetic units differ.")
		}
		if op == "add" {
			out.Add(a, b)
		} else {
			out.Sub(a, b)
		}
	case "multiply":
		if v[0].Type == Duration && v[1].Type == Number || v[1].Type == Duration && v[0].Type == Number {
			kind = Duration
		} else if v[0].Type != Number || v[1].Type != Number {
			return Unknown("Multiplication requires a numeric factor.")
		}
		out.Mul(a, b)
	case "divide":
		if b.Sign() == 0 {
			return Unknown("Division by zero.")
		}
		if v[0].Type == v[1].Type {
			kind = Number
		} else if v[0].Type != Duration || v[1].Type != Number {
			return Unknown("Division units differ.")
		}
		out.Quo(a, b)
	}
	return numeric(out, kind)
}

func aggregate(op string, value Value) Value {
	items, ok := value.Items()
	if !ok {
		return Unknown("Aggregate requires a list.")
	}
	if op == "count" {
		return Integer(int64(len(items)))
	}
	if len(items) == 0 {
		if op == "sum" {
			return Integer(0)
		}
		return Unknown("The list has no values.")
	}
	kind := items[0].Type
	if kind != Number && kind != Duration {
		return Unknown("Aggregate requires numbers or durations.")
	}
	first, ok := items[0].data.(*big.Rat)
	if !ok {
		return Unknown("Aggregate value is unknown.")
	}
	result := new(big.Rat).Set(first)
	for _, item := range items[1:] {
		if !item.Known() {
			return item
		}
		if item.Type != kind {
			return Unknown("Aggregate units differ.")
		}
		n := item.data.(*big.Rat)
		switch op {
		case "sum":
			result.Add(result, n)
		case "min":
			if n.Cmp(result) < 0 {
				result.Set(n)
			}
		case "max":
			if n.Cmp(result) > 0 {
				result.Set(n)
			}
		}
	}
	return numeric(result, kind)
}

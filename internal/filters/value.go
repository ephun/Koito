// Package filters evaluates typed, declarative IF/THEN/ELSE programs.
package filters

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"strconv"
	"strings"
	"time"
)

type Type string

const (
	Any        Type = "any"
	Missing    Type = "missing"
	Unresolved Type = "unknown"
	Null       Type = "null"
	Boolean    Type = "boolean"
	Text       Type = "text"
	Number     Type = "number"
	Date       Type = "date"
	Duration   Type = "duration"
	List       Type = "list"
	Object     Type = "object"
)

// Value keeps absence, null, false, empty text, and zero distinct. Numbers use
// exact rational arithmetic, including counters larger than JavaScript integers.
type Value struct {
	Type   Type
	data   any
	Reason string
}

type Env map[string]Value

type computedValue func(context.Context, Env) Value

// Derived fields read the current program environment. They can reflect a
// collection selected by an earlier SET without repeating the program.
func Deferred(read func(context.Context, Env) Value) Value {
	return Value{Type: Any, data: computedValue(read)}
}

func (v Value) Lookup(key string) Value {
	object, ok := v.data.(map[string]Value)
	if !ok || v.Type != Object {
		return Unknown("An object is required.")
	}
	if value, exists := object[key]; exists {
		return value
	}
	return Absent(key)
}

func (v Value) Pointer(path string) Value {
	if !ValidRawField("raw:" + path) {
		return Unknown("Invalid field path.")
	}
	if path == "" {
		return v
	}
	for _, part := range strings.Split(strings.TrimPrefix(path, "/"), "/") {
		key := strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		if v.Type == Object {
			v = v.Lookup(key)
		} else if items, ok := v.Items(); ok {
			index, err := strconv.Atoi(key)
			if err != nil || index < 0 || index >= len(items) || strconv.Itoa(index) != key {
				return Absent(path)
			}
			v = items[index]
		} else if !v.Known() {
			return v
		} else {
			return Absent(path)
		}
	}
	return v
}

func Unknown(reason string) Value { return Value{Type: Unresolved, Reason: reason} }
func Absent(field string) Value   { return Value{Type: Missing, Reason: "Missing field: " + field} }
func Bool(v bool) Value           { return Value{Type: Boolean, data: v} }
func String(v string) Value       { return Value{Type: Text, data: v} }
func Integer(v int64) Value       { return Value{Type: Number, data: big.NewRat(v, 1)} }
func Timestamp(v time.Time) Value { return Value{Type: Date, data: v} }
func Milliseconds(v int64) Value  { return Value{Type: Duration, data: big.NewRat(v, 1)} }
func Values(v ...Value) Value     { return Value{Type: List, data: v} }

func (v Value) Known() bool             { return v.Type != "" && v.Type != Missing && v.Type != Unresolved }
func (v Value) Bool() (bool, bool)      { b, ok := v.data.(bool); return b, ok && v.Type == Boolean }
func (v Value) Text() (string, bool)    { s, ok := v.data.(string); return s, ok && v.Type == Text }
func (v Value) Time() (time.Time, bool) { t, ok := v.data.(time.Time); return t, ok && v.Type == Date }
func (v Value) Items() ([]Value, bool) {
	items, ok := v.data.([]Value)
	return items, ok && v.Type == List
}
func (v Value) Int64() (int64, bool) {
	n, ok := v.data.(*big.Rat)
	if !ok || !n.IsInt() || !n.Num().IsInt64() {
		return 0, false
	}
	return n.Num().Int64(), true
}

func (v Value) Span() (time.Duration, bool) {
	if v.Type != Duration {
		return 0, false
	}
	number, ok := v.data.(*big.Rat)
	if !ok {
		return 0, false
	}
	nanos := new(big.Rat).Mul(number, big.NewRat(int64(time.Millisecond), 1))
	if !nanos.IsInt() || !nanos.Num().IsInt64() {
		return 0, false
	}
	return time.Duration(nanos.Num().Int64()), true
}

func numeric(n *big.Rat, kind Type) Value {
	if n.Num().BitLen() > 4096 || n.Denom().BitLen() > 4096 {
		return Unknown("Numeric result exceeds the supported range.")
	}
	return Value{Type: kind, data: n}
}

func FromJSON(raw json.RawMessage) (Value, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var v any
	if err := decoder.Decode(&v); err != nil {
		return Value{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return Value{}, fmt.Errorf("Literal must contain one JSON value.")
	}
	return From(v)
}

func From(v any) (Value, error) {
	switch x := v.(type) {
	case Value:
		return x, nil
	case nil:
		return Value{Type: Null}, nil
	case bool:
		return Bool(x), nil
	case string:
		return String(x), nil
	case int:
		return Integer(int64(x)), nil
	case int32:
		return Integer(int64(x)), nil
	case int64:
		return Integer(x), nil
	case float64:
		return From(json.Number(strconv.FormatFloat(x, 'g', -1, 64)))
	case json.Number:
		if len(x) > 256 {
			return Value{}, fmt.Errorf("Number is too long.")
		}
		// Bound exponents before constructing a potentially enormous integer.
		if f, _, err := big.ParseFloat(string(x), 10, 256, big.ToNearestEven); err != nil || f.IsInf() || f.MantExp(nil) > 4096 || f.MantExp(nil) < -4096 {
			return Value{}, fmt.Errorf("Invalid or excessive number.")
		}
		n, ok := new(big.Rat).SetString(string(x))
		if !ok {
			return Value{}, fmt.Errorf("Invalid number.")
		}
		out := numeric(n, Number)
		if !out.Known() {
			return Value{}, fmt.Errorf("%s", out.Reason)
		}
		return out, nil
	case time.Time:
		return Timestamp(x), nil
	case time.Duration:
		return numeric(new(big.Rat).SetFrac(big.NewInt(int64(x)), big.NewInt(int64(time.Millisecond))), Duration), nil
	case []Value:
		return Values(x...), nil
	case []any:
		items := make([]Value, len(x))
		for i, item := range x {
			converted, err := From(item)
			if err != nil {
				return Value{}, err
			}
			items[i] = converted
		}
		return Values(items...), nil
	case map[string]any:
		fields := map[string]Value{}
		for key, item := range x {
			converted, err := From(item)
			if err != nil {
				return Value{}, err
			}
			fields[key] = converted
		}
		return Value{Type: Object, data: fields}, nil
	default:
		return Value{}, fmt.Errorf("Unsupported value type %T.", v)
	}
}

// JSON uses decimal strings for exact numbers, preserving large counters when
// a browser inspects evaluated values. Expression literals retain their JSON.
func (v Value) MarshalJSON() ([]byte, error) {
	var data any = v.data
	if n, ok := data.(*big.Rat); ok {
		data = n.RatString()
	}
	return json.Marshal(struct {
		Type   Type   `json:"type"`
		Value  any    `json:"value"`
		Reason string `json:"reason,omitempty"`
	}{v.Type, data, v.Reason})
}

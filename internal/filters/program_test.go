package filters

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func literal(v any) Expr                { b, _ := json.Marshal(v); return Expr{Op: "literal", Value: b} }
func field(name string) Expr            { return Expr{Op: "field", Field: name} }
func op(name string, args ...Expr) Expr { return Expr{Op: name, Args: args} }
func pointer(e Expr) *Expr              { return &e }
func set(name string, e Expr) Statement { return Statement{Set: name, Value: &e} }

func evaluate(t *testing.T, e Expr, env Env) Value {
	t.Helper()
	fields := map[string]Type{}
	for k, v := range env {
		fields[k] = v.Type
	}
	fields["absent"] = Text
	x, err := Compile(e, Schema{Fields: fields})
	require.NoError(t, err)
	return x.Eval(context.Background(), env)
}

func TestLogicTruthTables(t *testing.T) {
	for _, a := range []bool{false, true} {
		for _, b := range []bool{false, true} {
			for name, want := range map[string]bool{"and": a && b, "or": a || b, "nor": !(a || b), "nand": !(a && b), "xor": a != b, "xnor": a == b} {
				actual, known := evaluate(t, op(name, literal(a), literal(b)), nil).Bool()
				require.True(t, known)
				require.Equal(t, want, actual, name)
			}
		}
	}
	for _, test := range []struct {
		e           Expr
		known, want bool
	}{
		{op("or", literal(true), op("eq", field("absent"), literal("x"))), true, true},
		{op("and", literal(false), op("eq", field("absent"), literal("x"))), true, false},
		{op("not", op("eq", field("absent"), literal("x"))), false, false},
		{op("nor", literal(false), op("eq", field("absent"), literal("x"))), false, false},
	} {
		value, known := evaluate(t, test.e, nil).Bool()
		require.Equal(t, test.known, known)
		if known {
			require.Equal(t, test.want, value)
		}
	}
}

func TestMissingNullEmptyAndZeroRemainDistinct(t *testing.T) {
	null, _ := From(nil)
	env := Env{"null": null, "empty": String(""), "zero": Integer(0), "false": Bool(false), "invalid": Unknown("Invalid value.")}
	for key := range env {
		b, known := evaluate(t, op("missing", field(key)), env).Bool()
		if key == "invalid" {
			require.False(t, known)
		} else {
			require.True(t, known)
			require.False(t, b)
		}
	}
	b, known := evaluate(t, op("missing", field("absent")), env).Bool()
	require.True(t, known)
	require.True(t, b)
	b, known = evaluate(t, op("empty", field("empty")), env).Bool()
	require.True(t, known)
	require.True(t, b)
	b, known = evaluate(t, op("null", field("null")), env).Bool()
	require.True(t, known)
	require.True(t, b)
}

func TestExactNumbersDurationsAndText(t *testing.T) {
	x := op("subtract", Expr{Op: "literal", Value: json.RawMessage("9007199254740993")}, Expr{Op: "literal", Value: json.RawMessage("9007199254740992")})
	n, ok := evaluate(t, x, nil).Int64()
	require.True(t, ok)
	require.EqualValues(t, 1, n)
	n, ok = evaluate(t, op("multiply", op("divide", literal(1), literal(3)), literal(3)), nil).Int64()
	require.True(t, ok)
	require.EqualValues(t, 1, n)
	n, ok = evaluate(t, op("floor", op("divide", literal(-7), literal(3))), nil).Int64()
	require.True(t, ok)
	require.EqualValues(t, -3, n)
	date := op("add", op("date", literal("2020-01-01T00:00:00Z")), op("seconds", literal(90)))
	got, ok := evaluate(t, date, nil).Time()
	require.True(t, ok)
	require.Equal(t, "2020-01-01T00:01:30Z", got.Format(time.RFC3339))
	b, ok := evaluate(t, op("eq", op("casefold", literal("Σ")), op("casefold", literal("ς"))), nil).Bool()
	require.True(t, ok)
	require.True(t, b)
	text, ok := evaluate(t, op("regex_replace", literal("Album (Deluxe)"), literal(` \(Deluxe\)$`), literal("")), nil).Text()
	require.True(t, ok)
	require.Equal(t, "Album", text)
}

func TestProgramsBranchAndApplyActionsAtomically(t *testing.T) {
	schema := Schema{Fields: map[string]Type{"album": Text, "source": Text}, Targets: map[string]Type{"display.album": Text, "listener": Text}}
	p := Program{Steps: []Statement{{If: pointer(op("missing", field("album"))), Then: []Statement{set("display.album", literal("Unspecified"))}, ElseIf: []Branch{{If: op("eq", field("source"), literal("spotify")), Then: []Statement{set("display.album", op("trim", field("album")))}}}, Else: []Statement{set("display.album", field("album"))}}}}
	compiled, err := CompileProgram(p, schema)
	require.NoError(t, err)
	for _, test := range []struct {
		input Env
		want  string
	}{{Env{}, "Unspecified"}, {Env{"source": String("spotify"), "album": String(" Album ")}, "Album"}, {Env{"source": String("lastfm"), "album": String(" Album ")}, " Album "}} {
		result, err := compiled.Run(context.Background(), test.input)
		require.NoError(t, err)
		require.Empty(t, result.Issues)
		got, _ := result.Values["display.album"].Text()
		require.Equal(t, test.want, got)
	}
	p = Program{Steps: []Statement{set("listener", literal("self")), {If: pointer(op("eq", field("album"), literal("Album"))), Then: []Statement{set("listener", literal("self"))}, Else: []Statement{set("listener", literal("other"))}}}}
	compiled, err = CompileProgram(p, schema)
	require.NoError(t, err)
	input := Env{"listener": String("unknown")}
	result, err := compiled.Run(context.Background(), input)
	require.NoError(t, err)
	require.Empty(t, result.Values)
	require.Len(t, result.Issues, 1)
	original, _ := input["listener"].Text()
	require.Equal(t, "unknown", original)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = compiled.Run(ctx, input)
	require.ErrorIs(t, err, context.Canceled)
}

func TestCompileRejectsInvalidPrograms(t *testing.T) {
	s := Schema{Fields: map[string]Type{"album": Text}, Targets: map[string]Type{"display.album": Text}}
	for _, e := range []Expr{field("unknown"), op("add", literal("x"), literal(1)), op("regex", field("album"), literal("[")), op("xor", literal(true)), op("and", literal(1), literal(true))} {
		_, err := Compile(e, s)
		require.Error(t, err)
	}
	for _, p := range []Program{{Steps: []Statement{set("album", literal("changed"))}}, {Steps: []Statement{set("display.album", literal(1))}}, {Steps: []Statement{{If: pointer(literal("yes")), Then: []Statement{}}}}} {
		_, err := CompileProgram(p, s)
		require.Error(t, err)
	}
}

func TestDynamicTargetsAndTimestampPrecision(t *testing.T) {
	s := Schema{Fields: map[string]Type{"raw": Any}, Targets: map[string]Type{"display.title": Text}}
	p, err := CompileProgram(Program{Steps: []Statement{set("display.title", field("raw"))}}, s)
	require.NoError(t, err)
	result, err := p.Run(context.Background(), Env{"raw": Integer(12)})
	require.NoError(t, err)
	require.Empty(t, result.Values)
	require.Len(t, result.Issues, 1)
	n, ok := evaluate(t, op("divide", op("subtract", op("date", literal("2020-01-01T00:00:00.000000002Z")), op("date", literal("2020-01-01T00:00:00.000000001Z"))), op("milliseconds", literal(0.000001))), nil).Int64()
	require.True(t, ok)
	require.EqualValues(t, 1, n)
	_, err = FromJSON(json.RawMessage("true false"))
	require.Error(t, err)
}

package filters

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRawFieldReferencesRequireExplicitSchemaSupport(t *testing.T) {
	for _, name := range []string{"raw:", "raw:/Genre", "raw:/a~1b/~0value", "column:/Artist", "column:/a~1b", "column:/"} {
		require.True(t, ValidRawField(name), name)
		_, err := Compile(field(name), Schema{RawFields: true})
		require.NoError(t, err)
		_, err = Compile(field(name), Schema{})
		require.Error(t, err)
	}
	for _, name := range []string{"Genre", "raw:Genre", "raw:/bad~", "raw:/bad~2", "column:", "column:/a/b"} {
		require.False(t, ValidRawField(name), name)
		_, err := Compile(field(name), Schema{RawFields: true})
		require.Error(t, err)
	}
}

func TestObjectAndListAccessPreserveTypesAndExactNumbers(t *testing.T) {
	root, err := FromJSON(json.RawMessage(`{"items":[{"count":9007199254740993,"genre":"Jazz"}],"empty":null}`))
	require.NoError(t, err)
	env := Env{"root": root}
	item := op("get", op("get", field("root"), literal("items")), literal(0))
	number := op("get", item, literal("count"))
	actual, known := evaluate(t, op("subtract", number, Expr{Op: "literal", Value: json.RawMessage(`9007199254740992`)}), env).Int64()
	require.True(t, known)
	require.EqualValues(t, 1, actual)
	genre, known := evaluate(t, op("get", item, literal("genre")), env).Text()
	require.True(t, known)
	require.Equal(t, "Jazz", genre)
	keys, ok := evaluate(t, op("keys", item), env).Items()
	require.True(t, ok)
	require.Equal(t, []Value{String("count"), String("genre")}, keys)
	require.Equal(t, Missing, evaluate(t, op("get", item, literal("missing")), env).Type)
	require.Equal(t, Null, evaluate(t, op("get", field("root"), literal("empty")), env).Type)
	require.Equal(t, Missing, evaluate(t, op("get", op("get", field("root"), literal("items")), literal(-1)), env).Type)
	require.Equal(t, Unresolved, evaluate(t, op("get", op("get", field("root"), literal("items")), literal("zero")), env).Type)
	copy, err := FromJSON(json.RawMessage(`{"empty":null,"items":[{"genre":"Jazz","count":9007199254740993}]}`))
	require.NoError(t, err)
	env["copy"] = copy
	equal, known := evaluate(t, op("eq", field("root"), field("copy")), env).Bool()
	require.True(t, known)
	require.True(t, equal)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	expr, err := Compile(op("keys", field("root")), Schema{Fields: map[string]Type{"root": Object}})
	require.NoError(t, err)
	require.Equal(t, Unresolved, expr.Eval(ctx, env).Type)
}

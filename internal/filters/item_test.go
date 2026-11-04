package filters

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestItemPathsAreLexicalAndPreserveNestedValues(t *testing.T) {
	rows, err := From([]any{map[string]any{"raw": map[string]any{"a/b": map[string]any{"~genre": "Jazz"}}, "count": 7}, map[string]any{"count": 0}})
	require.NoError(t, err)
	s := Schema{Fields: map[string]Type{"rows": List}}
	predicate := op("and", op("present", field("item:/raw/a~1b/~0genre")), op("eq", field("item:/raw/a~1b/~0genre"), literal("Jazz")))
	expr, err := Compile(op("sum", op("map", op("where", field("rows"), predicate), field("item:/count"))), s)
	require.NoError(t, err)
	n, known := expr.Eval(context.Background(), Env{"rows": rows}).Int64()
	require.True(t, known)
	require.EqualValues(t, 7, n)
	_, err = Compile(field("item:/count"), s)
	require.Error(t, err)
	_, err = Compile(op("where", field("rows"), op("present", field("item:/invalid~2"))), s)
	require.Error(t, err)
	list := Values(Integer(0), Bool(false))
	require.Equal(t, Missing, list.Pointer("/01").Type)
	require.Equal(t, Unresolved, list.Pointer("relative").Type)
}

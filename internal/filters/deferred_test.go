package filters

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDerivedFieldsReadEarlierAssignmentsWithoutChangingInput(t *testing.T) {
	input := Env{"selected": Values(Integer(1)), "count": Deferred(func(ctx context.Context, env Env) Value {
		items, valid := env["selected"].Items()
		if !valid {
			return Unknown("A list is required.")
		}
		return Integer(int64(len(items)))
	})}
	s := Schema{Fields: map[string]Type{"count": Number}, Targets: map[string]Type{"selected": List, "total": Number}}
	p, err := CompileProgram(Program{Steps: []Statement{set("selected", op("list", literal(1), literal(2), literal(3))), set("total", field("count"))}}, s)
	require.NoError(t, err)
	result, err := p.Run(context.Background(), input)
	require.NoError(t, err)
	require.Empty(t, result.Issues)
	n, valid := result.Values["total"].Int64()
	require.True(t, valid)
	require.EqualValues(t, 3, n)
	unchanged, valid := input["selected"].Items()
	require.True(t, valid)
	require.Len(t, unchanged, 1)
}

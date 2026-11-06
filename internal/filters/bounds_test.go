package filters

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNumericTargetBoundsValidateConstantsAndDynamicValuesAtomically(t *testing.T) {
	minimum, maximum := int64(0), int64(100)
	s := Schema{Fields: map[string]Type{"input": Any}, Targets: map[string]Type{"share": Number, "name": Text}, Bounds: map[string]NumberBounds{"share": {Min: &minimum, Max: &maximum, Whole: true}}}
	for _, input := range []any{-1, 101, 50.5} {
		_, err := CompileProgram(Program{Steps: []Statement{set("share", literal(input))}}, s)
		require.ErrorContains(t, err, "allowed range")
	}
	for _, input := range []any{0, 50, 100} {
		_, err := CompileProgram(Program{Steps: []Statement{set("share", literal(input))}}, s)
		require.NoError(t, err)
	}
	p, err := CompileProgram(Program{Steps: []Statement{set("name", literal("Changed")), set("share", field("input"))}}, s)
	require.NoError(t, err)
	for _, input := range []any{-1, 101, 50.5, "50"} {
		v, err := From(input)
		require.NoError(t, err)
		result, err := p.Run(context.Background(), Env{"input": v})
		require.NoError(t, err)
		require.Empty(t, result.Values)
		require.Len(t, result.Issues, 1)
	}
	result, err := p.Run(context.Background(), Env{"input": Integer(50)})
	require.NoError(t, err)
	require.Empty(t, result.Issues)
	n, valid := result.Values["share"].Int64()
	require.True(t, valid)
	require.EqualValues(t, 50, n)
	name, valid := result.Values["name"].Text()
	require.True(t, valid)
	require.Equal(t, "Changed", name)
	s.Bounds["share"] = NumberBounds{Min: &maximum, Max: &minimum}
	_, err = CompileProgram(Program{Steps: []Statement{set("share", literal(50))}}, s)
	require.ErrorContains(t, err, "Invalid numeric bounds")
}

package filters

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWhereAndMapRetainExactValuesAndLexicalItemScope(t *testing.T) {
	rows, err := From([]any{map[string]any{"name": "A", "count": int64(9007199254740993)}, map[string]any{"name": "B", "count": int64(7)}})
	require.NoError(t, err)
	s := Schema{Fields: map[string]Type{"rows": List, "item": Text}}
	condition := op("eq", op("get", field("item"), literal("name")), literal("A"))
	selected := op("where", field("rows"), condition)
	expr, err := Compile(op("sum", op("map", selected, op("get", field("item"), literal("count")))), s)
	require.NoError(t, err)
	env := Env{"rows": rows, "item": String("Outer value")}
	n, valid := expr.Eval(context.Background(), env).Int64()
	require.True(t, valid)
	require.EqualValues(t, 9007199254740993, n)
	outer, _ := env["item"].Text()
	require.Equal(t, "Outer value", outer)
	nested, err := Compile(op("map", field("rows"), op("map", op("list", field("item")), op("get", field("item"), literal("name")))), s)
	require.NoError(t, err)
	items, valid := nested.Eval(context.Background(), env).Items()
	require.True(t, valid)
	first, valid := items[0].Items()
	require.True(t, valid)
	name, valid := first[0].Text()
	require.True(t, valid)
	require.Equal(t, "A", name)
	_, err = Compile(field("item"), Schema{})
	require.Error(t, err)
	_, err = Compile(op("where", op("list", literal("A")), literal(1)), Schema{})
	require.Error(t, err)
	_, err = Compile(op("map", literal("A"), field("item")), Schema{})
	require.Error(t, err)
}

func TestWhereRequiresKnownConditionsAndCanHandleMissingFieldsExplicitly(t *testing.T) {
	rows, err := From([]any{map[string]any{"name": "A"}, map[string]any{"other": "B"}})
	require.NoError(t, err)
	s := Schema{Fields: map[string]Type{"rows": List}}
	name := op("get", field("item"), literal("name"))
	condition := op("eq", name, literal("A"))
	expr, err := Compile(op("where", field("rows"), condition), s)
	require.NoError(t, err)
	result := expr.Eval(context.Background(), Env{"rows": rows})
	require.Equal(t, Missing, result.Type)
	expr, err = Compile(op("count", op("where", field("rows"), op("and", op("present", name), condition))), s)
	require.NoError(t, err)
	n, valid := expr.Eval(context.Background(), Env{"rows": rows}).Int64()
	require.True(t, valid)
	require.EqualValues(t, 1, n)
}

func TestNestedListCalculationsHaveOneProgramBudget(t *testing.T) {
	items := []Value{}
	for i := 0; i < 400; i++ {
		items = append(items, Integer(int64(i)))
	}
	s := Schema{Fields: map[string]Type{"rows": List}, Targets: map[string]Type{"result": List, "name": Text}}
	p, err := CompileProgram(Program{Steps: []Statement{set("name", literal("Changed")), set("result", op("map", field("rows"), op("map", field("rows"), op("add", field("item"), literal(1)))))}}, s)
	require.NoError(t, err)
	result, err := p.Run(context.Background(), Env{"rows": Values(items...)})
	require.NoError(t, err)
	require.Empty(t, result.Values)
	require.Len(t, result.Issues, 1)
	require.Contains(t, result.Issues[0].Reason, "evaluation limit")
}

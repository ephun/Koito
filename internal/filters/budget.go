package filters

import "context"

type budgetKey struct{}

// One budget covers every expression and branch in a program, including nested
// list operations. Each independent program receives its own budget.
func evaluationContext(ctx context.Context) context.Context {
	if ctx.Value(budgetKey{}) != nil {
		return ctx
	}
	remaining := 100000
	return context.WithValue(ctx, budgetKey{}, &remaining)
}

func consumeEvaluation(ctx context.Context) bool {
	remaining := ctx.Value(budgetKey{}).(*int)
	*remaining--
	return *remaining >= 0
}

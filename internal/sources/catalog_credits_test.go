package sources

import (
	"context"
	"testing"

	"github.com/gabehf/koito/internal/filters"
	"github.com/stretchr/testify/require"
)

func TestCatalogCreditsPreserveOrderedMultipleArtistsAndValidatePrimaries(t *testing.T) {
	rule := Rule{ID: "credits", Type: "presentation", Subtype: "naming", Enabled: true, Program: namingFixture(t, `{"steps":[{"if":{"op":"eq","args":[{"op":"field","field":"catalog.kind"},{"op":"literal","value":"track"}]},"then":[{"set":"display.artist_ids","value":{"op":"literal","value":[2,1,2]}},{"set":"display.primary_artist_ids","value":{"op":"literal","value":[1,2]}},{"set":"display.title","value":{"op":"literal","value":"Corrected"}}]}]}`)}
	metadata := filters.Env{"catalog.artist_ids": identifierList([]int64{1}), "original.primary_artist_ids": identifierList([]int64{1}), "catalog.available_artist_ids": identifierList([]int64{1, 2, 3})}
	programs, err := compileNaming(RuleConfig{Rules: []Rule{rule}})
	require.NoError(t, err)
	result, issues, err := resolveCatalogNaming(context.Background(), programs, "track", 1, "Original", metadata)
	require.NoError(t, err)
	require.Empty(t, issues)
	require.Equal(t, []int64{2, 1}, result.Artists)
	require.Equal(t, []int64{1, 2}, result.Primary)
	require.Equal(t, "Corrected", result.Name)
	rule.Program = namingFixture(t, `{"steps":[{"if":{"op":"eq","args":[{"op":"field","field":"catalog.kind"},{"op":"literal","value":"track"}]},"then":[{"set":"display.artist_ids","value":{"op":"literal","value":[2]}},{"set":"display.primary_artist_ids","value":{"op":"literal","value":[3]}},{"set":"display.title","value":{"op":"literal","value":"Incorrect"}}]}]}`)
	programs, err = compileNaming(RuleConfig{Rules: []Rule{rule}})
	require.NoError(t, err)
	result, issues, err = resolveCatalogNaming(context.Background(), programs, "track", 1, "Original", metadata)
	require.NoError(t, err)
	require.NotEmpty(t, issues)
	require.Equal(t, "Original", result.Name)
	require.Equal(t, []int64{1}, result.Artists)
	require.Equal(t, []int64{1}, result.Primary)
}

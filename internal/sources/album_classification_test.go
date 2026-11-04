package sources

import (
	"context"
	"testing"

	"github.com/gabehf/koito/internal/filters"
	"github.com/stretchr/testify/require"
)

func TestAlbumClassificationUsesCreditsAndReportsUnavailableConditions(t *testing.T) {
	rule := Rule{ID: "compilation", Name: "Compilation credits", Type: "presentation", Subtype: "grouping", Enabled: true, Program: namingFixture(t, `{"steps":[{"if":{"op":"and","args":[{"op":"eq","args":[{"op":"field","field":"catalog.kind"},{"op":"literal","value":"album"}]},{"op":"in","args":[{"op":"literal","value":2},{"op":"field","field":"catalog.artist_ids"}]}]},"then":[{"set":"group.various_artists","value":{"op":"literal","value":true}}]}]}`)}
	programs, err := compileAlbumClassification(RuleConfig{Rules: []Rule{rule}})
	require.NoError(t, err)
	album := classificationAlbum{id: 1, title: "Compilation", metadata: filters.Env{"catalog.artist_ids": filters.Values(filters.Integer(1), filters.Integer(2))}}
	value, assigned, issues, err := resolveAlbumClassification(context.Background(), programs, album)
	require.NoError(t, err)
	require.True(t, value)
	require.True(t, assigned)
	require.Empty(t, issues)
	album.metadata = nil
	value, assigned, issues, err = resolveAlbumClassification(context.Background(), programs, album)
	require.NoError(t, err)
	require.False(t, value)
	require.False(t, assigned)
	require.NotEmpty(t, issues)
	require.Equal(t, rule.ID, issues[0].FilterID)
}

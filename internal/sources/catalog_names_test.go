package sources

import (
	"context"
	"github.com/gabehf/koito/internal/filters"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestCatalogNamePublicationPreservesAliasesAndReverses(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	mustImport(t, s, []byte("["+complete+"]"), "spotify.json", "")
	_, err := s.Rebuild(ctx)
	require.NoError(t, err)
	var id int64
	require.NoError(t, s.DB.QueryRow(`SELECT id FROM artists_with_name WHERE name='Artist'`).Scan(&id))
	condition := operation("and", operation("eq", fieldExpr("catalog.kind"), literalExpr("artist")), operation("eq", fieldExpr("catalog.id"), literalExpr(id)))
	value := literalExpr("Corrected artist")
	rule := Rule{ID: "catalog-name", Type: "presentation", Subtype: "naming", Enabled: true, Program: &filters.Program{Steps: []filters.Statement{{If: &condition, Then: []filters.Statement{{Set: "display.artist", Value: &value}}}}}}
	change := currentChange(t, s)
	change.Config.Rules = append(change.Config.Rules, rule)
	for _, enabled := range []bool{true, false, true} {
		change.BaseVersion = currentChange(t, s).BaseVersion
		change.Config.Rules[len(change.Config.Rules)-1].Enabled = enabled
		preview, e := s.Preview(ctx, change)
		require.NoError(t, e)
		require.NotEmpty(t, preview.NamingExamples)
		_, err = s.Save(ctx, change)
		require.NoError(t, err)
		var original string
		require.NoError(t, s.DB.QueryRow(`SELECT name FROM artists_with_name WHERE id=?`, id).Scan(&original))
		require.Equal(t, "Artist", original)
		if enabled {
			var name string
			require.NoError(t, s.DB.QueryRow(`SELECT display_name FROM catalog_names WHERE kind='artist' AND item_id=?`, id).Scan(&name))
			require.Equal(t, "Corrected artist", name)
		} else {
			require.Zero(t, count(t, s, "catalog_names"))
		}
		var payload string
		require.NoError(t, s.DB.QueryRow(`SELECT payload FROM source_records LIMIT 1`).Scan(&payload))
		require.JSONEq(t, complete, payload)
		require.Equal(t, 1, count(t, s, "listens"))
	}
}

func TestCatalogNamingSelectsContextAndPreservesOriginal(t *testing.T) {
	rule := Rule{ID: "catalog", Type: "presentation", Subtype: "naming", Enabled: true, Program: namingFixture(t, `{"steps":[{"if":{"op":"and","args":[{"op":"eq","args":[{"op":"field","field":"catalog.kind"},{"op":"literal","value":"artist"}]},{"op":"eq","args":[{"op":"field","field":"catalog.id"},{"op":"literal","value":1}]}]},"then":[{"set":"display.artist","value":{"op":"upper","args":[{"op":"field","field":"original.artist"}]}}]}]}`)}
	config := RuleConfig{Rules: []Rule{rule}}
	name, issues, err := CatalogName(context.Background(), config, "artist", 1, "Artist")
	require.NoError(t, err)
	require.Empty(t, issues)
	require.Equal(t, "ARTIST", name)
	name, issues, err = CatalogName(context.Background(), config, "artist", 2, "Artist")
	require.NoError(t, err)
	require.Empty(t, issues)
	require.Equal(t, "Artist", name)
	config.Rules[0].Enabled = false
	name, _, err = CatalogName(context.Background(), config, "artist", 1, "Artist")
	require.NoError(t, err)
	require.Equal(t, "Artist", name)
}

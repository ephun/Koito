package sources

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestCatalogIdentifierCorrectionsValidateClearAndDisable(t *testing.T) {
	original := "c9c172fb-2ae4-44d8-95bb-a84c50a5c591"
	rule := Rule{ID: "identifier", Type: "matching", Subtype: "music", Enabled: true, Program: namingFixture(t, `{"steps":[{"if":{"op":"eq","args":[{"op":"field","field":"catalog.kind"},{"op":"literal","value":"artist"}]},"then":[{"set":"music.identifier","value":{"op":"literal","value":"d9c172fb-2ae4-44d8-95bb-a84c50a5c591"}}]}]}`)}
	result, assigned, issues, err := catalogIdentifier(context.Background(), RuleConfig{Rules: []Rule{rule}}, "artist", 1, original)
	require.NoError(t, err)
	require.True(t, assigned)
	require.Empty(t, issues)
	require.Equal(t, "d9c172fb-2ae4-44d8-95bb-a84c50a5c591", result)
	rule.Enabled = false
	result, assigned, _, err = catalogIdentifier(context.Background(), RuleConfig{Rules: []Rule{rule}}, "artist", 1, original)
	require.NoError(t, err)
	require.False(t, assigned)
	require.Equal(t, original, result)
	rule.Enabled = true
	rule.Program = namingFixture(t, `{"steps":[{"if":{"op":"eq","args":[{"op":"field","field":"catalog.kind"},{"op":"literal","value":"artist"}]},"then":[{"set":"music.identifier","value":{"op":"literal","value":""}}]}]}`)
	result, assigned, issues, err = catalogIdentifier(context.Background(), RuleConfig{Rules: []Rule{rule}}, "artist", 1, original)
	require.NoError(t, err)
	require.True(t, assigned)
	require.Empty(t, issues)
	require.Empty(t, result)
}

func TestCatalogIdentifierPublicationPreservesStoredIdentifier(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	mustImport(t, s, []byte("["+complete+"]"), "spotify.json", "")
	_, err := s.Rebuild(ctx)
	require.NoError(t, err)
	var id int64
	require.NoError(t, s.DB.QueryRow(`SELECT id FROM artists_with_name WHERE name='Artist'`).Scan(&id))
	original := "c9c172fb-2ae4-44d8-95bb-a84c50a5c591"
	_, err = s.DB.Exec(`UPDATE artists SET musicbrainz_id=? WHERE id=?`, original, id)
	require.NoError(t, err)
	change := currentChange(t, s)
	change.Config = migrateMusicPrograms(change.Config)
	rule := Rule{ID: "identifier", Type: "matching", Subtype: "music", Enabled: true, Program: namingFixture(t, `{"steps":[{"if":{"op":"eq","args":[{"op":"field","field":"catalog.kind"},{"op":"literal","value":"artist"}]},"then":[{"set":"music.identifier","value":{"op":"literal","value":"d9c172fb-2ae4-44d8-95bb-a84c50a5c591"}}]}]}`)}
	change.Config.Rules = append(change.Config.Rules, rule)
	for _, enabled := range []bool{true, false} {
		change.BaseVersion = currentChange(t, s).BaseVersion
		change.Config.Rules[len(change.Config.Rules)-1].Enabled = enabled
		preview, e := s.Preview(ctx, change)
		require.NoError(t, e)
		require.NotEmpty(t, preview.MusicExamples)
		_, err = s.Save(ctx, change)
		require.NoError(t, err)
		var stored string
		require.NoError(t, s.DB.QueryRow(`SELECT musicbrainz_id FROM artists WHERE id=?`, id).Scan(&stored))
		require.Equal(t, original, stored)
		if enabled {
			var corrected string
			require.NoError(t, s.DB.QueryRow(`SELECT identifier FROM catalog_identifiers WHERE kind='artist' AND item_id=?`, id).Scan(&corrected))
			require.Equal(t, "d9c172fb-2ae4-44d8-95bb-a84c50a5c591", corrected)
		} else {
			require.Zero(t, count(t, s, "catalog_identifiers"))
		}
	}
}

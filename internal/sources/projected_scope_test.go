package sources

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestScopePreviewIncludesNewCatalogItemsWithoutPublishingThem(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	mustImport(t, s, []byte("["+complete+"]"), "spotify.json", "")
	require.Zero(t, count(t, s, "tracks"))
	change := currentChange(t, s)
	change.Config.Rules = append(change.Config.Rules, Rule{ID: "scope", Type: "inclusion", Subtype: "scope", Enabled: true, Program: namingFixture(t, `{"steps":[{"if":{"op":"eq","args":[{"op":"field","field":"display.title"},{"op":"literal","value":"Song"}]},"then":[{"set":"view.include","value":{"op":"literal","value":false}}]}]}`)})
	versions := count(t, s, "filter_versions")
	preview, err := s.Preview(ctx, change)
	require.NoError(t, err)
	require.NotEmpty(t, preview.ScopeExamples)
	require.Equal(t, "Absent", preview.ScopeExamples[0].Before)
	require.Equal(t, "Excluded", preview.ScopeExamples[0].After)
	require.Zero(t, count(t, s, "tracks"))
	require.Zero(t, count(t, s, "listens"))
	require.Equal(t, versions, count(t, s, "filter_versions"))
	change.Token = preview.Token
	_, err = s.Apply(ctx, change)
	require.NoError(t, err)
	require.Equal(t, 1, count(t, s, "listens"))
	require.Zero(t, count(t, s, "displayed_listens"))
}

func TestScopePreviewPublicationFailureRollsBackDerivedChanges(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	mustImport(t, s, []byte("["+complete+"]"), "spotify.json", "")
	_, err := s.Rebuild(ctx)
	require.NoError(t, err)
	var key string
	require.NoError(t, s.DB.QueryRow(`SELECT play_key FROM source_plays`).Scan(&key))
	versions, runs := count(t, s, "filter_versions"), count(t, s, "source_projection_runs")
	_, err = s.DB.Exec(`CREATE TRIGGER reject_scope_preview BEFORE INSERT ON source_plays BEGIN SELECT RAISE(ABORT,'projection rejected'); END`)
	require.NoError(t, err)
	change := currentChange(t, s)
	change.Config.Rules = append(change.Config.Rules, Rule{ID: "scope", Type: "inclusion", Subtype: "scope", Enabled: true, Program: namingFixture(t, `{"steps":[{"if":{"op":"literal","value":true},"then":[{"set":"view.include","value":{"op":"literal","value":false}}]}]}`)})
	_, err = s.Preview(ctx, change)
	require.ErrorContains(t, err, "projection rejected")
	var retained string
	require.NoError(t, s.DB.QueryRow(`SELECT play_key FROM source_plays`).Scan(&retained))
	require.Equal(t, key, retained)
	require.Equal(t, versions, count(t, s, "filter_versions"))
	require.Equal(t, runs, count(t, s, "source_projection_runs"))
	require.Equal(t, 1, count(t, s, "listens"))
	require.Equal(t, 1, count(t, s, "displayed_listens"))
	require.Zero(t, count(t, s, "track_scope"))
}

func TestScopePreviewUsesPendingHistoryMembership(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	other := strings.ReplaceAll(strings.ReplaceAll(complete, `"Song"`, `"song"`), "2020-01-02T03:04:05Z", "2020-01-03T03:04:05Z")
	mustImport(t, s, []byte("["+complete+","+other+"]"), "spotify.json", "")
	change := currentChange(t, s)
	change.Config.Rules[0].Settings.CaseSensitive = true
	change.Config.Rules = append(change.Config.Rules, Rule{ID: "history", Type: "inclusion", Subtype: "scope", Enabled: true, Program: namingFixture(t, `{"steps":[{"if":{"op":"in","args":[{"op":"literal","value":1},{"op":"field","field":"catalog.track_ids"}]},"then":[{"set":"view.include","value":{"op":"literal","value":false}}]}]}`)})
	_, err := s.Save(ctx, change)
	require.NoError(t, err)
	require.Equal(t, 1, count(t, s, "displayed_listens"))
	change = currentChange(t, s)
	change.Config.Rules[0].Settings.CaseSensitive = false
	preview, err := s.Preview(ctx, change)
	require.NoError(t, err)
	require.Contains(t, preview.ScopeExamples, NamingExample{Field: "song at 2020-01-03T03:01:04Z", Before: "Included", After: "Excluded"})
	require.Equal(t, 1, count(t, s, "displayed_listens"))
	change.Token = preview.Token
	_, err = s.Apply(ctx, change)
	require.NoError(t, err)
	require.Zero(t, count(t, s, "displayed_listens"))
	require.Equal(t, 2, count(t, s, "listens"))
}

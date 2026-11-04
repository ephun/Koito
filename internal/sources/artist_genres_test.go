package sources

import (
	"context"
	"encoding/json"
	"github.com/gabehf/koito/internal/filters"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestArtistGenresSelectEvidenceBySource(t *testing.T) {
	rule := Rule{ID: "source", Type: "presentation", Subtype: "naming", Enabled: true, Program: namingFixture(t, `{"steps":[{"set":"artist.genres","value":{"op":"map","args":[{"op":"where","args":[{"op":"field","field":"source.genre_records"},{"op":"eq","args":[{"op":"get","args":[{"op":"field","field":"item"},{"op":"literal","value":"source_id"}]},{"op":"literal","value":2}]}]},{"op":"get","args":[{"op":"field","field":"item"},{"op":"literal","value":"value"}]}]}}]}`)}
	programs, err := compileNamingDomain(RuleConfig{Rules: []Rule{rule}}, true)
	require.NoError(t, err)
	evidence := []filters.Value{}
	for _, raw := range []string{`{"source_id":1,"record_id":10,"file_hash":"first","value":"Rock"}`, `{"source_id":2,"record_id":11,"file_hash":"second","value":"Jazz"}`} {
		v, err := filters.FromJSON(json.RawMessage(raw))
		require.NoError(t, err)
		evidence = append(evidence, v)
	}
	genres, issues, err := resolveArtistGenres(context.Background(), programs, 1, "Artist", []string{"Rock", "Jazz"}, evidence...)
	require.NoError(t, err)
	require.Empty(t, issues)
	require.Equal(t, []string{"Jazz"}, genres)
}

func TestArtistGenreProgramsOverrideClearAndDisableWithoutChangingEvidence(t *testing.T) {
	source := []string{"Rock", "Rock", "Pop"}
	rule := Rule{ID: "genre", Type: "presentation", Subtype: "naming", Enabled: true, Program: namingFixture(t, `{"steps":[{"if":{"op":"eq","args":[{"op":"field","field":"catalog.artist_id"},{"op":"literal","value":1}]},"then":[{"set":"artist.genres","value":{"op":"literal","value":["Jazz"]}}]}]}`)}
	result, issues, err := ArtistGenres(context.Background(), RuleConfig{Rules: []Rule{rule}}, 1, "Artist", source)
	require.NoError(t, err)
	require.Empty(t, issues)
	require.Equal(t, []string{"Jazz"}, result)
	rule.Enabled = false
	result, _, err = ArtistGenres(context.Background(), RuleConfig{Rules: []Rule{rule}}, 1, "Artist", source)
	require.NoError(t, err)
	require.Equal(t, []string{"Rock", "Pop"}, result)
	rule.Enabled = true
	rule.Program = namingFixture(t, `{"steps":[{"set":"artist.genres","value":{"op":"literal","value":[]}}]}`)
	result, issues, err = ArtistGenres(context.Background(), RuleConfig{Rules: []Rule{rule}}, 1, "Artist", source)
	require.NoError(t, err)
	require.Empty(t, issues)
	require.Empty(t, result)
	require.Equal(t, []string{"Rock", "Rock", "Pop"}, source)
}

func TestGenreAndRecordNamingHaveSeparateEvaluationContexts(t *testing.T) {
	config := RuleConfig{Rules: []Rule{
		{ID: "record", Type: "presentation", Subtype: "naming", Enabled: true, Program: namingFixture(t, `{"steps":[{"set":"display.title","value":{"op":"field","field":"original.title"}}]}`)},
		{ID: "artist", Type: "presentation", Subtype: "naming", Enabled: true, Program: namingFixture(t, `{"steps":[{"set":"artist.genres","value":{"op":"literal","value":["Jazz"]}}]}`)},
	}}
	result, issues, err := ArtistGenres(context.Background(), config, 1, "Artist", nil)
	require.NoError(t, err)
	require.Empty(t, issues)
	require.Equal(t, []string{"Jazz"}, result)
	names, err := compileNaming(config)
	require.NoError(t, err)
	require.Len(t, names, 1)
	require.Equal(t, "record", names[0].rule.ID)
	config.Rules[1].Program = namingFixture(t, `{"steps":[{"set":"artist.genres","value":{"op":"literal","value":["Jazz"]}},{"set":"display.artist","value":{"op":"literal","value":"Changed"}}]}`)
	result, issues, err = ArtistGenres(context.Background(), config, 1, "Artist", nil)
	require.NoError(t, err)
	require.Empty(t, issues)
	require.Equal(t, []string{"Jazz"}, result)
	name, issues, err := CatalogName(context.Background(), config, "artist", 1, "Artist")
	require.NoError(t, err)
	require.Empty(t, issues)
	require.Equal(t, "Changed", name)
}

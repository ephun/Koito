package sources

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSourceGenresPreserveLabelsAndRepeatedEvidence(t *testing.T) {
	for _, parser := range []string{ITunesXML, ITunesJSON} {
		genres, err := SourceGenres(parser, []byte(`{"kind":"library_track","fields":{"Genre":{"$plist_repeated":["Rock, Pop"," Rock ","Rock, Pop",null,42]}}}`))
		require.NoError(t, err)
		require.Equal(t, []SourceGenre{{"Rock, Pop", "raw:/fields/Genre/$plist_repeated/0"}, {" Rock ", "raw:/fields/Genre/$plist_repeated/1"}, {"Rock, Pop", "raw:/fields/Genre/$plist_repeated/2"}}, genres)
	}
	for _, payload := range []string{`{"kind":"library_track","fields":{}}`, `{"kind":"library_track","fields":{"Genre":null}}`, `{"kind":"library_track","fields":{"Genre":["Rock","Pop"]}}`, `{"kind":"playlist","fields":{"Genre":"Rock"}}`} {
		genres, err := SourceGenres(ITunesJSON, []byte(payload))
		require.NoError(t, err)
		require.Empty(t, genres)
	}
	genres, err := SourceGenres(ITunesJSON, []byte(`{"kind":"library_track","fields":{"Genre":""}}`))
	require.NoError(t, err)
	require.Equal(t, []SourceGenre{{"", "raw:/fields/Genre"}}, genres)
	_, err = SourceGenres(ITunesJSON, []byte(`{`))
	require.Error(t, err)
	genres, err = SourceGenres("unsupported", []byte(`{`))
	require.NoError(t, err)
	require.Empty(t, genres)
}

func TestGenreInspectionDoesNotRequireCountInference(t *testing.T) {
	s := testStore(t)
	track := libraryFixtureTrack("A", 7)
	track["Genre"] = "Rock, Pop"
	importLibrarySnapshot(t, s, "ONE", "2012-01-01T00:00:00Z", []map[string]any{track})
	var id int64
	require.NoError(t, s.DB.QueryRow(`SELECT id FROM source_records WHERE kind='library_track'`).Scan(&id))
	inspected, err := s.Inspect(context.Background(), id)
	require.NoError(t, err)
	require.Equal(t, []SourceGenre{{"Rock, Pop", "raw:/fields/Genre"}}, inspected.SourceGenres)
	require.Empty(t, inspected.Contributions)
	require.Zero(t, count(t, s, "listens"))
	for i := 0; i < 2; i++ {
		_, err = s.Rebuild(context.Background())
		require.NoError(t, err)
		require.Equal(t, 1, count(t, s, "source_genres"))
		var value, field string
		require.NoError(t, s.DB.QueryRow(`SELECT value,field FROM source_genres WHERE record_id=?`, id).Scan(&value, &field))
		require.Equal(t, "Rock, Pop", value)
		require.Equal(t, "raw:/fields/Genre", field)
		require.Zero(t, count(t, s, "listens"))
	}
}

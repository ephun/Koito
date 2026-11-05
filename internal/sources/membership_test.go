package sources

import (
	"bytes"
	"context"
	"testing"

	"github.com/gabehf/koito/internal/filters"
	"github.com/gabehf/koito/internal/music"
	"github.com/stretchr/testify/require"
)

func TestMembershipSnapshotsViewAndFilters(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	mustImport(t, s, []byte("["+complete+"]"), "extended.json", "")
	library := `{"tracks":[{"artist":"Artist","track":"Song","album":"Album","uri":"spotify:track:one"}],"albums":[],"artists":[]}`
	_, err := s.Import(ctx, bytes.NewBufferString(library), "YourLibrary.json", 1)
	require.NoError(t, err)
	collection := "Catalog#,Artist,Title,Label,Format,Rating,Released,release_id,CollectionFolder\nA,Artist,Album,Label,LP,5,0,100,All\nA,Artist,Album,Label,LP,5,0,100,All\n"
	_, err = s.Import(ctx, bytes.NewBufferString(collection), "collection.csv", 1)
	require.NoError(t, err)
	result, err := s.Rebuild(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, result.Plays)
	var track, album int64
	require.NoError(t, s.DB.QueryRow(`SELECT t.id,t.release_id FROM source_plays p JOIN tracks t ON t.id=p.track_id`).Scan(&track, &album))
	states, err := s.MusicStates(ctx, track, 0)
	require.NoError(t, err)
	require.Len(t, states, 2)
	require.Equal(t, "owned", states[0].Kind)
	require.Equal(t, 2, states[0].Quantity)
	require.Equal(t, "saved_tracks", states[1].Kind)
	version, err := readRuleVersion(ctx, s.DB, 0)
	require.NoError(t, err)
	records, err := loadMatchingRecords(ctx, s.DB, version.Config)
	require.NoError(t, err)
	for _, key := range []string{"music.saved", "music.owned"} {
		value, ok := recordEnvironment(records[0])[key].Bool()
		require.True(t, ok)
		require.True(t, value)
	}
	inspection, err := s.Inspect(ctx, records[0].ID)
	require.NoError(t, err)
	owned, ok := recordEnvironment(inspection.Record.matchingRecord)["music.owned"].Bool()
	require.True(t, ok)
	require.True(t, owned)
	value := literalExpr("other")
	condition := fieldExpr("music.saved")
	saveTestRules(t, s, Rule{ID: "saved-listener", Type: "attribution", Subtype: "direct", Enabled: true, Program: &filters.Program{Steps: []filters.Statement{{If: &condition, Then: []filters.Statement{{Set: "listener", Value: &value}}}}}})
	require.Zero(t, count(t, s, "listens"))
	change := currentChange(t, s)
	testRule(t, &change.Config, "saved-listener").Enabled = false
	_, err = s.Save(ctx, change)
	require.NoError(t, err)
	require.Equal(t, 1, count(t, s, "listens"))
	// A later empty snapshot removes saves. Reimporting the older bytes keeps
	// its original snapshot ID and cannot restore an outdated membership.
	_, err = s.Import(ctx, bytes.NewBufferString(`{"tracks":[],"albums":[],"artists":[]}`), "YourLibrary.json", 1)
	require.NoError(t, err)
	_, err = s.Import(ctx, bytes.NewBufferString(library), "YourLibrary.json", 1)
	require.NoError(t, err)
	_, err = s.Rebuild(ctx)
	require.NoError(t, err)
	states, err = s.MusicStates(ctx, track, 0)
	require.NoError(t, err)
	require.Len(t, states, 1)
	require.Equal(t, "owned", states[0].Kind)
	items, err := music.ScopeItems(ctx, s.DB, track)
	require.NoError(t, err)
	require.NotEmpty(t, items)
	saved, _ := items[0].Env["music.saved"].Bool()
	require.False(t, saved)
	owned, _ = items[0].Env["music.owned"].Bool()
	require.True(t, owned)
	states, err = s.MusicStates(ctx, 0, album)
	require.NoError(t, err)
	require.Len(t, states, 1)
	require.Equal(t, 2, states[0].Quantity)
	require.Equal(t, 1, count(t, s, "listens"))
}

func TestDiscogsReleaseIDsRemainDistinct(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	data := "Catalog#,Artist,Title,Label,Format,Rating,Released,release_id,CollectionFolder\nA,Artist,Album,Label,LP,5,0,100,All\nA,Artist,Album,Label,LP,5,0,101,All\n"
	_, err := s.Import(ctx, bytes.NewBufferString(data), "collection.csv", 1)
	require.NoError(t, err)
	result, err := s.Rebuild(ctx)
	require.NoError(t, err)
	require.Zero(t, result.Plays)
	var distinct int
	require.NoError(t, s.DB.QueryRow(`SELECT COUNT(DISTINCT release_id) FROM source_music_states WHERE kind='owned'`).Scan(&distinct))
	require.Equal(t, 2, distinct)
	_, err = s.Rebuild(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, count(t, s, "source_release_ids"))
	require.Equal(t, 2, count(t, s, "source_music_states"))
}

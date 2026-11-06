package sources

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNamedSpotifySourcesKeepIdentityWithOneServiceChoice(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	for index, label := range []string{"", "parent-one", "parent-two"} {
		body := strings.Replace(complete, "03:04:05", []string{"03:04:05", "03:04:06", "03:04:07"}[index], 1)
		_, err := s.ImportSpotify(ctx, bytes.NewBufferString("["+body+"]"), "history.json", label, 1)
		require.NoError(t, err)
	}
	state, err := s.Filters(ctx)
	require.NoError(t, err)
	require.Len(t, state.Sources, 3)
	require.Equal(t, "parent-one", state.Sources[1].Label)
	require.Equal(t, "parent-two", state.Sources[2].Label)
	choices := state.Blocks.Schemas["attribution:direct"].Choices["source.service"]
	require.Len(t, choices, 1)
	require.Equal(t, "spotify", choices[0].Value)
}

func TestPendingFamilyHistoryKeepsCorroboratedPersonalPlays(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	parentOnly := strings.Replace(complete, "2020-01-02", "2020-01-03", 1)
	mustImport(t, s, []byte("["+complete+","+parentOnly+"]"), "parent.json", "parent")
	importLastfm(t, s, "["+lf("Song", "Album", 1577934065)+"]")
	var parentSource, personalSource int64
	require.NoError(t, s.DB.QueryRow(`SELECT id FROM source_accounts WHERE account='parent'`).Scan(&parentSource))
	require.NoError(t, s.DB.QueryRow(`SELECT id FROM source_accounts WHERE service='lastfm'`).Scan(&personalSource))
	change := currentChange(t, s)
	for _, assignment := range []struct {
		id       string
		source   int64
		listener string
	}{{"family", parentSource, "unknown"}, {"personal", personalSource, "self"}} {
		program := namingFixture(t, fmt.Sprintf(`{"steps":[{"if":{"op":"in","args":[{"op":"field","field":"source.id"},{"op":"literal","value":[%d]}]},"then":[{"set":"listener","value":{"op":"literal","value":%q}}]}]}`, assignment.source, assignment.listener))
		change.Config.Rules = append(change.Config.Rules, Rule{ID: assignment.id, Type: "attribution", Subtype: "direct", Enabled: true, Program: program})
	}
	_, err := s.Save(ctx, change)
	require.NoError(t, err)
	require.Equal(t, 1, count(t, s, "listens"))
	require.Equal(t, 3, count(t, s, "source_records"))
	var contributors int
	require.NoError(t, s.DB.QueryRow(`SELECT COUNT(*) FROM source_play_records i JOIN source_plays p ON p.play_key=i.play_key`).Scan(&contributors))
	require.Equal(t, 2, contributors, "family copy corroborates an existing play without adding a separate play")
	var excluded int
	require.NoError(t, s.DB.QueryRow(`SELECT COUNT(*) FROM source_records r JOIN source_interpretations i ON i.record_id=r.id WHERE r.source_id=? AND i.reason='listener: unknown'`, parentSource).Scan(&excluded))
	require.Equal(t, 1, excluded, "family-only listening remains unassigned")
}

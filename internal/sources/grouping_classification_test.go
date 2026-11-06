package sources

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAlbumContextGroupingAndClassificationApplyTogetherToImportedPlays(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	input := strings.ReplaceAll(complete, `"Album"`, `"Album (Deluxe)"`)
	mustImport(t, s, []byte("["+input+"]"), "spotify.json", "")
	_, err := s.Rebuild(ctx)
	require.NoError(t, err)
	change := currentChange(t, s)
	change.Config.Rules = append(change.Config.Rules, Rule{ID: "mixed", Type: "presentation", Subtype: "grouping", Enabled: true, Program: namingFixture(t, `{"steps":[{"if":{"op":"and","args":[{"op":"eq","args":[{"op":"field","field":"catalog.kind"},{"op":"literal","value":"album"}]},{"op":"in","args":[{"op":"literal","value":1},{"op":"field","field":"catalog.artist_ids"}]}]},"then":[{"set":"group.album","value":{"op":"literal","value":"Album"}},{"set":"group.various_artists","value":{"op":"eq","args":[{"op":"field","field":"group.album"},{"op":"literal","value":"Album"}]}}]}]}`)})
	for _, enabled := range []bool{true, false} {
		change.BaseVersion = currentChange(t, s).BaseVersion
		change.Config.Rules[len(change.Config.Rules)-1].Enabled = enabled
		preview, err := s.Preview(ctx, change)
		require.NoError(t, err)
		require.NotEmpty(t, preview.GroupingExamples)
		_, err = s.Save(ctx, change)
		require.NoError(t, err)
		var grouped int
		require.NoError(t, s.DB.QueryRow(`SELECT COUNT(*) FROM album_report_events WHERE album_group_id>0`).Scan(&grouped))
		if enabled {
			require.Equal(t, 1, grouped)
			var various bool
			require.NoError(t, s.DB.QueryRow(`SELECT various_artists FROM album_classification LIMIT 1`).Scan(&various))
			require.True(t, various)
		} else {
			require.Zero(t, grouped)
			require.Zero(t, count(t, s, "album_classification"))
		}
		var original bool
		require.NoError(t, s.DB.QueryRow(`SELECT various_artists FROM releases WHERE id=(SELECT release_id FROM tracks LIMIT 1)`).Scan(&original))
		require.False(t, original)
		require.Equal(t, 1, count(t, s, "listens"))
		require.Equal(t, 1, count(t, s, "source_records"))
	}
}

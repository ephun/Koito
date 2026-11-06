package sqlite_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/gabehf/koito/internal/db"
	"github.com/gabehf/koito/internal/filters"
	"github.com/gabehf/koito/internal/sources"
	"github.com/stretchr/testify/require"
)

func TestAlbumClassificationCanBeDisabledWithoutChangingStoredCredits(t *testing.T) {
	s := musicStore(t)
	ctx := context.Background()
	store := sources.Store{DB: s.SourceDatabase(), Root: t.TempDir()}
	state, err := store.Filters(ctx)
	require.NoError(t, err)
	var program filters.Program
	require.NoError(t, json.Unmarshal([]byte(`{"steps":[{"if":{"op":"eq","args":[{"op":"field","field":"catalog.kind"},{"op":"literal","value":"album"}]},"then":[{"set":"group.various_artists","value":{"op":"literal","value":true}}]}]}`), &program))
	state.Current.Config.Rules = append(state.Current.Config.Rules, sources.Rule{ID: "classification", Type: "presentation", Subtype: "grouping", Enabled: true, Program: &program})
	original, err := s.GetAlbum(ctx, db.GetAlbumOpts{ID: 1})
	require.NoError(t, err)
	for _, enabled := range []bool{true, false} {
		state.Current.Config.Rules[len(state.Current.Config.Rules)-1].Enabled = enabled
		preview, err := store.Preview(ctx, sources.FilterChange{BaseVersion: state.Current.ID, Config: state.Current.Config})
		require.NoError(t, err)
		require.NotEmpty(t, preview.GroupingExamples)
		version, err := store.Save(ctx, sources.FilterChange{BaseVersion: state.Current.ID, Config: state.Current.Config})
		require.NoError(t, err)
		state.Current.ID = version.ID
		album, err := s.GetAlbum(ctx, db.GetAlbumOpts{ID: 1})
		require.NoError(t, err)
		require.Equal(t, enabled || original.VariousArtists, album.VariousArtists)
		require.Equal(t, original.Artists, album.Artists)
		var stored bool
		require.NoError(t, s.SourceDatabase().QueryRow(`SELECT various_artists FROM releases WHERE id=1`).Scan(&stored))
		require.Equal(t, original.VariousArtists, stored)
	}
}

package sqlite_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/gabehf/koito/internal/db"
	"github.com/gabehf/koito/internal/filters"
	"github.com/gabehf/koito/internal/sources"
	"github.com/stretchr/testify/require"
)

func TestNativeCaptureAppliesCombinedGroupingAndClassificationWithReleaseException(t *testing.T) {
	s := musicStore(t)
	ctx := context.Background()
	base, err := s.SaveAlbum(ctx, db.SaveAlbumOpts{Title: "Album", ArtistIDs: []int32{1}})
	require.NoError(t, err)
	store := sources.Store{DB: s.SourceDatabase(), Root: t.TempDir()}
	state, err := store.Filters(ctx)
	require.NoError(t, err)
	var program filters.Program
	require.NoError(t, json.Unmarshal([]byte(fmt.Sprintf(`{"steps":[{"if":{"op":"regex","args":[{"op":"field","field":"original.album"},{"op":"literal","value":"Deluxe"}]},"then":[{"set":"group.album","value":{"op":"literal","value":"Album"}},{"set":"group.various_artists","value":{"op":"literal","value":true}}],"else_if":[{"if":{"op":"eq","args":[{"op":"field","field":"catalog.id"},{"op":"literal","value":%d}]},"then":[{"set":"group.album","value":{"op":"literal","value":""}}]}]}]}`, base.ID)), &program))
	state.Current.Config.Rules = append(state.Current.Config.Rules, sources.Rule{ID: "combined", Type: "presentation", Subtype: "grouping", Enabled: true, Program: &program})
	_, err = store.Save(ctx, sources.FilterChange{BaseVersion: state.Current.ID, Config: state.Current.Config})
	require.NoError(t, err)
	deluxe, err := s.SaveAlbum(ctx, db.SaveAlbumOpts{Title: "Album (Deluxe)", ArtistIDs: []int32{1}})
	require.NoError(t, err)
	item, err := s.GetAlbum(ctx, db.GetAlbumOpts{ID: deluxe.ID})
	require.NoError(t, err)
	require.True(t, item.VariousArtists)
	var members int
	require.NoError(t, s.SourceDatabase().QueryRow(`SELECT COUNT(*) FROM album_group_members WHERE release_id=?`, deluxe.ID).Scan(&members))
	require.Equal(t, 1, members)
	require.NoError(t, s.SourceDatabase().QueryRow(`SELECT COUNT(*) FROM album_group_members WHERE release_id=?`, base.ID).Scan(&members))
	require.Zero(t, members)
	var original bool
	require.NoError(t, s.SourceDatabase().QueryRow(`SELECT various_artists FROM releases WHERE id=?`, deluxe.ID).Scan(&original))
	require.False(t, original)
}

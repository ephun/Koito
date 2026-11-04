package sqlite_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/gabehf/koito/internal/db"
	"github.com/gabehf/koito/internal/filters"
	"github.com/gabehf/koito/internal/sources"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestCatalogAliasesAreSearchableReversibleAndSeparateFromOriginals(t *testing.T) {
	s := musicStore(t)
	ctx := context.Background()
	musicAppearance(t, s, "Song", 1, uuid.Nil)
	store := sources.Store{DB: s.SourceDatabase(), Root: t.TempDir()}
	state, err := store.Filters(ctx)
	require.NoError(t, err)
	for _, kind := range []string{"artist", "album", "track"} {
		var program filters.Program
		raw := `{"steps":[{"if":{"op":"eq","args":[{"op":"field","field":"catalog.kind"},{"op":"literal","value":"` + kind + `"}]},"then":[{"set":"display.aliases","value":{"op":"literal","value":["` + kind + ` alternative"]}}]}]}`
		require.NoError(t, json.Unmarshal([]byte(raw), &program))
		state.Current.Config.Rules = append(state.Current.Config.Rules, sources.Rule{ID: "aliases-" + kind, Type: "presentation", Subtype: "naming", Enabled: true, Program: &program})
	}
	for _, enabled := range []bool{true, false} {
		for i := range state.Current.Config.Rules {
			if state.Current.Config.Rules[i].Subtype == "naming" {
				state.Current.Config.Rules[i].Enabled = enabled
			}
		}
		preview, err := store.Preview(ctx, sources.FilterChange{BaseVersion: state.Current.ID, Config: state.Current.Config})
		require.NoError(t, err)
		require.NotEmpty(t, preview.NamingExamples)
		version, err := store.Save(ctx, sources.FilterChange{BaseVersion: state.Current.ID, Config: state.Current.Config})
		require.NoError(t, err)
		state.Current.ID = version.ID
		artists, err := s.SearchArtists(ctx, "artist alternative")
		require.NoError(t, err)
		require.Equal(t, enabled, len(artists) > 0)
		albums, err := s.SearchAlbums(ctx, "album alternative")
		require.NoError(t, err)
		require.Equal(t, enabled, len(albums) > 0)
		tracks, err := s.SearchTracks(ctx, "track alternative")
		require.NoError(t, err)
		require.Equal(t, enabled, len(tracks) > 0)
		var original string
		require.NoError(t, s.SourceDatabase().QueryRow(`SELECT alias FROM artist_aliases WHERE artist_id=1 AND is_primary=1`).Scan(&original))
		require.Equal(t, "Artist", original)
	}
}

func TestEmptyAliasSetKeepsDisplayedNameSearchable(t *testing.T) {
	s := musicStore(t)
	ctx := context.Background()
	musicAppearance(t, s, "Song", 1, uuid.Nil)
	require.NoError(t, s.Exec(`INSERT INTO catalog_alias_sets VALUES('artist',1),('album',1),('track',1)`))
	aliases, err := s.GetAllArtistAliases(ctx, 1)
	require.NoError(t, err)
	require.NotNil(t, aliases)
	require.Empty(t, aliases)
	artists, err := s.SearchArtists(ctx, "Artist")
	require.NoError(t, err)
	require.Len(t, artists, 1)
	albums, err := s.SearchAlbums(ctx, "Single")
	require.NoError(t, err)
	require.Len(t, albums, 1)
	tracks, err := s.SearchTracks(ctx, "Song")
	require.NoError(t, err)
	require.Len(t, tracks, 1)
}

func TestArtistAliasDetailsPreserveAliasPunctuation(t *testing.T) {
	s := musicStore(t)
	require.NoError(t, s.Exec(`INSERT INTO catalog_alias_sets VALUES('artist',1);INSERT INTO catalog_aliases VALUES('artist',1,'Name||with punctuation')`))
	item, err := s.GetArtist(context.Background(), db.GetArtistOpts{ID: 1})
	require.NoError(t, err)
	require.Equal(t, []string{"Name||with punctuation"}, item.Aliases)
}

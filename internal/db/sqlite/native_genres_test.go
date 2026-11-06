package sqlite_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gabehf/koito/internal/db"
	"github.com/gabehf/koito/internal/filters"
	"github.com/gabehf/koito/internal/sources"
	"github.com/stretchr/testify/require"
)

func TestNativeArtistReceivesSavedGenresAndFailureRollsBack(t *testing.T) {
	s := musicStore(t)
	ctx := context.Background()
	store := sources.Store{DB: s.SourceDatabase(), Root: t.TempDir()}
	state, err := store.Filters(ctx)
	require.NoError(t, err)
	var program filters.Program
	require.NoError(t, json.Unmarshal([]byte(`{"steps":[{"if":{"op":"eq","args":[{"op":"field","field":"original.artist"},{"op":"literal","value":"Incoming"}]},"then":[{"set":"artist.genres","value":{"op":"literal","value":["Ambient"]}}]}]}`), &program))
	state.Current.Config.Rules = append(state.Current.Config.Rules, sources.Rule{ID: "incoming-genre", Type: "presentation", Subtype: "naming", Enabled: true, Program: &program})
	_, err = store.Save(ctx, sources.FilterChange{BaseVersion: state.Current.ID, Config: state.Current.Config})
	require.NoError(t, err)
	require.NoError(t, s.Exec(`CREATE TRIGGER reject_native_genre BEFORE INSERT ON artist_genres WHEN NEW.genre='Ambient' BEGIN SELECT RAISE(ABORT,'genre rejected'); END`))
	_, err = s.SaveArtist(ctx, db.SaveArtistOpts{Name: "Incoming"})
	require.ErrorContains(t, err, "genre rejected")
	var count int
	require.NoError(t, s.SourceDatabase().QueryRow(`SELECT COUNT(*) FROM artists`).Scan(&count))
	require.Equal(t, 1, count)
	require.NoError(t, s.SourceDatabase().QueryRow(`SELECT COUNT(*) FROM artist_aliases WHERE alias='Incoming'`).Scan(&count))
	require.Zero(t, count)
	require.NoError(t, s.Exec(`DROP TRIGGER reject_native_genre`))
	artist, err := s.SaveArtist(ctx, db.SaveArtistOpts{Name: "Incoming"})
	require.NoError(t, err)
	got, err := s.GetArtist(ctx, db.GetArtistOpts{ID: artist.ID})
	require.NoError(t, err)
	require.Equal(t, []string{"Ambient"}, got.Genres)
	_, err = store.Rebuild(ctx)
	require.NoError(t, err)
	got, err = s.GetArtist(ctx, db.GetArtistOpts{ID: artist.ID})
	require.NoError(t, err)
	require.Equal(t, []string{"Ambient"}, got.Genres)
}

func TestNativeSharedAliasRefreshesSourceGenreAmbiguity(t *testing.T) {
	s := musicStore(t)
	ctx := context.Background()
	store := sources.Store{DB: s.SourceDatabase(), Root: t.TempDir()}
	_, err := store.Import(ctx, strings.NewReader(`{"Library Persistent ID":"ONE","Tracks":{"1":{"Track ID":1,"Persistent ID":"A","Name":"Song","Artist":"Artist","Album":"Album","Genre":"Rock, Pop","Play Count":7}}}`), "library.json", 1)
	require.NoError(t, err)
	_, err = store.Rebuild(ctx)
	require.NoError(t, err)
	before, err := s.GetArtist(ctx, db.GetArtistOpts{ID: 1})
	require.NoError(t, err)
	require.Equal(t, []string{"Rock, Pop"}, before.Genres)
	require.Len(t, before.GenreEvidence, 1)
	other, err := s.SaveArtist(ctx, db.SaveArtistOpts{Name: "Different"})
	require.NoError(t, err)
	unchanged, err := s.GetArtist(ctx, db.GetArtistOpts{ID: 1})
	require.NoError(t, err)
	require.Equal(t, before.Genres, unchanged.Genres)
	require.NoError(t, s.SaveArtistAliases(ctx, other.ID, []string{"Artist"}, "MusicBrainz"))
	for _, id := range []int32{1, other.ID} {
		got, err := s.GetArtist(ctx, db.GetArtistOpts{ID: id})
		require.NoError(t, err)
		require.Empty(t, got.Genres)
		require.Empty(t, got.GenreEvidence)
	}
	var original string
	require.NoError(t, s.SourceDatabase().QueryRow(`SELECT value FROM source_genres WHERE record_id=?`, before.GenreEvidence[0].RecordID).Scan(&original))
	require.Equal(t, "Rock, Pop", original)
}

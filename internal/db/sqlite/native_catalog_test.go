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

func TestNativeCaptureAppliesCatalogNamesAliasesAndCreditsImmediately(t *testing.T) {
	s := musicStore(t)
	ctx := context.Background()
	store := sources.Store{DB: s.SourceDatabase(), Root: t.TempDir()}
	state, err := store.Filters(ctx)
	require.NoError(t, err)
	var program filters.Program
	require.NoError(t, json.Unmarshal([]byte(`{"steps":[
 {"if":{"op":"eq","args":[{"op":"field","field":"catalog.kind"},{"op":"literal","value":"artist"}]},"then":[{"set":"display.artist","value":{"op":"literal","value":"Displayed artist"}}]},
 {"if":{"op":"eq","args":[{"op":"field","field":"catalog.kind"},{"op":"literal","value":"album"}]},"then":[{"set":"display.album","value":{"op":"literal","value":"Displayed album"}}]},
 {"if":{"op":"and","args":[{"op":"eq","args":[{"op":"field","field":"catalog.kind"},{"op":"literal","value":"track"}]},{"op":"eq","args":[{"op":"field","field":"original.album"},{"op":"literal","value":"Incoming album"}]},{"op":"in","args":[{"op":"literal","value":"Incoming, artist"},{"op":"field","field":"original.artists"}]}]},"then":[
 {"set":"display.title","value":{"op":"literal","value":"Displayed song"}},
 {"set":"display.aliases","value":{"op":"literal","value":["Alternate song"]}},
 {"set":"display.artist_ids","value":{"op":"literal","value":[1]}},
 {"set":"display.primary_artist_ids","value":{"op":"literal","value":[1]}}
 ]} ]}`), &program))
	state.Current.Config.Rules = append(state.Current.Config.Rules, sources.Rule{ID: "incoming", Type: "presentation", Subtype: "naming", Enabled: true, Program: &program})
	_, err = store.Save(ctx, sources.FilterChange{BaseVersion: state.Current.ID, Config: state.Current.Config})
	require.NoError(t, err)
	artist, err := s.SaveArtist(ctx, db.SaveArtistOpts{Name: "Incoming, artist"})
	require.NoError(t, err)
	album, err := s.SaveAlbum(ctx, db.SaveAlbumOpts{Title: "Incoming album", ArtistIDs: []int32{artist.ID}})
	require.NoError(t, err)
	track, err := s.SaveTrack(ctx, db.SaveTrackOpts{Title: "Incoming song", AlbumID: album.ID, ArtistIDs: []int32{artist.ID}})
	require.NoError(t, err)
	check := func() {
		existing, e := s.GetArtist(ctx, db.GetArtistOpts{ID: 1})
		require.NoError(t, e)
		require.Equal(t, "Displayed artist", existing.Name)
		a, e := s.GetArtist(ctx, db.GetArtistOpts{ID: artist.ID})
		require.NoError(t, e)
		require.Equal(t, "Displayed artist", a.Name)
		r, e := s.GetAlbum(ctx, db.GetAlbumOpts{ID: album.ID})
		require.NoError(t, e)
		require.Equal(t, "Displayed album", r.Title)
		tr, e := s.GetTrack(ctx, db.GetTrackOpts{ID: track.ID})
		require.NoError(t, e)
		require.Equal(t, "Displayed song", tr.Title)
		require.Len(t, tr.Artists, 1)
		require.Equal(t, int32(1), tr.Artists[0].ID)
		var alias string
		require.NoError(t, s.SourceDatabase().QueryRow(`SELECT alias FROM catalog_aliases WHERE kind='track' AND item_id=?`, track.ID).Scan(&alias))
		require.Equal(t, "Alternate song", alias)
	}
	check()
	_, err = store.Rebuild(ctx)
	require.NoError(t, err)
	check()
	var original string
	require.NoError(t, s.SourceDatabase().QueryRow(`SELECT name FROM artists_with_name WHERE id=?`, artist.ID).Scan(&original))
	require.Equal(t, "Incoming, artist", original)
	require.NoError(t, s.SourceDatabase().QueryRow(`SELECT title FROM tracks_with_title WHERE id=?`, track.ID).Scan(&original))
	require.Equal(t, "Incoming song", original)
	state, err = store.Filters(ctx)
	require.NoError(t, err)
	state.Current.Config.Rules[len(state.Current.Config.Rules)-1].Enabled = false
	_, err = store.Save(ctx, sources.FilterChange{BaseVersion: state.Current.ID, Config: state.Current.Config})
	require.NoError(t, err)
	tr, err := s.GetTrack(ctx, db.GetTrackOpts{ID: track.ID})
	require.NoError(t, err)
	require.Equal(t, "Incoming song", tr.Title)
	require.Equal(t, artist.ID, tr.Artists[0].ID)
}

func TestNativeAliasArrivalRefreshesCatalogConditionsAndFailureRollsBack(t *testing.T) {
	s := musicStore(t)
	ctx := context.Background()
	store := sources.Store{DB: s.SourceDatabase(), Root: t.TempDir()}
	state, err := store.Filters(ctx)
	require.NoError(t, err)
	var program filters.Program
	require.NoError(t, json.Unmarshal([]byte(`{"steps":[{"if":{"op":"and","args":[{"op":"eq","args":[{"op":"field","field":"catalog.kind"},{"op":"literal","value":"artist"}]},{"op":"in","args":[{"op":"literal","value":"Later alias"},{"op":"field","field":"original.aliases"}]}]},"then":[{"set":"display.artist","value":{"op":"literal","value":"Corrected"}}]}]}`), &program))
	state.Current.Config.Rules = append(state.Current.Config.Rules, sources.Rule{ID: "aliases", Type: "presentation", Subtype: "naming", Enabled: true, Program: &program})
	_, err = store.Save(ctx, sources.FilterChange{BaseVersion: state.Current.ID, Config: state.Current.Config})
	require.NoError(t, err)
	artist, err := s.SaveArtist(ctx, db.SaveArtistOpts{Name: "Original"})
	require.NoError(t, err)
	a, err := s.GetArtist(ctx, db.GetArtistOpts{ID: artist.ID})
	require.NoError(t, err)
	require.Equal(t, "Original", a.Name)
	require.NoError(t, s.Exec(`CREATE TRIGGER reject_native_name BEFORE INSERT ON catalog_names BEGIN SELECT RAISE(ABORT,'correction rejected'); END`))
	err = s.SaveArtistAliases(ctx, artist.ID, []string{"Later alias"}, "MusicBrainz")
	require.ErrorContains(t, err, "correction rejected")
	var count int
	require.NoError(t, s.SourceDatabase().QueryRow(`SELECT COUNT(*) FROM artist_aliases WHERE artist_id=? AND alias='Later alias'`, artist.ID).Scan(&count))
	require.Zero(t, count)
	require.NoError(t, s.Exec(`DROP TRIGGER reject_native_name`))
	require.NoError(t, s.SaveArtistAliases(ctx, artist.ID, []string{"Later alias"}, "MusicBrainz"))
	a, err = s.GetArtist(ctx, db.GetArtistOpts{ID: artist.ID})
	require.NoError(t, err)
	require.Equal(t, "Corrected", a.Name)
}

func TestNativeIdentifierCorrectionPreservesIncomingIdentifier(t *testing.T) {
	s := musicStore(t)
	ctx := context.Background()
	store := sources.Store{DB: s.SourceDatabase(), Root: t.TempDir()}
	state, err := store.Filters(ctx)
	require.NoError(t, err)
	for i, rule := range state.Current.Config.Rules {
		if migrated, ok := state.Blocks.Legacy[rule.ID]; ok {
			state.Current.Config.Rules[i] = migrated
		}
	}
	var program filters.Program
	require.NoError(t, json.Unmarshal([]byte(`{"steps":[{"if":{"op":"eq","args":[{"op":"field","field":"catalog.kind"},{"op":"literal","value":"artist"}]},"then":[{"set":"music.identifier","value":{"op":"literal","value":"d9c172fb-2ae4-44d8-95bb-a84c50a5c591"}}]}]}`), &program))
	state.Current.Config.Rules = append(state.Current.Config.Rules, sources.Rule{ID: "identifier", Type: "matching", Subtype: "music", Enabled: true, Program: &program})
	_, err = store.Save(ctx, sources.FilterChange{BaseVersion: state.Current.ID, Config: state.Current.Config})
	require.NoError(t, err)
	original := uuid.MustParse("c9c172fb-2ae4-44d8-95bb-a84c50a5c591")
	a, err := s.SaveArtist(ctx, db.SaveArtistOpts{Name: "Incoming", MusicBrainzID: original})
	require.NoError(t, err)
	got, err := s.GetArtist(ctx, db.GetArtistOpts{ID: a.ID})
	require.NoError(t, err)
	require.Equal(t, "d9c172fb-2ae4-44d8-95bb-a84c50a5c591", got.MbzID.String())
	var raw string
	require.NoError(t, s.SourceDatabase().QueryRow(`SELECT musicbrainz_id FROM artists WHERE id=?`, a.ID).Scan(&raw))
	require.Equal(t, original.String(), raw)
}

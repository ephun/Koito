package sqlite_test

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gabehf/koito/internal/db"
	"github.com/gabehf/koito/internal/filters"
	"github.com/gabehf/koito/internal/sources"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestCatalogCorrectionScopesUseCompleteArtistCredits(t *testing.T) {
	s := musicStore(t)
	ctx := context.Background()
	require.NoError(t, s.Exec(`UPDATE artist_aliases SET alias='Artist, with comma' WHERE artist_id=1;INSERT INTO artists(id) VALUES(2);INSERT INTO artist_aliases VALUES(2,'Other','test',1);INSERT INTO artist_releases VALUES(2,1,0)`))
	store := sources.Store{DB: s.SourceDatabase(), Root: t.TempDir()}
	state, err := store.Filters(ctx)
	require.NoError(t, err)
	var program filters.Program
	require.NoError(t, json.Unmarshal([]byte(`{"steps":[{"if":{"op":"and","args":[{"op":"eq","args":[{"op":"field","field":"catalog.kind"},{"op":"literal","value":"album"}]},{"op":"in","args":[{"op":"literal","value":2},{"op":"field","field":"catalog.artist_ids"}]},{"op":"eq","args":[{"op":"count","args":[{"op":"field","field":"original.artists"}]},{"op":"literal","value":2}]}]},"then":[{"set":"display.album","value":{"op":"literal","value":"Duet"}}]}]}`), &program))
	state.Current.Config.Rules = append(state.Current.Config.Rules, sources.Rule{ID: "duet", Type: "presentation", Subtype: "naming", Enabled: true, Program: &program})
	_, err = store.Save(ctx, sources.FilterChange{BaseVersion: state.Current.ID, Config: state.Current.Config})
	require.NoError(t, err)
	album, err := s.GetAlbum(ctx, db.GetAlbumOpts{ID: 1})
	require.NoError(t, err)
	require.Equal(t, "Duet", album.Title)
	require.Len(t, album.Artists, 2)
	other, err := s.GetAlbum(ctx, db.GetAlbumOpts{ID: 2})
	require.NoError(t, err)
	require.Equal(t, "Compilation", other.Title)
}

func TestInvalidCatalogNameRetainsOriginalAndExposesIssue(t *testing.T) {
	s := musicStore(t)
	ctx := context.Background()
	store := sources.Store{DB: s.SourceDatabase(), Root: t.TempDir()}
	state, err := store.Filters(ctx)
	require.NoError(t, err)
	var program filters.Program
	require.NoError(t, json.Unmarshal([]byte(`{"steps":[{"if":{"op":"eq","args":[{"op":"field","field":"catalog.kind"},{"op":"literal","value":"artist"}]},"then":[{"set":"display.artist","value":{"op":"literal","value":""}}]}]}`), &program))
	state.Current.Config.Rules = append(state.Current.Config.Rules, sources.Rule{ID: "empty-name", Type: "presentation", Subtype: "naming", Enabled: true, Program: &program})
	_, err = store.Save(ctx, sources.FilterChange{BaseVersion: state.Current.ID, Config: state.Current.Config})
	require.NoError(t, err)
	artist, err := s.GetArtist(ctx, db.GetArtistOpts{ID: 1})
	require.NoError(t, err)
	require.Equal(t, "Artist", artist.Name)
	require.Len(t, artist.NameIssues, 1)
	require.Equal(t, "empty-name", artist.NameIssues[0].FilterID)
}

func TestCatalogNamesReachDetailsHistoryAndChartsWithoutChangingAliases(t *testing.T) {
	s := musicStore(t)
	ctx := context.Background()
	track := musicAppearance(t, s, "Song", 1, uuid.Nil)
	require.NoError(t, s.SaveListen(ctx, db.SaveListenOpts{TrackID: track.ID, Time: time.Unix(1577934065, 0), UserID: 1}))
	store := sources.Store{DB: s.SourceDatabase(), Root: t.TempDir()}
	state, err := store.Filters(ctx)
	require.NoError(t, err)
	for _, item := range []struct {
		kind, field, name string
		id                int32
	}{{"artist", "artist", "New artist", 1}, {"album", "album", "New album", 1}, {"track", "title", "New song", track.ID}} {
		var program filters.Program
		raw := fmt.Sprintf(`{"steps":[{"if":{"op":"and","args":[{"op":"eq","args":[{"op":"field","field":"catalog.kind"},{"op":"literal","value":%q}]},{"op":"eq","args":[{"op":"field","field":"catalog.id"},{"op":"literal","value":%d}]}]},"then":[{"set":%q,"value":{"op":"literal","value":%q}}]}]}`, item.kind, item.id, "display."+item.field, item.name)
		require.NoError(t, json.Unmarshal([]byte(raw), &program))
		state.Current.Config.Rules = append(state.Current.Config.Rules, sources.Rule{ID: item.kind + "-name", Type: "presentation", Subtype: "naming", Enabled: true, Program: &program})
	}
	_, err = store.Save(ctx, sources.FilterChange{BaseVersion: state.Current.ID, Config: state.Current.Config})
	require.NoError(t, err)
	artist, err := s.GetArtist(ctx, db.GetArtistOpts{ID: 1})
	require.NoError(t, err)
	require.Equal(t, "New artist", artist.Name)
	album, err := s.GetAlbum(ctx, db.GetAlbumOpts{ID: 1})
	require.NoError(t, err)
	require.Equal(t, "New album", album.Title)
	require.Equal(t, "New artist", album.Artists[0].Name)
	viewed, err := s.GetTrack(ctx, db.GetTrackOpts{ID: track.ID})
	require.NoError(t, err)
	require.Equal(t, "New song", viewed.Title)
	require.Equal(t, "New album", viewed.Albums[0].Title)
	opts := db.GetItemsOpts{Page: 1, Timeframe: db.Timeframe{Period: db.PeriodAllTime}}
	history, err := s.GetListensPaginated(ctx, opts)
	require.NoError(t, err)
	require.Equal(t, "New song", history.Items[0].Track.Title)
	require.Equal(t, "New album", history.Items[0].Album)
	tracks, err := s.GetTopTracksPaginated(ctx, opts)
	require.NoError(t, err)
	require.Equal(t, "New song", tracks.Items[0].Item.Title)
	albums, err := s.GetTopAlbumsPaginated(ctx, opts)
	require.NoError(t, err)
	require.Equal(t, "New album", albums.Items[0].Item.Title)
	artists, err := s.GetTopArtistsPaginated(ctx, opts)
	require.NoError(t, err)
	require.Equal(t, "New artist", artists.Items[0].Item.Name)
	var original string
	require.NoError(t, s.SourceDatabase().QueryRow(`SELECT title FROM tracks_with_title WHERE id=?`, track.ID).Scan(&original))
	require.Equal(t, "Song", original)
	require.EqualValues(t, 1, history.TotalCount)
	for _, query := range []string{"Artist", "New artist"} {
		found, err := s.SearchArtists(ctx, query)
		require.NoError(t, err)
		require.Len(t, found, 1)
		require.Equal(t, "New artist", found[0].Name)
	}
	for _, query := range []string{"Single", "New album"} {
		found, err := s.SearchAlbums(ctx, query)
		require.NoError(t, err)
		require.Len(t, found, 1)
		require.Equal(t, "New album", found[0].Title)
	}
	for _, query := range []string{"Song", "New song"} {
		found, err := s.SearchTracks(ctx, query)
		require.NoError(t, err)
		require.Len(t, found, 1)
		require.Equal(t, "New song", found[0].Title)
	}
	state, err = store.Filters(ctx)
	require.NoError(t, err)
	for i := range state.Current.Config.Rules {
		if state.Current.Config.Rules[i].Subtype == "naming" {
			state.Current.Config.Rules[i].Enabled = false
		}
	}
	_, err = store.Save(ctx, sources.FilterChange{BaseVersion: state.Current.ID, Config: state.Current.Config})
	require.NoError(t, err)
	found, err := s.SearchTracks(ctx, "New song")
	require.NoError(t, err)
	require.Empty(t, found)
	found, err = s.SearchTracks(ctx, "Song")
	require.NoError(t, err)
	require.Len(t, found, 1)
	require.Equal(t, "Song", found[0].Title)
}

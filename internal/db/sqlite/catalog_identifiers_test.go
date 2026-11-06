package sqlite_test

import (
	"context"
	"encoding/json"
	"github.com/gabehf/koito/internal/db"
	"github.com/gabehf/koito/internal/filters"
	"github.com/gabehf/koito/internal/sources"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestCatalogIdentifierViewCorrectionAndClearRetainOriginal(t *testing.T) {
	s := musicStore(t)
	ctx := context.Background()
	original := "c9c172fb-2ae4-44d8-95bb-a84c50a5c591"
	require.NoError(t, s.Exec(`UPDATE artists SET musicbrainz_id=? WHERE id=1`, original))
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
	for _, value := range []string{"d9c172fb-2ae4-44d8-95bb-a84c50a5c591", ""} {
		raw, _ := json.Marshal(value)
		state.Current.Config.Rules[len(state.Current.Config.Rules)-1].Program.Steps[0].Then[0].Value.Value = raw
		version, err := store.Save(ctx, sources.FilterChange{BaseVersion: state.Current.ID, Config: state.Current.Config})
		require.NoError(t, err)
		state.Current.ID = version.ID
		artist, err := s.GetArtist(ctx, db.GetArtistOpts{ID: 1})
		require.NoError(t, err)
		if value == "" {
			require.Nil(t, artist.MbzID)
		} else {
			require.Equal(t, value, artist.MbzID.String())
		}
		var stored string
		require.NoError(t, s.SourceDatabase().QueryRow(`SELECT musicbrainz_id FROM artists WHERE id=1`).Scan(&stored))
		require.Equal(t, original, stored)
	}
}

func TestCatalogCorrectionsAgreeAcrossSearchChartsAndArtistCredits(t *testing.T) {
	s := musicStore(t)
	ctx := context.Background()
	track := musicAppearance(t, s, "Song", 1, uuid.Nil)
	require.NoError(t, s.SaveListen(ctx, db.SaveListenOpts{TrackID: track.ID, Time: time.Unix(1577934065, 0), UserID: 1, Client: "test"}))
	identifier := "d9c172fb-2ae4-44d8-95bb-a84c50a5c591"
	for _, kind := range []string{"artist", "album", "track"} {
		require.NoError(t, s.Exec(`INSERT INTO catalog_identifiers(kind,item_id,identifier) VALUES(?,1,?)`, kind, identifier))
	}
	require.NoError(t, s.Exec(`INSERT INTO album_classification VALUES(1,1)`))
	artists, err := s.SearchArtists(ctx, "Artist")
	require.NoError(t, err)
	require.NotEmpty(t, artists)
	require.Equal(t, identifier, artists[0].MbzID.String())
	albums, err := s.SearchAlbums(ctx, "Single")
	require.NoError(t, err)
	require.NotEmpty(t, albums)
	require.Equal(t, identifier, albums[0].MbzID.String())
	require.True(t, albums[0].VariousArtists)
	tracks, err := s.SearchTracks(ctx, "Song")
	require.NoError(t, err)
	require.NotEmpty(t, tracks)
	require.Equal(t, identifier, tracks[0].MbzID.String())
	opts := db.GetItemsOpts{Page: 1, Timeframe: db.Timeframe{Period: db.PeriodAllTime}}
	artistChart, err := s.GetTopArtistsPaginated(ctx, opts)
	require.NoError(t, err)
	require.Equal(t, identifier, artistChart.Items[0].Item.MbzID.String())
	albumChart, err := s.GetTopAlbumsPaginated(ctx, opts)
	require.NoError(t, err)
	for _, item := range albumChart.Items {
		if item.Item.ID == 1 {
			require.Equal(t, identifier, item.Item.MbzID.String())
			require.True(t, item.Item.VariousArtists)
		}
	}
	trackChart, err := s.GetTopTracksPaginated(ctx, opts)
	require.NoError(t, err)
	require.Equal(t, identifier, trackChart.Items[0].Item.MbzID.String())
	credits, err := s.GetArtistsForAlbum(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, identifier, credits[0].MbzID.String())
	require.NoError(t, s.Exec(`UPDATE catalog_identifiers SET identifier=''`))
	artists, err = s.SearchArtists(ctx, "Artist")
	require.NoError(t, err)
	require.Nil(t, artists[0].MbzID)
}

package sqlite_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gabehf/koito/internal/db"
	"github.com/gabehf/koito/internal/db/sqlite"
	"github.com/gabehf/koito/internal/sources"
	"github.com/stretchr/testify/require"
)

func TestUndatedTotalsRankingsAndCalendarBoundaries(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	catalog, err := sqlite.NewInMemory()
	require.NoError(t, err)
	defer catalog.Close(context.Background())
	require.NoError(t, catalog.Exec(`INSERT INTO users(id,username,role,password) VALUES(1,'test','admin',x'00')`))
	store := &sources.Store{DB: catalog.SourceDatabase(), Root: t.TempDir()}
	input := `{"Library Persistent ID":"LIBRARY","Date":"2020-01-09T00:00:00Z","Tracks":{"1":{"Track ID":1,"Persistent ID":"A","Name":"First","Artist":"Artist","Album":"Album","Play Count":7,"Date Added":"2020-01-01T00:00:00Z","Play Date UTC":"2020-01-08T00:00:00Z"},"2":{"Track ID":2,"Persistent ID":"B","Name":"Second","Artist":"Artist","Album":"Album","Play Count":3,"Date Added":"2020-01-01T00:00:00Z","Play Date UTC":"2020-01-08T00:00:00Z"}}}`
	_, err = store.Import(ctx, strings.NewReader(input), "library.json", 1)
	require.NoError(t, err)
	state, err := store.Filters(ctx)
	require.NoError(t, err)
	config := state.Current.Config
	config.Rules = append(config.Rules,
		sources.Rule{ID: "count", Type: "inference", Subtype: "count", Enabled: true, Scope: sources.RuleScope{Service: "itunes"}, Settings: sources.RuleSettings{CountMode: "maximum"}},
		sources.Rule{ID: "listener", Type: "attribution", Subtype: "direct", Enabled: true, Scope: sources.RuleScope{Service: "itunes"}, Settings: sources.RuleSettings{Listener: "self"}})
	_, err = store.Save(ctx, sources.FilterChange{BaseVersion: state.Current.ID, Config: config})
	require.NoError(t, err)
	all := db.Timeframe{Period: db.PeriodAllTime, Timezone: time.UTC}
	bounded := db.Timeframe{Period: db.PeriodAllTime, Year: 2020, Timezone: time.UTC}
	count, err := catalog.CountListens(ctx, all)
	require.NoError(t, err)
	require.EqualValues(t, 10, count)
	count, err = catalog.CountUndatedListens(ctx, all)
	require.NoError(t, err)
	require.EqualValues(t, 10, count)
	count, err = catalog.CountListens(ctx, bounded)
	require.NoError(t, err)
	require.Zero(t, count)
	count, err = catalog.CountUndatedListens(ctx, bounded)
	require.NoError(t, err)
	require.Zero(t, count)
	count, err = catalog.CountTracks(ctx, all)
	require.NoError(t, err)
	require.EqualValues(t, 2, count)
	count, err = catalog.CountAlbums(ctx, all)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	count, err = catalog.CountArtists(ctx, all)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	tracks, err := catalog.GetTopTracksPaginated(ctx, db.GetItemsOpts{Timeframe: all, Page: 1, Limit: 1})
	require.NoError(t, err)
	require.EqualValues(t, 2, tracks.TotalCount)
	require.True(t, tracks.HasNextPage)
	require.Len(t, tracks.Items, 1)
	require.EqualValues(t, 7, tracks.Items[0].Item.ListenCount)
	trackID := tracks.Items[0].Item.ID
	albumID := tracks.Items[0].Item.AlbumID
	artistID := tracks.Items[0].Item.Artists[0].ID
	track, err := catalog.GetTrack(ctx, db.GetTrackOpts{ID: trackID})
	require.NoError(t, err)
	require.EqualValues(t, 7, track.ListenCount)
	require.Zero(t, track.FirstListen)
	require.EqualValues(t, 1, track.AllTimeRank)
	for _, ids := range []db.GetItemsOpts{{ArtistID: int(artistID)}, {AlbumID: int(albumID)}} {
		ids.Timeframe = all
		ids.Page = 1
		ids.Limit = 10
		page, err := catalog.GetTopTracksPaginated(ctx, ids)
		require.NoError(t, err)
		require.Len(t, page.Items, 2)
	}
	albums, err := catalog.GetTopAlbumsPaginated(ctx, db.GetItemsOpts{Timeframe: all, Page: 1, Limit: 10})
	require.NoError(t, err)
	require.Len(t, albums.Items, 1)
	require.EqualValues(t, 10, albums.Items[0].Item.ListenCount)
	artists, err := catalog.GetTopArtistsPaginated(ctx, db.GetItemsOpts{Timeframe: all, Page: 1, Limit: 10})
	require.NoError(t, err)
	require.Len(t, artists.Items, 1)
	require.EqualValues(t, 10, artists.Items[0].Item.ListenCount)
	for _, opts := range []db.TimeListenedOpts{{Timeframe: all, ArtistID: artistID}, {Timeframe: all, AlbumID: albumID}, {Timeframe: all, TrackID: trackID}} {
		count, err := catalog.CountListensToItem(ctx, opts)
		require.NoError(t, err)
		if opts.TrackID > 0 {
			require.EqualValues(t, 7, count)
		} else {
			require.EqualValues(t, 10, count)
		}
	}
	listens, err := catalog.GetListensPaginated(ctx, db.GetItemsOpts{Timeframe: all, Page: 1, Limit: 100})
	require.NoError(t, err)
	require.Empty(t, listens.Items)
	active, err := catalog.GetActiveDays(ctx, time.UTC)
	require.NoError(t, err)
	require.Zero(t, active)
	for _, counter := range []func(context.Context, db.Timeframe) (int64, error){catalog.CountNewTracks, catalog.CountNewAlbums, catalog.CountNewArtists} {
		count, err := counter(ctx, bounded)
		require.NoError(t, err)
		require.Zero(t, count)
	}
	seconds, err := catalog.CountTimeListened(ctx, all)
	require.NoError(t, err)
	require.Zero(t, seconds)
	state, err = store.Filters(ctx)
	require.NoError(t, err)
	config = state.Current.Config
	config.Rules = append(config.Rules, sources.Rule{ID: "time", Type: "inference", Subtype: "time", Enabled: true, Scope: sources.RuleScope{Service: "itunes"}, Settings: sources.RuleSettings{UseLastPlayed: true, EstimateTimes: true}})
	_, err = store.Save(ctx, sources.FilterChange{BaseVersion: state.Current.ID, Config: config})
	require.NoError(t, err)
	count, err = catalog.CountListens(ctx, all)
	require.NoError(t, err)
	require.EqualValues(t, 10, count)
	count, err = catalog.CountUndatedListens(ctx, all)
	require.NoError(t, err)
	require.Zero(t, count)
	listens, err = catalog.GetListensPaginated(ctx, db.GetItemsOpts{Timeframe: all, Page: 1, Limit: 100})
	require.NoError(t, err)
	require.Len(t, listens.Items, 10)
	for _, listen := range listens.Items {
		require.Contains(t, []string{"estimated", "last_played"}, listen.Evidence)
		require.Equal(t, "direct", listen.AttributionBasis)
	}
	count, err = catalog.CountListens(ctx, bounded)
	require.NoError(t, err)
	require.EqualValues(t, 10, count)
	count, err = catalog.CountNewTracks(ctx, bounded)
	require.NoError(t, err)
	require.EqualValues(t, 2, count)
}

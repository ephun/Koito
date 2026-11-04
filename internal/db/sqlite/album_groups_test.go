package sqlite_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/gabehf/koito/internal/db"
	"github.com/gabehf/koito/internal/filters"
	"github.com/gabehf/koito/internal/sources"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestAlbumGroupsAcrossChartsHistoryActivityAndIncomingReleases(t *testing.T) {
	s := musicStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	for i, album := range []int32{1, 2} {
		track := musicAppearance(t, s, "Song", album, uuid.Nil)
		require.NoError(t, s.SaveListen(ctx, db.SaveListenOpts{TrackID: track.ID, Time: time.Unix(1577934065+int64(i), 0), UserID: 1}))
	}
	store := sources.Store{DB: s.SourceDatabase(), Root: t.TempDir()}
	state, err := store.Filters(ctx)
	require.NoError(t, err)
	var program filters.Program
	require.NoError(t, json.Unmarshal([]byte(`{"steps":[{"set":"group.album","value":{"op":"literal","value":"Album"}}]}`), &program))
	state.Current.Config.Rules = append(state.Current.Config.Rules, sources.Rule{ID: "editions", Type: "presentation", Subtype: "grouping", Enabled: true, Program: &program})
	_, err = store.Save(ctx, sources.FilterChange{BaseVersion: state.Current.ID, Config: state.Current.Config})
	require.NoError(t, err)
	all := db.Timeframe{Period: db.PeriodAllTime, Timezone: time.UTC}
	top, err := s.GetTopAlbumsPaginated(ctx, db.GetItemsOpts{Timeframe: all, Page: 1})
	require.NoError(t, err)
	require.Len(t, top.Items, 1)
	groupID := top.Items[0].Item.AlbumGroupID
	require.Positive(t, groupID)
	require.Equal(t, "Album", top.Items[0].Item.Title)
	require.EqualValues(t, 2, top.Items[0].Item.ListenCount)
	require.Nil(t, top.Items[0].Item.MbzID)
	group, err := s.GetAlbumGroup(ctx, groupID)
	require.NoError(t, err)
	require.EqualValues(t, 2, group.ListenCount)
	require.Len(t, group.Editions, 3)
	require.EqualValues(t, 360, group.TimeListened)
	for _, tf := range []db.Timeframe{all, {Period: db.PeriodAllTime, Year: 2020, Timezone: time.UTC}} {
		tracks, err := s.GetTopTracksPaginated(ctx, db.GetItemsOpts{Timeframe: tf, AlbumGroupID: int(groupID), Page: 1})
		require.NoError(t, err)
		require.Len(t, tracks.Items, 1)
		require.EqualValues(t, 2, tracks.Items[0].Item.ListenCount)
		count, err := s.CountListensToItem(ctx, db.TimeListenedOpts{Timeframe: tf, AlbumGroupID: groupID})
		require.NoError(t, err)
		require.EqualValues(t, 2, count)
	}
	count, err := s.CountAlbums(ctx, all)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	count, err = s.CountNewAlbums(ctx, db.Timeframe{Period: db.PeriodAllTime, Year: 2020, Timezone: time.UTC})
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	listens, err := s.GetListensPaginated(ctx, db.GetItemsOpts{Timeframe: all, AlbumGroupID: int(groupID), Page: 1})
	require.NoError(t, err)
	require.Len(t, listens.Items, 2)
	require.NotEqual(t, listens.Items[0].AlbumID, listens.Items[1].AlbumID)
	activity, err := s.GetListenActivity(ctx, db.ListenActivityOpts{AlbumGroupID: groupID, Year: 2020, Timezone: time.UTC})
	require.NoError(t, err)
	var activityTotal int64
	for _, day := range activity {
		activityTotal += day.Listens
	}
	require.EqualValues(t, 2, activityTotal)
	interest, err := s.GetInterest(ctx, db.GetInterestOpts{AlbumGroupID: groupID, Buckets: 4})
	require.NoError(t, err)
	var interestTotal int64
	for _, bucket := range interest {
		interestTotal += bucket.ListenCount
	}
	require.EqualValues(t, 2, interestTotal)
	// A new live release follows the enabled grouping program immediately.
	release, err := s.SaveAlbum(ctx, db.SaveAlbumOpts{Title: "Album (Expanded)", ArtistIDs: []int32{1}})
	require.NoError(t, err)
	track := musicAppearance(t, s, "Song", release.ID, uuid.Nil)
	require.NoError(t, s.SaveListen(ctx, db.SaveListenOpts{TrackID: track.ID, Time: time.Unix(1577934165, 0), UserID: 1}))
	group, err = s.GetAlbumGroup(ctx, groupID)
	require.NoError(t, err)
	require.EqualValues(t, 3, group.ListenCount)
	// Complete artist credits keep unrelated releases in separate groups.
	require.NoError(t, s.Exec(`INSERT INTO artists(id) VALUES(2);INSERT INTO artist_aliases VALUES(2,'Other','test',1)`))
	other, err := s.SaveAlbum(ctx, db.SaveAlbumOpts{Title: "Album", ArtistIDs: []int32{2}})
	require.NoError(t, err)
	otherTrack, err := s.SaveTrack(ctx, db.SaveTrackOpts{Title: "Song", AlbumID: other.ID, ArtistIDs: []int32{2}})
	require.NoError(t, err)
	require.NoError(t, s.SaveListen(ctx, db.SaveListenOpts{TrackID: otherTrack.ID, Time: time.Unix(1577934265, 0), UserID: 1}))
	top, err = s.GetTopAlbumsPaginated(ctx, db.GetItemsOpts{Timeframe: all, Page: 1})
	require.NoError(t, err)
	require.Len(t, top.Items, 2)
	top, err = s.GetTopAlbumsPaginated(ctx, db.GetItemsOpts{Timeframe: all, ArtistID: 2, Page: 1})
	require.NoError(t, err)
	require.Len(t, top.Items, 1)
	require.EqualValues(t, 1, top.Items[0].Item.ListenCount)
	state, err = store.Filters(ctx)
	require.NoError(t, err)
	state.Current.Config.Rules[len(state.Current.Config.Rules)-1].Enabled = false
	_, err = store.Save(ctx, sources.FilterChange{BaseVersion: state.Current.ID, Config: state.Current.Config})
	require.NoError(t, err)
	top, err = s.GetTopAlbumsPaginated(ctx, db.GetItemsOpts{Timeframe: all, Page: 1})
	require.NoError(t, err)
	require.Len(t, top.Items, 4)
	_, err = s.GetAlbumGroup(ctx, groupID)
	require.ErrorIs(t, err, db.ErrNotFound)
	for _, id := range []int32{1, 2} {
		album, err := s.GetAlbum(ctx, db.GetAlbumOpts{ID: id})
		require.NoError(t, err)
		require.EqualValues(t, 1, album.ListenCount)
	}
	var originals int
	require.NoError(t, s.SourceDatabase().QueryRow(`SELECT COUNT(*) FROM release_aliases WHERE alias IN ('Single','Compilation')`).Scan(&originals))
	require.Equal(t, 2, originals)
}

func TestIncomingEditionJoinsExistingBaseAndNativePreviewShowsChanges(t *testing.T) {
	s := musicStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	base := musicAppearance(t, s, "Song", 1, uuid.Nil)
	require.NoError(t, s.SaveListen(ctx, db.SaveListenOpts{TrackID: base.ID, Time: time.Unix(1577934065, 0), UserID: 1}))
	store := sources.Store{DB: s.SourceDatabase(), Root: t.TempDir()}
	state, err := store.Filters(ctx)
	require.NoError(t, err)
	template := state.Blocks.Templates["presentation:grouping"][0]
	state.Current.Config.Rules = append(state.Current.Config.Rules, sources.Rule{ID: "edition", Type: "presentation", Subtype: "grouping", Enabled: true, Program: template.Program})
	_, err = store.Save(ctx, sources.FilterChange{BaseVersion: state.Current.ID, Config: state.Current.Config})
	require.NoError(t, err)
	incoming, err := s.SaveAlbum(ctx, db.SaveAlbumOpts{Title: "Single (Deluxe)", ArtistIDs: []int32{1}})
	require.NoError(t, err)
	track := musicAppearance(t, s, "Song", incoming.ID, uuid.Nil)
	require.NoError(t, s.SaveListen(ctx, db.SaveListenOpts{TrackID: track.ID, Time: time.Unix(1577934165, 0), UserID: 1}))
	all := db.Timeframe{Period: db.PeriodAllTime, Timezone: time.UTC}
	chart, err := s.GetTopAlbumsPaginated(ctx, db.GetItemsOpts{Timeframe: all, Page: 1})
	require.NoError(t, err)
	require.Len(t, chart.Items, 1)
	require.EqualValues(t, 2, chart.Items[0].Item.ListenCount)
	require.EqualValues(t, 1, chart.Items[0].Item.ID)
	group, err := s.GetAlbumGroup(ctx, chart.Items[0].Item.AlbumGroupID)
	require.NoError(t, err)
	require.Len(t, group.Editions, 2)
	state, err = store.Filters(ctx)
	require.NoError(t, err)
	state.Current.Config.Rules[len(state.Current.Config.Rules)-1].Enabled = false
	preview, err := store.Preview(ctx, sources.FilterChange{BaseVersion: state.Current.ID, Config: state.Current.Config})
	require.NoError(t, err)
	require.Contains(t, preview.GroupingExamples, sources.NamingExample{Field: "Album group", Before: "Single", After: "Single (Deluxe)"})
}

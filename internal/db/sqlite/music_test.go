package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/gabehf/koito/internal/catalog"
	"github.com/gabehf/koito/internal/db"
	"github.com/gabehf/koito/internal/db/sqlite"
	"github.com/gabehf/koito/internal/mbz"
	"github.com/gabehf/koito/internal/models"
	"github.com/gabehf/koito/internal/music"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func musicStore(t *testing.T) *sqlite.Sqlite {
	t.Helper()
	s, err := sqlite.NewInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { s.Close(context.Background()) })
	require.NoError(t, s.Exec(`INSERT INTO users VALUES(1,'test','admin',x'00');
	    INSERT INTO artists(id) VALUES(1); INSERT INTO artist_aliases VALUES(1,'Artist','test',1);
	    INSERT INTO releases(id) VALUES(1),(2),(3);
	    INSERT INTO release_aliases VALUES(1,'Single','test',1),(2,'Compilation','test',1),(3,'Unknown Album','test',1);
	    INSERT INTO artist_releases VALUES(1,1,1),(1,2,1),(1,3,1);`))
	return s
}

func musicAppearance(t *testing.T, s *sqlite.Sqlite, title string, album int32, recording uuid.UUID) *models.Track {
	t.Helper()
	v, err := s.SaveTrack(context.Background(), db.SaveTrackOpts{Title: title, AlbumID: album, ArtistIDs: []int32{1}, RecordingMbzID: recording, Duration: 180})
	require.NoError(t, err)
	return v
}

func TestMusicHistorySpansAlbumsWithoutCombiningPlays(t *testing.T) {
	s := musicStore(t)
	ctx := context.Background()
	mbid := uuid.MustParse("c9c172fb-2ae4-44d8-95bb-a84c50a5c591")
	a := musicAppearance(t, s, "Song", 1, mbid)
	b := musicAppearance(t, s, "Song", 2, mbid)
	c := musicAppearance(t, s, "Song", 3, uuid.Nil)
	for i, v := range []*models.Track{a, b, c} {
		require.NoError(t, s.SaveListen(ctx, db.SaveListenOpts{TrackID: v.ID, Time: time.Unix(1577934065+int64(i)*86400, 0), UserID: 1, Client: "test"}))
	}
	all := db.Timeframe{Period: db.PeriodAllTime}
	for _, v := range []*models.Track{a, b, c} {
		track, err := s.GetTrack(ctx, db.GetTrackOpts{ID: v.ID})
		require.NoError(t, err)
		require.EqualValues(t, 3, track.ListenCount)
		require.EqualValues(t, 540, track.TimeListened)
		require.EqualValues(t, 1, track.AllTimeRank)
		require.Len(t, track.Albums, 3)
		require.Len(t, track.Recordings, 2, "unidentified appearance must not acquire the shared recording ID")
		for _, album := range track.Albums {
			require.EqualValues(t, 1, album.ListenCount)
		}
		page, err := s.GetListensPaginated(ctx, db.GetItemsOpts{TrackID: int(v.ID), Page: 1, Limit: 10, Timeframe: all})
		require.NoError(t, err)
		require.EqualValues(t, 3, page.TotalCount)
		require.Len(t, page.Items, 3)
		seen := map[int32]string{}
		for _, play := range page.Items {
			seen[play.AlbumID] = play.Album
		}
		require.Equal(t, map[int32]string{1: "Single", 2: "Compilation", 3: "Unknown Album"}, seen)
		n, err := s.CountListensToItem(ctx, db.TimeListenedOpts{TrackID: v.ID, Timeframe: all})
		require.NoError(t, err)
		require.EqualValues(t, 3, n)
		interest, err := s.GetInterest(ctx, db.GetInterestOpts{TrackID: v.ID, Buckets: 10})
		require.NoError(t, err)
		var sum int64
		for _, bucket := range interest {
			sum += bucket.ListenCount
		}
		require.EqualValues(t, 3, sum)
		activity, err := s.GetListenActivity(ctx, db.ListenActivityOpts{TrackID: v.ID, Year: 2020, Timezone: time.UTC})
		require.NoError(t, err)
		sum = 0
		for _, day := range activity {
			sum += day.Listens
		}
		require.EqualValues(t, 3, sum)
	}
	for _, v := range []*models.Track{a, b, c} {
		n, err := s.CountListensToItem(ctx, db.TimeListenedOpts{AlbumID: v.AlbumID, Timeframe: all})
		require.NoError(t, err)
		require.EqualValues(t, 1, n)
		chart, err := s.GetTopTracksPaginated(ctx, db.GetItemsOpts{AlbumID: int(v.AlbumID), Page: 1, Timeframe: all})
		require.NoError(t, err)
		require.Len(t, chart.Items, 1)
		require.Equal(t, v.AlbumID, chart.Items[0].Item.AlbumID)
	}
	exported, err := s.GetExportPage(ctx, db.GetExportPageOpts{UserID: 1, ListenedAt: time.Unix(0, 0), Limit: 100})
	require.NoError(t, err)
	require.Len(t, exported, 3)
	for _, play := range exported {
		if play.ReleaseID == 3 {
			require.Nil(t, play.TrackMbid)
		} else {
			require.NotNil(t, play.TrackMbid)
			require.Equal(t, mbid, *play.TrackMbid)
		}
	}
	top, err := s.GetTopTracksPaginated(ctx, db.GetItemsOpts{Page: 1, Timeframe: all})
	require.NoError(t, err)
	require.Len(t, top.Items, 1)
	require.EqualValues(t, 3, top.Items[0].Item.ListenCount)
	n, err := s.CountTracks(ctx, all)
	require.NoError(t, err)
	require.EqualValues(t, 1, n)
	search, err := s.SearchTracks(ctx, "Song")
	require.NoError(t, err)
	require.Len(t, search, 1)
	for _, v := range []*models.Track{a, b} {
		found, err := s.GetTrack(ctx, db.GetTrackOpts{MusicBrainzID: mbid, ReleaseID: v.AlbumID})
		require.NoError(t, err)
		require.Equal(t, v.ID, found.ID)
	}
}

func TestLiveRecordingKeepsEachSubmittedAlbum(t *testing.T) {
	s := musicStore(t)
	ctx := context.Background()
	mbid := uuid.MustParse("c9c172fb-2ae4-44d8-95bb-a84c50a5c591")
	for i, album := range []string{"Single", "Compilation", "Compilation", ""} {
		require.NoError(t, catalog.SubmitListen(ctx, s, catalog.SubmitListenOpts{
			Artist: "Artist", ArtistNames: []string{"Artist"}, TrackTitle: "Song", ReleaseTitle: album, RecordingMbzID: mbid,
			UserID: 1, Time: time.Unix(1577934065+int64(i)*86400, 0), MbzCaller: &mbz.MbzErrorCaller{}, SkipCacheImage: true,
		}))
	}
	page, err := s.GetListensPaginated(ctx, db.GetItemsOpts{Page: 1, Timeframe: db.Timeframe{Period: db.PeriodAllTime}})
	require.NoError(t, err)
	require.Len(t, page.Items, 4)
	require.Equal(t, []string{"Unknown Album", "Compilation", "Compilation", "Single"}, []string{page.Items[0].Album, page.Items[1].Album, page.Items[2].Album, page.Items[3].Album})
	require.Equal(t, page.Items[1].Track.ID, page.Items[2].Track.ID, "repeated submissions reuse the same appearance")
	track, err := s.GetTrack(ctx, db.GetTrackOpts{ID: page.Items[0].Track.ID})
	require.NoError(t, err)
	require.Len(t, track.Recordings, 1)
	require.EqualValues(t, 4, track.ListenCount)
	for _, album := range track.Albums {
		want := int64(1)
		if album.Title == "Compilation" {
			want = 2
		}
		require.Equal(t, want, album.ListenCount)
	}
}

func TestNameHistoryKeepsRecordingVariantsDistinct(t *testing.T) {
	s := musicStore(t)
	ctx := context.Background()
	for i, mbid := range []string{"c9c172fb-2ae4-44d8-95bb-a84c50a5c591", "c9c172fb-2ae4-44d8-95bb-a84c50a5c592"} {
		require.NoError(t, catalog.SubmitListen(ctx, s, catalog.SubmitListenOpts{
			Artist: "Artist", ArtistNames: []string{"Artist"}, TrackTitle: "Untitled", ReleaseTitle: "Single", RecordingMbzID: uuid.MustParse(mbid),
			UserID: 1, Time: time.Unix(1577934065+int64(i)*86400, 0), MbzCaller: &mbz.MbzErrorCaller{}, SkipCacheImage: true,
		}))
	}
	top, err := s.GetTopTracksPaginated(ctx, db.GetItemsOpts{Page: 1, Timeframe: db.Timeframe{Period: db.PeriodAllTime}})
	require.NoError(t, err)
	require.Len(t, top.Items, 1)
	track, err := s.GetTrack(ctx, db.GetTrackOpts{ID: top.Items[0].Item.ID})
	require.NoError(t, err)
	require.Len(t, track.Recordings, 2)
	require.EqualValues(t, 2, track.ListenCount)
	count, err := s.Count(`SELECT COUNT(*) FROM tracks`)
	require.NoError(t, err)
	require.Equal(t, 2, count)
	for _, title := range []string{"Song", "Song (Live)", "Song (Acoustic)", "Song (Remastered)"} {
		v := musicAppearance(t, s, title, 1, uuid.Nil)
		require.NoError(t, s.SaveListen(ctx, db.SaveListenOpts{TrackID: v.ID, Time: time.Unix(1577934065, 0), UserID: 1}))
	}
	n, err := s.CountTracks(ctx, db.Timeframe{Period: db.PeriodAllTime})
	require.NoError(t, err)
	require.EqualValues(t, 5, n, "version suffixes retain distinct histories")
}

func TestHistoryCaseComparisonIsReversible(t *testing.T) {
	s := musicStore(t)
	ctx := context.Background()
	a := musicAppearance(t, s, "Σong", 1, uuid.Nil)
	b := musicAppearance(t, s, "ςong", 2, uuid.Nil)
	for _, v := range []*models.Track{a, b} {
		require.NoError(t, s.SaveListen(ctx, db.SaveListenOpts{TrackID: v.ID, Time: time.Unix(1577934065, 0), UserID: 1}))
	}
	for _, sensitive := range []bool{true, false, true} {
		require.NoError(t, music.RefreshHistories(ctx, s.SourceDatabase(), sensitive))
		n, err := s.CountTracks(ctx, db.Timeframe{Period: db.PeriodAllTime})
		want := int64(2)
		if !sensitive {
			want = 1
		}
		require.NoError(t, err)
		require.Equal(t, want, n)
	}
	var original string
	require.NoError(t, s.QueryRow(`SELECT title FROM tracks_with_title WHERE id=?`, a.ID).Scan(&original))
	require.Equal(t, "Σong", original)
	require.NoError(t, s.QueryRow(`SELECT title FROM tracks_with_title WHERE id=?`, b.ID).Scan(&original))
	require.Equal(t, "ςong", original)
	n, err := s.Count(`SELECT COUNT(*) FROM listens`)
	require.NoError(t, err)
	require.Equal(t, 2, n)
}

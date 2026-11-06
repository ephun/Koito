package sqlite_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gabehf/koito/internal/db"
	"github.com/gabehf/koito/internal/db/sqlite"
	"github.com/gabehf/koito/internal/filters"
	"github.com/gabehf/koito/internal/sources"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

const caseCatalogImport = `[
 {"ts":"2020-01-02T03:04:05Z","ms_played":180001,"master_metadata_track_name":"Song","master_metadata_album_artist_name":"Artist","master_metadata_album_album_name":"Album","spotify_track_uri":"spotify:track:one","reason_end":"trackdone"},
 {"ts":"2020-01-03T03:04:05Z","ms_played":180001,"master_metadata_track_name":"Song","master_metadata_album_artist_name":"ARTIST","master_metadata_album_album_name":"ALBUM","spotify_track_uri":"spotify:track:one","reason_end":"trackdone"}]`

func caseCatalog(t *testing.T) (*sqlite.Sqlite, *sources.Store, sources.FilterState) {
	t.Helper()
	ctx := context.Background()
	s, err := sqlite.NewInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { s.Close(ctx) })
	require.NoError(t, s.Exec(`INSERT INTO users(id,username,role,password) VALUES(1,'test','admin',x'00')`))
	store := &sources.Store{DB: s.SourceDatabase(), Root: t.TempDir()}
	_, err = store.Import(ctx, strings.NewReader(caseCatalogImport), "spotify.json", 1)
	require.NoError(t, err)
	_, err = store.Rebuild(ctx)
	require.NoError(t, err)
	state, err := store.Filters(ctx)
	require.NoError(t, err)
	return s, store, state
}

func insensitiveCatalog(t *testing.T, store *sources.Store) sources.FilterState {
	t.Helper()
	ctx := context.Background()
	state, err := store.Filters(ctx)
	require.NoError(t, err)
	for i, r := range state.Current.Config.Rules {
		if migrated, ok := state.Blocks.Legacy[r.ID]; ok {
			state.Current.Config.Rules[i] = migrated
		}
	}
	for _, addition := range state.Blocks.Additions {
		found := false
		for _, r := range state.Current.Config.Rules {
			found = found || r.ID == addition.ID
		}
		if !found {
			state.Current.Config.Rules = append(state.Current.Config.Rules, addition)
		}
	}
	for i, r := range state.Current.Config.Rules {
		if r.Type == "matching" && r.Subtype == "music" {
			state.Current.Config.Rules[i].Program = state.Blocks.Templates["matching:music"][1].Program
			state.Current.Config.Rules[i].Migration = nil
			state.Current.Config.Rules[i].Settings.CaseSensitive = false
			state.Current.Config.Rules[i].Enabled = true
		}
	}
	_, err = store.Save(ctx, sources.FilterChange{BaseVersion: state.Current.ID, Config: state.Current.Config})
	require.NoError(t, err)
	state, err = store.Filters(ctx)
	require.NoError(t, err)
	return state
}

func TestCatalogMatchingTotalsKeepOriginalsAndReverse(t *testing.T) {
	s, store, original := caseCatalog(t)
	ctx := context.Background()
	all := db.Timeframe{Period: db.PeriodAllTime}
	insensitiveCatalog(t, store)
	_, err := store.Import(ctx, strings.NewReader(`{"Library Persistent ID":"GENRES","Tracks":{"1":{"Name":"Song","Artist":"Artist","Album":"Album","Genre":"Rock"},"2":{"Name":"Song","Artist":"ARTIST","Album":"ALBUM","Genre":"Jazz"}}}`), "genres.json", 1)
	require.NoError(t, err)
	_, err = store.Rebuild(ctx)
	require.NoError(t, err)
	assertCounts := func(artists, albums, tracks int64) {
		t.Helper()
		n, err := s.CountArtists(ctx, all)
		require.NoError(t, err)
		require.Equal(t, artists, n)
		n, err = s.CountAlbums(ctx, all)
		require.NoError(t, err)
		require.Equal(t, albums, n)
		n, err = s.CountTracks(ctx, all)
		require.NoError(t, err)
		require.Equal(t, tracks, n)
		n, err = s.CountListens(ctx, all)
		require.NoError(t, err)
		require.EqualValues(t, 2, n)
	}
	assertCounts(1, 1, 1)
	var rawArtists, rawAlbums int
	require.NoError(t, s.SourceDatabase().QueryRow(`SELECT COUNT(*) FROM artists`).Scan(&rawArtists))
	require.Equal(t, 2, rawArtists)
	require.NoError(t, s.SourceDatabase().QueryRow(`SELECT COUNT(*) FROM releases`).Scan(&rawAlbums))
	require.Equal(t, 2, rawAlbums)
	artist, err := s.GetArtist(ctx, db.GetArtistOpts{ID: 2, Original: true})
	require.NoError(t, err)
	require.Equal(t, "ARTIST", artist.Name)
	artist, err = s.GetArtist(ctx, db.GetArtistOpts{ID: 2})
	require.NoError(t, err)
	require.EqualValues(t, 2, artist.ListenCount)
	require.EqualValues(t, 1, artist.ID)
	artists, err := s.SearchArtists(ctx, "ARTIST")
	require.NoError(t, err)
	require.Len(t, artists, 1)
	albums, err := s.GetTopAlbumsPaginated(ctx, db.GetItemsOpts{Timeframe: all, Page: 1})
	require.NoError(t, err)
	require.Len(t, albums.Items, 1)
	group, err := s.GetAlbumGroup(ctx, albums.Items[0].Item.AlbumGroupID)
	require.NoError(t, err)
	require.Len(t, group.Editions, 2)
	require.EqualValues(t, 2, group.ListenCount)
	for _, edition := range group.Editions {
		require.EqualValues(t, 1, edition.ListenCount)
	}
	artist, err = s.GetArtist(ctx, db.GetArtistOpts{ID: 1})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"Jazz", "Rock"}, artist.Genres)
	chart, err := s.GetTopArtistsPaginated(ctx, db.GetItemsOpts{Timeframe: all, Genre: "Jazz", Page: 1})
	require.NoError(t, err)
	require.Len(t, chart.Items, 1)
	require.EqualValues(t, 2, chart.Items[0].Item.ListenCount)
	current, err := store.Filters(ctx)
	require.NoError(t, err)
	matchedConfig := current.Current.Config
	var clear *filters.Program
	require.NoError(t, json.Unmarshal([]byte(`{"steps":[{"if":{"op":"eq","args":[{"op":"field","field":"catalog.artist_id"},{"op":"literal","value":1}]},"then":[{"set":"artist.genres","value":{"op":"literal","value":[]}}]}]}`), &clear))
	cleared := current.Current.Config
	cleared.Rules = append(append([]sources.Rule(nil), cleared.Rules...), sources.Rule{ID: "clear-genres", Type: "presentation", Subtype: "naming", Enabled: true, Program: clear})
	_, err = store.Save(ctx, sources.FilterChange{BaseVersion: current.Current.ID, Config: cleared})
	require.NoError(t, err)
	artist, err = s.GetArtist(ctx, db.GetArtistOpts{ID: 1})
	require.NoError(t, err)
	require.Empty(t, artist.Genres)
	require.Len(t, artist.GenreEvidence, 2)
	current, err = store.Filters(ctx)
	require.NoError(t, err)
	_, err = store.Save(ctx, sources.FilterChange{BaseVersion: current.Current.ID, Config: matchedConfig})
	require.NoError(t, err)
	_, err = store.Import(ctx, strings.NewReader(caseCatalogImport), "again.json", 1)
	require.NoError(t, err)
	_, err = store.Rebuild(ctx)
	require.NoError(t, err)
	assertCounts(1, 1, 1)
	current, err = store.Filters(ctx)
	require.NoError(t, err)
	_, err = store.Save(ctx, sources.FilterChange{BaseVersion: current.Current.ID, Config: original.Current.Config})
	require.NoError(t, err)
	assertCounts(2, 2, 2)
}

func TestCatalogMatchingConflictingKnownIdentifiersStaySeparate(t *testing.T) {
	s, store, _ := caseCatalog(t)
	ctx := context.Background()
	require.NoError(t, s.Exec(`UPDATE artists SET musicbrainz_id=CASE id WHEN 1 THEN '00000000-0000-0000-0000-000000000001' ELSE '00000000-0000-0000-0000-000000000002' END`))
	insensitiveCatalog(t, store)
	n, err := s.CountArtists(ctx, db.Timeframe{Period: db.PeriodAllTime})
	require.NoError(t, err)
	require.EqualValues(t, 2, n)
	n, err = s.CountTracks(ctx, db.Timeframe{Period: db.PeriodAllTime})
	require.NoError(t, err)
	require.EqualValues(t, 2, n)
	n, err = s.CountAlbums(ctx, db.Timeframe{Period: db.PeriodAllTime})
	require.NoError(t, err)
	require.EqualValues(t, 2, n)
	// Release IDs conflict even when the artist comparison agrees.
	require.NoError(t, s.Exec(`UPDATE artists SET musicbrainz_id=NULL;UPDATE releases SET musicbrainz_id=CASE id WHEN 1 THEN '00000000-0000-0000-0000-000000000003' ELSE '00000000-0000-0000-0000-000000000004' END`))
	_, err = store.Rebuild(ctx)
	require.NoError(t, err)
	n, err = s.CountArtists(ctx, db.Timeframe{Period: db.PeriodAllTime})
	require.NoError(t, err)
	require.EqualValues(t, 1, n)
	n, err = s.CountAlbums(ctx, db.Timeframe{Period: db.PeriodAllTime})
	require.NoError(t, err)
	require.EqualValues(t, 2, n)
}

func TestAlbumMatchingUsesAlbumContextAndGroupingExceptions(t *testing.T) {
	s, store, _ := caseCatalog(t)
	ctx := context.Background()
	state := insensitiveCatalog(t, store)
	var program *filters.Program
	require.NoError(t, json.Unmarshal([]byte(`{"steps":[{"if":{"op":"eq","args":[{"op":"field","field":"source.service"},{"op":"literal","value":"spotify"}]},"then":[{"set":"group.album","value":{"op":"literal","value":""}}]}]}`), &program))
	state.Current.Config.Rules = append(state.Current.Config.Rules, sources.Rule{ID: "separate", Type: "presentation", Subtype: "grouping", Enabled: true, Program: program})
	_, err := store.Save(ctx, sources.FilterChange{BaseVersion: state.Current.ID, Config: state.Current.Config})
	require.NoError(t, err)
	n, err := s.CountAlbums(ctx, db.Timeframe{Period: db.PeriodAllTime})
	require.NoError(t, err)
	require.EqualValues(t, 2, n)
}

func TestCatalogMatchingIncludesIncomingNativeTracks(t *testing.T) {
	ctx := context.Background()
	s, store, _ := caseCatalog(t)
	insensitiveCatalog(t, store)
	artist, err := s.SaveArtist(ctx, db.SaveArtistOpts{Name: "artist"})
	require.NoError(t, err)
	album, err := s.SaveAlbum(ctx, db.SaveAlbumOpts{Title: "album", ArtistIDs: []int32{artist.ID}})
	require.NoError(t, err)
	track, err := s.SaveTrack(ctx, db.SaveTrackOpts{Title: "song", AlbumID: album.ID, ArtistIDs: []int32{artist.ID}})
	require.NoError(t, err)
	require.NoError(t, s.SaveListen(ctx, db.SaveListenOpts{TrackID: track.ID, UserID: 1, Time: time.Date(2020, 1, 4, 0, 0, 0, 0, time.UTC), Client: "native"}))
	check := func() {
		t.Helper()
		n, err := s.CountArtists(ctx, db.Timeframe{Period: db.PeriodAllTime})
		require.NoError(t, err)
		require.EqualValues(t, 1, n)
		n, err = s.CountAlbums(ctx, db.Timeframe{Period: db.PeriodAllTime})
		require.NoError(t, err)
		require.EqualValues(t, 1, n)
		n, err = s.CountTracks(ctx, db.Timeframe{Period: db.PeriodAllTime})
		require.NoError(t, err)
		require.EqualValues(t, 1, n)
		n, err = s.CountListens(ctx, db.Timeframe{Period: db.PeriodAllTime})
		require.NoError(t, err)
		require.EqualValues(t, 3, n)
	}
	check()
	_, err = store.Rebuild(ctx)
	require.NoError(t, err)
	check()
	var native int
	require.NoError(t, s.SourceDatabase().QueryRow(`SELECT COUNT(*) FROM listens WHERE client='native'`).Scan(&native))
	require.Equal(t, 1, native)
}

func TestCatalogComparisonsApplyToSourceTracks(t *testing.T) {
	ctx := context.Background()
	s, store, state := caseCatalog(t)
	for i, r := range state.Current.Config.Rules {
		if migrated, ok := state.Blocks.Legacy[r.ID]; ok {
			migrated.Migration = nil
			state.Current.Config.Rules[i] = migrated
		}
	}
	var program *filters.Program
	require.NoError(t, json.Unmarshal([]byte(`{"steps":[{"if":{"op":"eq","args":[{"op":"field","field":"catalog.kind"},{"op":"literal","value":"artist"}]},"then":[{"set":"music.artist","value":{"op":"literal","value":"Same artist"}}],"else_if":[{"if":{"op":"eq","args":[{"op":"field","field":"catalog.kind"},{"op":"literal","value":"album"}]},"then":[{"set":"music.album","value":{"op":"literal","value":"Same album"}}]}]}]}`), &program))
	state.Current.Config.Rules = append(state.Current.Config.Rules, sources.Rule{ID: "catalog-comparisons", Type: "matching", Subtype: "music", Enabled: true, Program: program})
	preview, err := store.Preview(ctx, sources.FilterChange{BaseVersion: state.Current.ID, Config: state.Current.Config})
	require.NoError(t, err)
	require.NotEmpty(t, preview.MusicExamples)
	n, err := s.CountArtists(ctx, db.Timeframe{Period: db.PeriodAllTime})
	require.NoError(t, err)
	require.EqualValues(t, 2, n)
	_, err = store.Save(ctx, sources.FilterChange{BaseVersion: state.Current.ID, Config: state.Current.Config})
	require.NoError(t, err)
	n, err = s.CountArtists(ctx, db.Timeframe{Period: db.PeriodAllTime})
	require.NoError(t, err)
	require.EqualValues(t, 1, n)
	n, err = s.CountAlbums(ctx, db.Timeframe{Period: db.PeriodAllTime})
	require.NoError(t, err)
	require.EqualValues(t, 1, n)
	n, err = s.CountTracks(ctx, db.Timeframe{Period: db.PeriodAllTime})
	require.NoError(t, err)
	require.EqualValues(t, 1, n)
}

func TestSourceScopedCatalogMatchingKeepsIncomingNativeContext(t *testing.T) {
	ctx := context.Background()
	s, store, state := caseCatalog(t)
	for i, r := range state.Current.Config.Rules {
		if migrated, ok := state.Blocks.Legacy[r.ID]; ok {
			migrated.Migration = nil
			state.Current.Config.Rules[i] = migrated
		}
	}
	var scoped, named, grouped *filters.Program
	require.NoError(t, json.Unmarshal([]byte(`{"steps":[{"if":{"op":"eq","args":[{"op":"field","field":"source.service"},{"op":"literal","value":"spotify"}]},"then":[{"set":"music.artist","value":{"op":"casefold","args":[{"op":"field","field":"original.artist"}]}},{"set":"music.album","value":{"op":"casefold","args":[{"op":"field","field":"original.album"}]}},{"set":"music.title","value":{"op":"casefold","args":[{"op":"field","field":"original.title"}]}}]}]}`), &scoped))
	require.NoError(t, json.Unmarshal([]byte(`{"steps":[{"if":{"op":"and","args":[{"op":"eq","args":[{"op":"field","field":"catalog.kind"},{"op":"literal","value":"track"}]},{"op":"eq","args":[{"op":"field","field":"catalog.id"},{"op":"literal","value":1}]}]},"then":[{"set":"display.title","value":{"op":"literal","value":"Clean song"}}]}]}`), &named))
	state.Current.Config.Rules = append(state.Current.Config.Rules, sources.Rule{ID: "spotify-comparisons", Type: "matching", Subtype: "music", Enabled: true, Program: scoped}, sources.Rule{ID: "track-name", Type: "presentation", Subtype: "naming", Enabled: true, Program: named})
	require.NoError(t, json.Unmarshal([]byte(`{"steps":[{"set":"group.album","value":{"op":"literal","value":"Album"}}]}`), &grouped))
	state.Current.Config.Rules = append(state.Current.Config.Rules, sources.Rule{ID: "grouped", Type: "presentation", Subtype: "grouping", Enabled: true, Program: grouped})
	_, err := store.Save(ctx, sources.FilterChange{BaseVersion: state.Current.ID, Config: state.Current.Config})
	require.NoError(t, err)
	opts := db.SaveListenOpts{TrackID: 1, Time: time.Date(2020, 1, 4, 0, 0, 0, 0, time.UTC), UserID: 1, Client: "native"}
	require.NoError(t, s.SaveListen(ctx, opts))
	require.NoError(t, s.SaveListen(ctx, opts))
	check := func() {
		t.Helper()
		all := db.Timeframe{Period: db.PeriodAllTime}
		n, err := s.CountArtists(ctx, all)
		require.NoError(t, err)
		require.EqualValues(t, 2, n)
		n, err = s.CountAlbums(ctx, all)
		require.NoError(t, err)
		require.EqualValues(t, 2, n)
		n, err = s.CountTracks(ctx, all)
		require.NoError(t, err)
		require.EqualValues(t, 2, n)
		n, err = s.CountListens(ctx, all)
		require.NoError(t, err)
		require.EqualValues(t, 3, n)
		artist, err := s.GetArtist(ctx, db.GetArtistOpts{ID: 1})
		require.NoError(t, err)
		require.EqualValues(t, 1, artist.ListenCount)
		artist, err = s.GetArtist(ctx, db.GetArtistOpts{ID: 2})
		require.NoError(t, err)
		require.EqualValues(t, 2, artist.ListenCount)
		for id := int32(1); id <= 2; id++ {
			albums, err := s.GetTopAlbumsPaginated(ctx, db.GetItemsOpts{Timeframe: all, ArtistID: int(id), Page: 1})
			require.NoError(t, err)
			require.Len(t, albums.Items, 1)
			require.EqualValues(t, id, albums.Items[0].Item.ListenCount)
			require.Len(t, albums.Items[0].Item.Artists, 1)
			require.Equal(t, id, albums.Items[0].Item.Artists[0].ID)
		}
		var native int32
		require.NoError(t, s.SourceDatabase().QueryRow(`SELECT track_id FROM listens WHERE client='native'`).Scan(&native))
		require.NotEqualValues(t, 1, native)
		track, err := s.GetTrack(ctx, db.GetTrackOpts{ID: native})
		require.NoError(t, err)
		require.Equal(t, "Clean song", track.Title)
	}
	check()
	_, err = store.Rebuild(ctx)
	require.NoError(t, err)
	check()
}

func TestNativeEmptyGroupingKeepsMatchedAlbumsSeparate(t *testing.T) {
	ctx := context.Background()
	s, store, _ := caseCatalog(t)
	state := insensitiveCatalog(t, store)
	var program *filters.Program
	require.NoError(t, json.Unmarshal([]byte(`{"steps":[{"if":{"op":"eq","args":[{"op":"field","field":"catalog.kind"},{"op":"literal","value":"album"}]},"then":[{"set":"group.album","value":{"op":"literal","value":""}}]}]}`), &program))
	state.Current.Config.Rules = append(state.Current.Config.Rules, sources.Rule{ID: "native-separate", Type: "presentation", Subtype: "grouping", Enabled: true, Program: program})
	_, err := store.Save(ctx, sources.FilterChange{BaseVersion: state.Current.ID, Config: state.Current.Config})
	require.NoError(t, err)
	n, err := s.CountAlbums(ctx, db.Timeframe{Period: db.PeriodAllTime})
	require.NoError(t, err)
	require.EqualValues(t, 2, n)
}

func TestNativeArtistEnrichmentRefreshesMatchedHistoriesAndGroups(t *testing.T) {
	ctx := context.Background()
	s, store, _ := caseCatalog(t)
	state := insensitiveCatalog(t, store)
	var grouped *filters.Program
	require.NoError(t, json.Unmarshal([]byte(`{"steps":[{"set":"group.album","value":{"op":"literal","value":"Album"}}]}`), &grouped))
	state.Current.Config.Rules = append(state.Current.Config.Rules, sources.Rule{ID: "grouped", Type: "presentation", Subtype: "grouping", Enabled: true, Program: grouped})
	_, err := store.Save(ctx, sources.FilterChange{BaseVersion: state.Current.ID, Config: state.Current.Config})
	require.NoError(t, err)
	for id := int32(1); id <= 2; id++ {
		identifier := uuid.MustParse("00000000-0000-0000-0000-00000000000" + string(rune('0'+id)))
		require.NoError(t, s.UpdateArtist(ctx, db.UpdateArtistOpts{ID: id, MusicBrainzID: identifier}))
	}
	all := db.Timeframe{Period: db.PeriodAllTime}
	n, err := s.CountArtists(ctx, all)
	require.NoError(t, err)
	require.EqualValues(t, 2, n)
	n, err = s.CountAlbums(ctx, all)
	require.NoError(t, err)
	require.EqualValues(t, 2, n)
	n, err = s.CountTracks(ctx, all)
	require.NoError(t, err)
	require.EqualValues(t, 2, n)
}

func TestMatchingKeepsRecordNamingVisibleAfterRebuild(t *testing.T) {
	ctx := context.Background()
	s, store, _ := caseCatalog(t)
	state := insensitiveCatalog(t, store)
	var program *filters.Program
	require.NoError(t, json.Unmarshal([]byte(`{"steps":[{"if":{"op":"eq","args":[{"op":"field","field":"source.service"},{"op":"literal","value":"spotify"}]},"then":[{"set":"display.artist","value":{"op":"literal","value":"Corrected artist"}},{"set":"display.album","value":{"op":"literal","value":"Corrected album"}},{"set":"display.title","value":{"op":"literal","value":"Corrected title"}}]}]}`), &program))
	state.Current.Config.Rules = append(state.Current.Config.Rules, sources.Rule{ID: "record-names", Type: "presentation", Subtype: "naming", Enabled: true, Program: program})
	_, err := store.Save(ctx, sources.FilterChange{BaseVersion: state.Current.ID, Config: state.Current.Config})
	require.NoError(t, err)
	check := func() {
		t.Helper()
		opts := db.GetItemsOpts{Timeframe: db.Timeframe{Period: db.PeriodAllTime}, Page: 1}
		artists, err := s.GetTopArtistsPaginated(ctx, opts)
		require.NoError(t, err)
		require.Len(t, artists.Items, 1)
		require.Equal(t, "Corrected artist", artists.Items[0].Item.Name)
		albums, err := s.GetTopAlbumsPaginated(ctx, opts)
		require.NoError(t, err)
		require.Len(t, albums.Items, 1)
		require.Equal(t, "Corrected album", albums.Items[0].Item.Title)
		tracks, err := s.GetTopTracksPaginated(ctx, opts)
		require.NoError(t, err)
		require.Len(t, tracks.Items, 1)
		require.Equal(t, "Corrected title", tracks.Items[0].Item.Title)
	}
	check()
	_, err = store.Rebuild(ctx)
	require.NoError(t, err)
	check()
}

func TestLegacyCaseMatchingCleansArtistAndAlbumTotals(t *testing.T) {
	ctx := context.Background()
	s, store, state := caseCatalog(t)
	for i, r := range state.Current.Config.Rules {
		if r.Type == "matching" && r.Subtype == "music" {
			state.Current.Config.Rules[i].Program = nil
			state.Current.Config.Rules[i].Settings.CaseSensitive = false
		}
	}
	_, err := store.Save(ctx, sources.FilterChange{BaseVersion: state.Current.ID, Config: state.Current.Config})
	require.NoError(t, err)
	all := db.Timeframe{Period: db.PeriodAllTime}
	n, err := s.CountArtists(ctx, all)
	require.NoError(t, err)
	require.EqualValues(t, 1, n)
	n, err = s.CountAlbums(ctx, all)
	require.NoError(t, err)
	require.EqualValues(t, 1, n)
	n, err = s.CountTracks(ctx, all)
	require.NoError(t, err)
	require.EqualValues(t, 1, n)
}

package sqlite_test

import (
	"context"
	"encoding/json"
	"github.com/gabehf/koito/internal/db"
	"github.com/gabehf/koito/internal/filters"
	"github.com/gabehf/koito/internal/sources"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
	"time"
)

func TestUnresolvedArtistGenreAssignmentIsInspectable(t *testing.T) {
	s := musicStore(t)
	ctx := context.Background()
	store := sources.Store{DB: s.SourceDatabase(), Root: t.TempDir()}
	state, err := store.Filters(ctx)
	require.NoError(t, err)
	var program filters.Program
	require.NoError(t, json.Unmarshal([]byte(`{"steps":[{"if":{"op":"eq","args":[{"op":"field","field":"raw:/missing"},{"op":"literal","value":true}]},"then":[{"set":"artist.genres","value":{"op":"list","args":[{"op":"literal","value":"Rock"}]}}]}]}`), &program))
	state.Current.Config.Rules = append(state.Current.Config.Rules, sources.Rule{ID: "missing-genre", Name: "Genre condition", Type: "presentation", Subtype: "naming", Enabled: true, Program: &program})
	_, err = store.Save(ctx, sources.FilterChange{BaseVersion: state.Current.ID, Config: state.Current.Config})
	require.NoError(t, err)
	artist, err := s.GetArtist(ctx, db.GetArtistOpts{ID: 1})
	require.NoError(t, err)
	require.Empty(t, artist.Genres)
	require.Len(t, artist.GenreIssues, 1)
	require.Equal(t, "missing-genre", artist.GenreIssues[0].FilterID)
}

func TestArtistGenreEvidenceRetainsProvenanceWithoutInferringPlays(t *testing.T) {
	s := musicStore(t)
	ctx := context.Background()
	store := sources.Store{DB: s.SourceDatabase(), Root: t.TempDir()}
	_, err := store.Import(ctx, strings.NewReader(`{"Library Persistent ID":"ONE","Tracks":{"1":{"Track ID":1,"Persistent ID":"A","Name":"Song","Artist":"Artist","Album":"Album","Genre":"Rock, Pop","Play Count":7}}}`), "library.json", 1)
	require.NoError(t, err)
	_, err = store.Rebuild(ctx)
	require.NoError(t, err)
	artist, err := s.GetArtist(ctx, db.GetArtistOpts{ID: 1})
	require.NoError(t, err)
	require.Len(t, artist.GenreEvidence, 1)
	require.Equal(t, []string{"Rock, Pop"}, artist.Genres)
	evidence := artist.GenreEvidence[0]
	require.Equal(t, "Rock, Pop", evidence.Value)
	require.Equal(t, "raw:/fields/Genre", evidence.Field)
	require.Positive(t, evidence.RecordID)
	require.Positive(t, evidence.SourceID)
	require.Zero(t, artist.ListenCount)
	require.NoError(t, s.Exec(`INSERT INTO artists(id) VALUES(2);INSERT INTO artist_aliases VALUES(2,'Artist','test',1)`))
	artist, err = s.GetArtist(ctx, db.GetArtistOpts{ID: 1})
	require.NoError(t, err)
	require.Empty(t, artist.GenreEvidence)
	var preserved string
	require.NoError(t, s.SourceDatabase().QueryRow(`SELECT value FROM source_genres WHERE record_id=?`, evidence.RecordID).Scan(&preserved))
	require.Equal(t, "Rock, Pop", preserved)
}

func TestGenreChartsConstrainCountsBeforeRanking(t *testing.T) {
	s := musicStore(t)
	ctx := context.Background()
	first := musicAppearance(t, s, "First", 1, uuid.Nil)
	require.NoError(t, s.Exec(`INSERT INTO artists(id) VALUES(2);INSERT INTO artist_aliases VALUES(2,'Other','test',1);INSERT INTO artist_genres VALUES(1,'Rock'),(1,'Pop'),(2,'Jazz')`))
	second, err := s.SaveTrack(ctx, db.SaveTrackOpts{Title: "Second", AlbumID: 2, ArtistIDs: []int32{2}})
	require.NoError(t, err)
	for i, track := range []int32{first.ID, second.ID, second.ID} {
		require.NoError(t, s.SaveListen(ctx, db.SaveListenOpts{TrackID: track, Time: time.Unix(1577934065+int64(i), 0), UserID: 1}))
	}
	for _, tf := range []db.Timeframe{{Period: db.PeriodAllTime}, {Period: db.PeriodAllTime, Year: 2020, Timezone: time.UTC}} {
		opts := db.GetItemsOpts{Genre: "Rock", Timeframe: tf, Page: 1, Limit: 10}
		tracks, err := s.GetTopTracksPaginated(ctx, opts)
		require.NoError(t, err)
		require.Len(t, tracks.Items, 1)
		require.Equal(t, first.ID, tracks.Items[0].Item.ID)
		require.EqualValues(t, 1, tracks.Items[0].Item.ListenCount)
		albums, err := s.GetTopAlbumsPaginated(ctx, opts)
		require.NoError(t, err)
		require.Len(t, albums.Items, 1)
		require.EqualValues(t, 1, albums.Items[0].Item.ListenCount)
		artists, err := s.GetTopArtistsPaginated(ctx, opts)
		require.NoError(t, err)
		require.Len(t, artists.Items, 1)
		require.EqualValues(t, 1, artists.Items[0].Item.ID)
		opts.Genre = "Missing"
		tracks, err = s.GetTopTracksPaginated(ctx, opts)
		require.NoError(t, err)
		require.Empty(t, tracks.Items)
	}
}

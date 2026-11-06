package sqlite_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/gabehf/koito/internal/db"
	"github.com/gabehf/koito/internal/filters"
	"github.com/gabehf/koito/internal/sources"
	"github.com/stretchr/testify/require"
)

func TestConcertCalendarDatesReachHistoryAndMusicPages(t *testing.T) {
	s := musicStore(t)
	ctx := context.Background()
	store := sources.Store{DB: s.SourceDatabase(), Root: t.TempDir()}
	data := []byte(`{"type":"setlist","id":"show","versionId":"one","eventDate":"02-01-2020","artist":{"name":"Concert Performer"},"venue":{"name":"Venue"},"sets":{"set":[{"song":[{"name":"Concert Song"}]}]}}`)
	_, err := store.Import(ctx, bytes.NewReader(data), "concert.json", 1)
	require.NoError(t, err)
	state, err := store.Filters(ctx)
	require.NoError(t, err)
	var owner filters.Program
	require.NoError(t, json.Unmarshal([]byte(`{"steps":[{"if":{"op":"eq","args":[{"op":"field","field":"source.service"},{"op":"literal","value":"setlistfm"}]},"then":[{"set":"listener","value":{"op":"literal","value":"self"}}]}]}`), &owner))
	state.Current.Config.Rules = append(state.Current.Config.Rules, sources.Rule{ID: "attendance", Type: "attribution", Subtype: "direct", Enabled: true, Program: &owner})
	version, err := store.Save(ctx, sources.FilterChange{BaseVersion: state.Current.ID, Config: state.Current.Config})
	require.NoError(t, err)
	var trackID, albumID, artistID int32
	require.NoError(t, s.SourceDatabase().QueryRow(`SELECT p.track_id,t.release_id,a.artist_id FROM source_plays p JOIN tracks t ON t.id=p.track_id JOIN artist_tracks a ON a.track_id=t.id LIMIT 1`).Scan(&trackID, &albumID, &artistID))
	assertDates := func(want string) {
		t.Helper()
		track, err := s.GetTrack(ctx, db.GetTrackOpts{ID: trackID})
		require.NoError(t, err)
		require.Equal(t, want, track.FirstListenDate)
		album, err := s.GetAlbum(ctx, db.GetAlbumOpts{ID: albumID})
		require.NoError(t, err)
		require.Equal(t, want, album.FirstListenDate)
		artist, err := s.GetArtist(ctx, db.GetArtistOpts{ID: artistID})
		require.NoError(t, err)
		require.Equal(t, want, artist.FirstListenDate)
	}
	assertDates("2020-01-02")
	page, err := s.GetListensPaginated(ctx, db.GetItemsOpts{Page: 1, Limit: 10, Timeframe: db.Timeframe{Period: db.PeriodAllTime}})
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	require.True(t, page.Items[0].DateOnly)
	require.Equal(t, "2020-01-02", page.Items[0].LocalDate)
	require.Equal(t, "concert", page.Items[0].ContextKind)
	require.False(t, page.Items[0].TimeEstimated)
	// Scope affects all three page summaries and history, while preserving the
	// source play and its calendar date for a later reversal.
	var hide filters.Program
	require.NoError(t, json.Unmarshal([]byte(`{"steps":[{"if":{"op":"eq","args":[{"op":"field","field":"event.kind"},{"op":"literal","value":"concert"}]},"then":[{"set":"view.include","value":{"op":"literal","value":false}}]}]}`), &hide))
	version.Config.Rules = append(version.Config.Rules, sources.Rule{ID: "concerts", Type: "inclusion", Subtype: "scope", Enabled: true, Program: &hide})
	version, err = store.Save(ctx, sources.FilterChange{BaseVersion: version.ID, Config: version.Config})
	require.NoError(t, err)
	assertDates("")
	var preserved int
	require.NoError(t, s.SourceDatabase().QueryRow(`SELECT COUNT(*) FROM source_play_context`).Scan(&preserved))
	require.Equal(t, 1, preserved)
	version.Config.Rules[len(version.Config.Rules)-1].Enabled = false
	_, err = store.Save(ctx, sources.FilterChange{BaseVersion: version.ID, Config: version.Config})
	require.NoError(t, err)
	assertDates("2020-01-02")
	require.NoError(t, s.SaveListen(ctx, db.SaveListenOpts{TrackID: trackID, Time: time.Date(2020, 1, 1, 12, 0, 0, 0, time.UTC), UserID: 1, Client: "audio"}))
	assertDates("")
}

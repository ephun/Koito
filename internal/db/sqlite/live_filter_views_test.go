package sqlite_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gabehf/koito/internal/db"
	"github.com/gabehf/koito/internal/filters"
	"github.com/gabehf/koito/internal/sources"
	"github.com/stretchr/testify/require"
)

func TestLiveAndImportedFilterConditionsAgreeAcrossViews(t *testing.T) {
	catalog := musicStore(t)
	ctx := context.Background()
	store := sources.Store{DB: catalog.SourceDatabase(), Root: t.TempDir()}
	for _, input := range []struct{ name, data string }{
		{"spotify.json", `[{"endTime":"2020-01-02 03:04","artistName":"Artist","trackName":"Song","msPlayed":180000},{"endTime":"2020-01-02 03:06","artistName":"Artist","trackName":"Song","msPlayed":180000}]`},
		{"lastfm.json", `[{"name":"Song","artist":{"#text":"Artist"},"date":{"uts":"1577934480"}}]`},
	} {
		_, err := store.Import(ctx, strings.NewReader(input.data), input.name, 1)
		require.NoError(t, err)
	}
	live := []byte(`{"listen_type":"single","payload":[{"listened_at":1577934600,"track_metadata":{"artist_name":"Artist","track_name":"Song","additional_info":{"duration_ms":180000,"unknown_field":{"preserved":true}}}}]}`)
	_, err := store.CaptureLive(ctx, live, 1, "live-view-test", "test")
	require.NoError(t, err)
	require.NoError(t, store.ProcessLive(ctx))
	state, err := store.Filters(ctx)
	require.NoError(t, err)
	program := func(raw string) *filters.Program {
		var p filters.Program
		require.NoError(t, json.Unmarshal([]byte(raw), &p))
		return &p
	}
	state.Current.Config.Rules = append(state.Current.Config.Rules,
		sources.Rule{ID: "known-spotify", Type: "attribution", Subtype: "direct", Enabled: true, Program: program(`{"steps":[{"if":{"op":"eq","args":[{"op":"field","field":"source.service"},{"op":"literal","value":"spotify"}]},"then":[{"set":"listener","value":{"op":"literal","value":"self"}}]}]}`)},
		sources.Rule{ID: "source-date", Type: "qualification", Subtype: "validity", Program: program(`{"steps":[{"if":{"op":"and","args":[{"op":"eq","args":[{"op":"field","field":"source.service"},{"op":"literal","value":"spotify"}]},{"op":"lt","args":[{"op":"field","field":"record.time"},{"op":"date","args":[{"op":"literal","value":"2020-01-02T03:05:00Z"}]}]}]},"then":[{"set":"validity.count","value":{"op":"literal","value":false}}]}]}`)},
		sources.Rule{ID: "play-date", Type: "inclusion", Subtype: "scope", Program: program(`{"steps":[{"if":{"op":"ge","args":[{"op":"field","field":"play.time"},{"op":"date","args":[{"op":"literal","value":"2020-01-02T03:07:00Z"}]}]},"then":[{"set":"view.include","value":{"op":"literal","value":false}}]}]}`)},
		sources.Rule{ID: "explicit-attribution", Type: "inclusion", Subtype: "evidence", Program: program(`{"steps":[{"if":{"op":"ne","args":[{"op":"field","field":"play.attribution_basis"},{"op":"literal","value":"direct"}]},"then":[{"set":"view.include","value":{"op":"literal","value":false}}]}]}`)},
	)
	for _, test := range []struct {
		source, date, attribution bool
		want                      int64
	}{
		{false, false, false, 4}, {true, false, false, 3}, {false, true, false, 2}, {false, false, true, 2}, {true, true, true, 1}, {false, false, false, 4},
	} {
		n := len(state.Current.Config.Rules)
		state.Current.Config.Rules[n-3].Enabled = test.source
		state.Current.Config.Rules[n-2].Enabled = test.date
		state.Current.Config.Rules[n-1].Enabled = test.attribution
		version, err := store.Save(ctx, sources.FilterChange{BaseVersion: state.Current.ID, Config: state.Current.Config})
		require.NoError(t, err)
		state.Current.ID = version.ID
		all := db.Timeframe{Period: db.PeriodAllTime, Timezone: time.UTC}
		total, err := catalog.CountListens(ctx, all)
		require.NoError(t, err)
		require.Equal(t, test.want, total, test)
		opts := db.GetItemsOpts{Timeframe: all, Page: 1, Limit: 100}
		history, err := catalog.GetListensPaginated(ctx, opts)
		require.NoError(t, err)
		require.Len(t, history.Items, int(test.want))
		tracks, err := catalog.GetTopTracksPaginated(ctx, opts)
		require.NoError(t, err)
		var trackTotal int64
		for _, track := range tracks.Items {
			trackTotal += track.Item.ListenCount
		}
		require.Equal(t, test.want, trackTotal)
		artists, err := catalog.GetTopArtistsPaginated(ctx, opts)
		require.NoError(t, err)
		require.Len(t, artists.Items, 1)
		require.Equal(t, test.want, artists.Items[0].Item.ListenCount)
		albums, err := catalog.GetTopAlbumsPaginated(ctx, opts)
		require.NoError(t, err)
		var albumTotal int64
		for _, album := range albums.Items {
			albumTotal += album.Item.ListenCount
		}
		require.Equal(t, test.want, albumTotal)
		activity, err := catalog.GetListenActivity(ctx, db.ListenActivityOpts{Step: db.StepDay, Month: 1, Year: 2020, Timezone: time.UTC})
		require.NoError(t, err)
		var activityTotal int64
		for _, day := range activity {
			activityTotal += day.Listens
		}
		require.Equal(t, test.want, activityTotal)
		exported, err := catalog.GetExportPage(ctx, db.GetExportPageOpts{UserID: 1, ListenedAt: time.Unix(0, 0), Limit: 100})
		require.NoError(t, err)
		require.Len(t, exported, int(test.want))
		found, err := catalog.SearchTracks(ctx, "Song")
		require.NoError(t, err)
		require.NotEmpty(t, found)
		stored, err := catalog.Count(`SELECT COUNT(*) FROM source_records`)
		require.NoError(t, err)
		require.Equal(t, 4, stored)
	}
}

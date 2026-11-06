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

func TestEvidenceLevelFiltersSharedPublicTotals(t *testing.T) {
	catalog := musicStore(t)
	ctx := context.Background()
	store := sources.Store{DB: catalog.SourceDatabase(), Root: t.TempDir()}
	for _, input := range []struct{ name, data string }{
		{"library.json", `{"Library Persistent ID":"LIBRARY","Date":"2020-01-09T00:00:00Z","Tracks":{"1":{"Track ID":1,"Persistent ID":"A","Name":"Song","Artist":"Artist","Album":"Album","Play Count":7,"Date Added":"2020-01-01T00:00:00Z","Play Date UTC":"2020-01-08T00:00:00Z"}}}`},
		{"lastfm.json", `[{"name":"Song","artist":{"#text":"Artist"},"album":{"#text":"Album"},"date":{"uts":"1577923200"}}]`},
	} {
		_, err := store.Import(ctx, strings.NewReader(input.data), input.name, 1)
		require.NoError(t, err)
	}
	state, err := store.Filters(ctx)
	require.NoError(t, err)
	var program filters.Program
	require.NoError(t, json.Unmarshal([]byte(`{"steps":[{"if":{"op":"ne","args":[{"op":"field","field":"play.evidence_level"},{"op":"literal","value":"observed"}]},"then":[{"set":"view.include","value":{"op":"literal","value":false}}]}]}`), &program))
	config := state.Current.Config
	config.Rules = append(config.Rules,
		sources.Rule{ID: "count", Type: "inference", Subtype: "count", Enabled: true, Scope: sources.RuleScope{Service: "itunes"}, Settings: sources.RuleSettings{CountMode: "maximum"}},
		sources.Rule{ID: "self", Type: "attribution", Subtype: "direct", Enabled: true, Scope: sources.RuleScope{Service: "itunes"}, Settings: sources.RuleSettings{Listener: "self"}},
		sources.Rule{ID: "observed", Type: "inclusion", Subtype: "evidence", Enabled: true, Program: &program})
	for _, enabled := range []bool{true, false} {
		config.Rules[len(config.Rules)-1].Enabled = enabled
		version, err := store.Save(ctx, sources.FilterChange{BaseVersion: state.Current.ID, Config: config})
		require.NoError(t, err)
		state.Current.ID = version.ID
		want, undated := int64(7), int64(6)
		if enabled {
			want, undated = 1, 0
		}
		all := db.Timeframe{Period: db.PeriodAllTime, Timezone: time.UTC}
		total, err := catalog.CountListens(ctx, all)
		require.NoError(t, err)
		require.Equal(t, want, total)
		total, err = catalog.CountUndatedListens(ctx, all)
		require.NoError(t, err)
		require.Equal(t, undated, total)
		opts := db.GetItemsOpts{Timeframe: all, Page: 1, Limit: 10}
		tracks, err := catalog.GetTopTracksPaginated(ctx, opts)
		require.NoError(t, err)
		require.Len(t, tracks.Items, 1)
		require.Equal(t, want, tracks.Items[0].Item.ListenCount)
		track, err := catalog.GetTrack(ctx, db.GetTrackOpts{ID: tracks.Items[0].Item.ID})
		require.NoError(t, err)
		require.Equal(t, want, track.ListenCount)
		albums, err := catalog.GetTopAlbumsPaginated(ctx, opts)
		require.NoError(t, err)
		require.Len(t, albums.Items, 1)
		require.Equal(t, want, albums.Items[0].Item.ListenCount)
		artists, err := catalog.GetTopArtistsPaginated(ctx, opts)
		require.NoError(t, err)
		require.Len(t, artists.Items, 1)
		require.Equal(t, want, artists.Items[0].Item.ListenCount)
		page, err := catalog.GetExportPage(ctx, db.GetExportPageOpts{UserID: 1, ListenedAt: time.Unix(0, 0), Limit: 100})
		require.NoError(t, err)
		require.Len(t, page, 1, "Dated exports retain the observed play while all-time totals also support undated counts")
		var stored int64
		require.NoError(t, catalog.SourceDatabase().QueryRow(`SELECT SUM(undated_count) FROM source_aggregates`).Scan(&stored))
		require.EqualValues(t, 6, stored)
	}
}

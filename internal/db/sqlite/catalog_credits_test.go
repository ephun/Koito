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

func TestCatalogCreditCorrectionChangesReportsAndCanBeDisabled(t *testing.T) {
	s := musicStore(t)
	ctx := context.Background()
	track := musicAppearance(t, s, "Song", 1, uuid.Nil)
	require.NoError(t, s.SaveListen(ctx, db.SaveListenOpts{TrackID: track.ID, Time: time.Unix(1577934065, 0), UserID: 1, Client: "test"}))
	require.NoError(t, s.Exec(`INSERT INTO artists(id) VALUES(2); INSERT INTO artist_aliases VALUES(2,'Other, with comma','test',1)`))
	store := sources.Store{DB: s.SourceDatabase(), Root: t.TempDir()}
	state, err := store.Filters(ctx)
	require.NoError(t, err)
	var program filters.Program
	require.NoError(t, json.Unmarshal([]byte(`{"steps":[{"if":{"op":"eq","args":[{"op":"field","field":"catalog.kind"},{"op":"literal","value":"track"}]},"then":[{"set":"display.artist_ids","value":{"op":"literal","value":[2]}},{"set":"display.primary_artist_ids","value":{"op":"literal","value":[2]}}]}]}`), &program))
	state.Current.Config.Rules = append(state.Current.Config.Rules, sources.Rule{ID: "credits", Type: "presentation", Subtype: "naming", Enabled: true, Program: &program})
	for _, enabled := range []bool{true, false} {
		state.Current.Config.Rules[len(state.Current.Config.Rules)-1].Enabled = enabled
		preview, err := store.Preview(ctx, sources.FilterChange{BaseVersion: state.Current.ID, Config: state.Current.Config})
		require.NoError(t, err)
		require.NotEmpty(t, preview.NamingExamples)
		version, err := store.Save(ctx, sources.FilterChange{BaseVersion: state.Current.ID, Config: state.Current.Config})
		require.NoError(t, err)
		state.Current.ID = version.ID
		want := int32(1)
		if enabled {
			want = 2
		}
		item, err := s.GetTrack(ctx, db.GetTrackOpts{ID: track.ID})
		require.NoError(t, err)
		require.Len(t, item.Artists, 1)
		require.Equal(t, want, item.Artists[0].ID)
		credits, err := s.GetArtistsForTrack(ctx, track.ID)
		require.NoError(t, err)
		require.Len(t, credits, 1)
		require.Equal(t, want, credits[0].ID)
		require.True(t, credits[0].IsPrimary)
		chart, err := s.GetTopArtistsPaginated(ctx, db.GetItemsOpts{Page: 1, Timeframe: db.Timeframe{Period: db.PeriodAllTime}})
		require.NoError(t, err)
		require.Len(t, chart.Items, 1)
		require.Equal(t, want, chart.Items[0].Item.ID)
		var original int32
		require.NoError(t, s.SourceDatabase().QueryRow(`SELECT artist_id FROM artist_tracks WHERE track_id=?`, track.ID).Scan(&original))
		require.Equal(t, int32(1), original)
	}
}

func TestInvalidCatalogCreditsLeaveNameAndCreditsUnchanged(t *testing.T) {
	s := musicStore(t)
	ctx := context.Background()
	track := musicAppearance(t, s, "Song", 1, uuid.Nil)
	store := sources.Store{DB: s.SourceDatabase(), Root: t.TempDir()}
	state, err := store.Filters(ctx)
	require.NoError(t, err)
	var program filters.Program
	require.NoError(t, json.Unmarshal([]byte(`{"steps":[{"if":{"op":"eq","args":[{"op":"field","field":"catalog.kind"},{"op":"literal","value":"track"}]},"then":[{"set":"display.title","value":{"op":"literal","value":"Changed"}},{"set":"display.artist_ids","value":{"op":"literal","value":[999999]}},{"set":"display.primary_artist_ids","value":{"op":"literal","value":[999999]}}]}]}`), &program))
	state.Current.Config.Rules = append(state.Current.Config.Rules, sources.Rule{ID: "invalid-credits", Type: "presentation", Subtype: "naming", Enabled: true, Program: &program})
	_, err = store.Save(ctx, sources.FilterChange{BaseVersion: state.Current.ID, Config: state.Current.Config})
	require.NoError(t, err)
	item, err := s.GetTrack(ctx, db.GetTrackOpts{ID: track.ID})
	require.NoError(t, err)
	require.Equal(t, "Song", item.Title)
	require.Equal(t, int32(1), item.Artists[0].ID)
	require.NotEmpty(t, item.NameIssues)
}

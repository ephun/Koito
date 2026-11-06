package sqlite_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/gabehf/koito/internal/db"
	"github.com/gabehf/koito/internal/filters"
	"github.com/gabehf/koito/internal/sources"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestListenScopeExclusionAndExceptionPreserveNativeListens(t *testing.T) {
	s := musicStore(t)
	ctx := context.Background()
	track := musicAppearance(t, s, "Song", 1, uuid.Nil)
	start := time.Unix(1577934065, 0).UTC()
	for i := 0; i < 2; i++ {
		require.NoError(t, s.SaveListen(ctx, db.SaveListenOpts{TrackID: track.ID, Time: start.Add(time.Duration(i) * time.Second), UserID: 1, Client: "test"}))
	}
	store := sources.Store{DB: s.SourceDatabase(), Root: t.TempDir()}
	state, err := store.Filters(ctx)
	require.NoError(t, err)
	var selected, all filters.Program
	require.NoError(t, json.Unmarshal([]byte(fmt.Sprintf(`{"steps":[{"if":{"op":"and","args":[{"op":"eq","args":[{"op":"field","field":"catalog.appearance_id"},{"op":"literal","value":%d}]},{"op":"eq","args":[{"op":"field","field":"play.time"},{"op":"date","args":[{"op":"literal","value":"%s"}]}]},{"op":"eq","args":[{"op":"field","field":"play.key"},{"op":"literal","value":""}]}]},"then":[{"set":"view.include","value":{"op":"literal","value":false}}]}]}`, track.ID, start.Format(time.RFC3339))), &selected))
	require.NoError(t, json.Unmarshal([]byte(`{"steps":[{"if":{"op":"literal","value":true},"then":[{"set":"view.include","value":{"op":"literal","value":false}}]}]}`), &all))
	state.Current.Config.Rules = append(state.Current.Config.Rules, sources.Rule{ID: "all", Type: "inclusion", Subtype: "scope", Enabled: false, Program: &all}, sources.Rule{ID: "selected", Type: "inclusion", Subtype: "scope", Enabled: true, Program: &selected})
	save := func() {
		version, e := store.Save(ctx, sources.FilterChange{BaseVersion: state.Current.ID, Config: state.Current.Config})
		require.NoError(t, e)
		state.Current.ID = version.ID
	}
	save()
	count, err := s.CountListens(ctx, db.Timeframe{Period: db.PeriodAllTime})
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	require.NoError(t, s.SaveListen(ctx, db.SaveListenOpts{TrackID: track.ID, Time: start.Add(2 * time.Second), UserID: 1, Client: "test"}))
	count, err = s.CountListens(ctx, db.Timeframe{Period: db.PeriodAllTime})
	require.NoError(t, err)
	require.EqualValues(t, 2, count)
	state.Current.Config.Rules[len(state.Current.Config.Rules)-2].Enabled = true
	state.Current.Config.Rules[len(state.Current.Config.Rules)-1].Program.Steps[0].Then[0].Value.Value = json.RawMessage(`true`)
	save()
	count, err = s.CountListens(ctx, db.Timeframe{Period: db.PeriodAllTime})
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	state.Current.Config.Rules[len(state.Current.Config.Rules)-2].Enabled = false
	state.Current.Config.Rules[len(state.Current.Config.Rules)-1].Enabled = false
	save()
	count, err = s.CountListens(ctx, db.Timeframe{Period: db.PeriodAllTime})
	require.NoError(t, err)
	require.EqualValues(t, 3, count)
	var stored int
	require.NoError(t, s.SourceDatabase().QueryRow(`SELECT COUNT(*) FROM listens`).Scan(&stored))
	require.Equal(t, 3, stored)
}

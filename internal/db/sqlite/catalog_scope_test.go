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
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestScopeExclusionPreservesListensAndSupportsLaterExceptions(t *testing.T) {
	s := musicStore(t)
	ctx := context.Background()
	a := musicAppearance(t, s, "Song", 1, uuid.Nil)
	b := musicAppearance(t, s, "Song", 2, uuid.Nil)
	for _, track := range []int32{a.ID, b.ID} {
		require.NoError(t, s.SaveListen(ctx, db.SaveListenOpts{TrackID: track, Time: time.Unix(1577934065, 0), UserID: 1, Client: "test"}))
	}
	store := sources.Store{DB: s.SourceDatabase(), Root: t.TempDir()}
	state, err := store.Filters(ctx)
	require.NoError(t, err)
	var exclude, exception filters.Program
	require.NoError(t, json.Unmarshal([]byte(`{"steps":[{"if":{"op":"eq","args":[{"op":"field","field":"catalog.track_id"},{"op":"literal","value":1}]},"then":[{"set":"view.include","value":{"op":"literal","value":false}}]}]}`), &exclude))
	require.NoError(t, json.Unmarshal([]byte(`{"steps":[{"if":{"op":"eq","args":[{"op":"field","field":"catalog.album_id"},{"op":"literal","value":2}]},"then":[{"set":"view.include","value":{"op":"literal","value":true}}]}]}`), &exception))
	state.Current.Config.Rules = append(state.Current.Config.Rules, sources.Rule{ID: "exclude", Type: "inclusion", Subtype: "scope", Enabled: true, Program: &exclude}, sources.Rule{ID: "exception", Type: "inclusion", Subtype: "scope", Enabled: false, Program: &exception})
	for _, test := range []struct {
		excluded, exception bool
		count               int64
	}{{true, false, 0}, {true, true, 1}, {false, false, 2}} {
		state.Current.Config.Rules[len(state.Current.Config.Rules)-2].Enabled = test.excluded
		state.Current.Config.Rules[len(state.Current.Config.Rules)-1].Enabled = test.exception
		preview, err := store.Preview(ctx, sources.FilterChange{BaseVersion: state.Current.ID, Config: state.Current.Config})
		require.NoError(t, err)
		require.NotEmpty(t, preview.ScopeExamples)
		version, err := store.Save(ctx, sources.FilterChange{BaseVersion: state.Current.ID, Config: state.Current.Config})
		require.NoError(t, err)
		state.Current.ID = version.ID
		count, err := s.CountListens(ctx, db.Timeframe{Period: db.PeriodAllTime})
		require.NoError(t, err)
		require.Equal(t, test.count, count)
		count, err = s.CountListens(ctx, db.Timeframe{FromUnix: 1577934000, ToUnix: 1577935000})
		require.NoError(t, err)
		require.Equal(t, test.count, count)
		track, err := s.GetTrack(ctx, db.GetTrackOpts{ID: a.ID})
		require.NoError(t, err)
		require.Equal(t, test.count, track.ListenCount)
		var original int64
		require.NoError(t, s.SourceDatabase().QueryRow(`SELECT COUNT(*) FROM listens`).Scan(&original))
		require.Equal(t, int64(2), original)
	}
}

func TestScopeAppliesToFutureNativeTracks(t *testing.T) {
	s := musicStore(t)
	ctx := context.Background()
	store := sources.Store{DB: s.SourceDatabase(), Root: t.TempDir()}
	state, err := store.Filters(ctx)
	require.NoError(t, err)
	var program filters.Program
	require.NoError(t, json.Unmarshal([]byte(`{"steps":[{"if":{"op":"in","args":[{"op":"literal","value":1},{"op":"field","field":"catalog.artist_ids"}]},"then":[{"set":"view.include","value":{"op":"literal","value":false}}]}]}`), &program))
	state.Current.Config.Rules = append(state.Current.Config.Rules, sources.Rule{ID: "exclude-artist", Type: "inclusion", Subtype: "scope", Enabled: true, Program: &program})
	_, err = store.Save(ctx, sources.FilterChange{BaseVersion: state.Current.ID, Config: state.Current.Config})
	require.NoError(t, err)
	track := musicAppearance(t, s, "New native track", 1, uuid.Nil)
	require.NoError(t, s.SaveListen(ctx, db.SaveListenOpts{TrackID: track.ID, Time: time.Unix(1577934065, 0), UserID: 1, Client: "test"}))
	count, err := s.CountListens(ctx, db.Timeframe{Period: db.PeriodAllTime})
	require.NoError(t, err)
	require.Zero(t, count)
	var stored int
	require.NoError(t, s.SourceDatabase().QueryRow(`SELECT COUNT(*) FROM listens`).Scan(&stored))
	require.Equal(t, 1, stored)
}

func TestScopeExclusionPreservesUndatedLibraryCounts(t *testing.T) {
	s := musicStore(t)
	ctx := context.Background()
	store := sources.Store{DB: s.SourceDatabase(), Root: t.TempDir()}
	input := `{"Library Persistent ID":"LIBRARY","Date":"2020-01-09T00:00:00Z","Tracks":{"1":{"Track ID":1,"Persistent ID":"A","Name":"Counted","Artist":"Artist","Album":"Single","Play Count":7,"Date Added":"2020-01-01T00:00:00Z","Play Date UTC":"2020-01-08T00:00:00Z"}}}`
	_, err := store.Import(ctx, strings.NewReader(input), "library.json", 1)
	require.NoError(t, err)
	state, err := store.Filters(ctx)
	require.NoError(t, err)
	var program filters.Program
	require.NoError(t, json.Unmarshal([]byte(`{"steps":[{"if":{"op":"eq","args":[{"op":"field","field":"original.title"},{"op":"literal","value":"Counted"}]},"then":[{"set":"view.include","value":{"op":"literal","value":false}}],"else_if":[{"if":{"op":"eq","args":[{"op":"field","field":"play.client"},{"op":"literal","value":"test"}]},"then":[{"set":"view.include","value":{"op":"literal","value":true}}]}]}]}`), &program))
	state.Current.Config.Rules = append(state.Current.Config.Rules,
		sources.Rule{ID: "count", Type: "inference", Subtype: "count", Enabled: true, Scope: sources.RuleScope{Service: "itunes"}, Settings: sources.RuleSettings{CountMode: "maximum"}},
		sources.Rule{ID: "self", Type: "attribution", Subtype: "direct", Enabled: true, Scope: sources.RuleScope{Service: "itunes"}, Settings: sources.RuleSettings{Listener: "self"}},
		sources.Rule{ID: "exclude", Type: "inclusion", Subtype: "scope", Enabled: true, Program: &program})
	for _, enabled := range []bool{true, false} {
		state.Current.Config.Rules[len(state.Current.Config.Rules)-1].Enabled = enabled
		version, err := store.Save(ctx, sources.FilterChange{BaseVersion: state.Current.ID, Config: state.Current.Config})
		require.NoError(t, err)
		state.Current.ID = version.ID
		want := int64(7)
		if enabled {
			want = 0
		}
		count, err := s.CountListens(ctx, db.Timeframe{Period: db.PeriodAllTime})
		require.NoError(t, err)
		require.Equal(t, want, count)
		count, err = s.CountUndatedListens(ctx, db.Timeframe{Period: db.PeriodAllTime})
		require.NoError(t, err)
		require.Equal(t, want, count)
		var stored int64
		require.NoError(t, s.SourceDatabase().QueryRow(`SELECT SUM(undated_count) FROM source_aggregates`).Scan(&stored))
		require.Equal(t, int64(7), stored)
	}
}

func TestScopePreviewUsesPendingNamesAndArtistCredits(t *testing.T) {
	s := musicStore(t)
	ctx := context.Background()
	track := musicAppearance(t, s, "Song", 1, uuid.Nil)
	require.NoError(t, s.SaveListen(ctx, db.SaveListenOpts{TrackID: track.ID, Time: time.Unix(1577934065, 0), UserID: 1, Client: "test"}))
	require.NoError(t, s.Exec(`INSERT INTO artists(id) VALUES(2);INSERT INTO artist_aliases VALUES(2,'Other','test',1)`))
	store := sources.Store{DB: s.SourceDatabase(), Root: t.TempDir()}
	state, err := store.Filters(ctx)
	require.NoError(t, err)
	var naming, scope filters.Program
	require.NoError(t, json.Unmarshal([]byte(`{"steps":[{"if":{"op":"eq","args":[{"op":"field","field":"catalog.kind"},{"op":"literal","value":"track"}]},"then":[{"set":"display.title","value":{"op":"literal","value":"Corrected"}},{"set":"display.artist_ids","value":{"op":"literal","value":[2]}},{"set":"display.primary_artist_ids","value":{"op":"literal","value":[2]}}]}]}`), &naming))
	require.NoError(t, json.Unmarshal([]byte(`{"steps":[{"if":{"op":"and","args":[{"op":"eq","args":[{"op":"field","field":"display.title"},{"op":"literal","value":"Corrected"}]},{"op":"in","args":[{"op":"literal","value":2},{"op":"field","field":"catalog.artist_ids"}]}]},"then":[{"set":"view.include","value":{"op":"literal","value":false}}]}]}`), &scope))
	state.Current.Config.Rules = append(state.Current.Config.Rules, sources.Rule{ID: "names", Type: "presentation", Subtype: "naming", Enabled: true, Program: &naming}, sources.Rule{ID: "scope", Type: "inclusion", Subtype: "scope", Enabled: true, Program: &scope})
	change := sources.FilterChange{BaseVersion: state.Current.ID, Config: state.Current.Config}
	preview, err := store.Preview(ctx, change)
	require.NoError(t, err)
	require.NotEmpty(t, preview.ScopeExamples)
	count, err := s.CountListens(ctx, db.Timeframe{Period: db.PeriodAllTime})
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	_, err = store.Save(ctx, change)
	require.NoError(t, err)
	count, err = s.CountListens(ctx, db.Timeframe{Period: db.PeriodAllTime})
	require.NoError(t, err)
	require.Zero(t, count)
}

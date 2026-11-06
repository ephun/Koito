package sources

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/gabehf/koito/internal/filters"
	"github.com/stretchr/testify/require"
)

func TestPerformanceTemplatesCompileAndEstimatedStartsAreMarked(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	_, err := s.Import(ctx, bytes.NewReader(yargFixture(t, false)), "scores.db", 1)
	require.NoError(t, err)
	_, err = s.Import(ctx, bytes.NewReader(setlistFixture("show", "one", "2020-01-03T00:00:00Z", "Song")), "concert.json", 1)
	require.NoError(t, err)
	state, err := s.Filters(ctx)
	require.NoError(t, err)
	var start *filters.Program
	for key, templates := range state.Blocks.Templates {
		for _, template := range templates {
			_, err := filters.CompileProgram(*template.Program, state.Blocks.Schemas[key])
			require.NoError(t, err, "%s: %s", key, template.Name)
			if template.Name == "yarg-start" {
				start = template.Program
			}
		}
	}
	require.NotNil(t, start)
	change := currentChange(t, s)
	addYARGTime(t, &change)
	change.Config.Rules = append(change.Config.Rules, Rule{ID: "estimated-start", Type: "inference", Subtype: "time", Enabled: true, Program: start})
	require.NoError(t, prepareTimePrograms(change.Config))
	r := matchingRecord{Service: "yarg", Parser: YARGDB, Payload: `{"kind":"game","fields":{"Date":638577685386176539,"SongName":"Song","SongArtist":"Artist","SongSpeed":1},"context":{}}`, LibraryFields: filters.Env{"game.catalog.duration_ms": filters.Integer(180000)}}
	interpretMatchingRecord(&r, change.Config)
	require.True(t, r.Values.TimeEstimated)
	require.True(t, r.Values.Performance.TimeEstimated)
	require.Equal(t, "estimated start", r.Basis)
	require.Equal(t, "2024-07-28T18:59:18.6176539Z", r.Start.Format(time.RFC3339Nano))
}

func TestContextClassificationAndInclusionAreReversible(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	data := sqliteFixture(t,
		`CREATE TABLE components(id INTEGER PRIMARY KEY,mode TEXT,type TEXT,name TEXT)`,
		`INSERT INTO components VALUES(1,'source','listenbrainz','YARGfm')`,
		`CREATE TABLE plays(id INTEGER PRIMARY KEY,uid TEXT,componentId INTEGER,parentId INTEGER,play TEXT,state TEXT)`,
		`CREATE TABLE play_inputs(id INTEGER PRIMARY KEY,playId INTEGER,data TEXT,play TEXT)`,
		`INSERT INTO plays VALUES(1,'event',1,NULL,'{"data":{"track":"Song","artists":[{"name":"Artist"}],"playDate":"2020-01-02T03:04:05Z"},"meta":{"source":"listenbrainz"}}','discovered')`)
	_, err := s.Import(ctx, bytes.NewReader(data), "source.db", 1)
	require.NoError(t, err)
	state, err := s.Filters(ctx)
	require.NoError(t, err)
	contextProgram := state.Blocks.Templates["inference:context"][0].Program
	change := currentChange(t, s)
	change.Config.Rules = append(change.Config.Rules,
		Rule{ID: "context", Type: "inference", Subtype: "context", Enabled: true, Program: contextProgram},
		Rule{ID: "owner", Type: "attribution", Subtype: "direct", Enabled: true, Scope: RuleScope{Service: "multi-scrobbler"}, Settings: RuleSettings{Listener: "self"}},
		Rule{ID: "hide-gameplay", Type: "inclusion", Subtype: "scope", Enabled: true, Program: assignWhen(contextKindCondition("gameplay"), "view.include", literalExpr(false))})
	_, err = s.Save(ctx, change)
	require.NoError(t, err)
	require.Equal(t, 1, count(t, s, "source_plays"))
	require.Zero(t, count(t, s, "displayed_listens"))
	var raw string
	require.NoError(t, s.DB.QueryRow(`SELECT context_json FROM source_play_context`).Scan(&raw))
	require.Contains(t, raw, `"kind":"gameplay"`)
	change = currentChange(t, s)
	testRule(t, &change.Config, "context").Enabled = false
	_, err = s.Save(ctx, change)
	require.NoError(t, err)
	require.Equal(t, 1, count(t, s, "displayed_listens"))
	require.Zero(t, count(t, s, "source_play_context"))
}

func TestGameCatalogConflictPreservesCandidates(t *testing.T) {
	records := []matchingRecord{
		{ID: 1, Service: "yarg", Values: recordValues{NativeTrackID: "chart", Performance: &performanceValues{Kind: "gameplay"}}},
		{ID: 2, Service: "yarg", Parser: YARGCache, Values: recordValues{NativeTrackID: "chart", Album: "One", DurationMS: 180000}},
		{ID: 3, Service: "yarg", Parser: YARGCache, Values: recordValues{NativeTrackID: "chart", Album: "Two", DurationMS: 190000}},
	}
	attachGameCatalog(records)
	require.Len(t, records[0].GameCatalog.Records, 2)
	require.Nil(t, records[0].GameCatalog.DurationMS)
	require.Empty(t, records[0].GameCatalog.Album)
	require.Len(t, records[0].GameCatalog.Items, 2)
}

func TestInvalidContextDoesNotReplaceKnownPerformance(t *testing.T) {
	r := matchingRecord{Values: recordValues{Performance: &performanceValues{Kind: "concert", SetlistID: "show", Legacy: true}}}
	config := RuleConfig{Rules: []Rule{{ID: "context", Type: "inference", Subtype: "context", Enabled: true, Program: assignWhen(literalExpr(true), "context.kind", literalExpr("other"))}}}
	applyContextPrograms(&r, config)
	require.Equal(t, "concert", r.Values.Performance.Kind)
	require.Len(t, r.ContextIssues, 1)
	env := r.Values.Performance.Environment()
	_, set := env["concert.set"]
	_, tape := env["concert.tape"]
	require.False(t, set, "a recovered record supplies no set position")
	require.False(t, tape, "a recovered record supplies no tape flag")
}

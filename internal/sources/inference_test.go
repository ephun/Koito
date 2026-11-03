package sources

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func countRules() []Rule {
	return []Rule{
		{ID: "library-count", Type: "inference", Subtype: "count", Enabled: true, Scope: RuleScope{Service: "itunes"}, Settings: RuleSettings{CountMode: "maximum"}},
		{ID: "library-listener", Type: "attribution", Subtype: "direct", Enabled: true, Scope: RuleScope{Service: "itunes"}, Settings: RuleSettings{Listener: "self"}},
	}
}

func saveTestRules(t *testing.T, s *Store, rules ...Rule) RuleVersion {
	t.Helper()
	c := currentChange(t, s)
	c.Config.Rules = append(c.Config.Rules, rules...)
	v, err := s.Save(context.Background(), c)
	require.NoError(t, err)
	return v
}

func readAggregate(t *testing.T, s *Store) AggregateContribution {
	t.Helper()
	var text string
	require.NoError(t, s.DB.QueryRow(`SELECT details_json FROM source_aggregates`).Scan(&text))
	var out AggregateContribution
	require.NoError(t, json.Unmarshal([]byte(text), &out))
	return out
}

func TestInferenceUndatedCountsAndDisabledRules(t *testing.T) {
	s := testStore(t)
	importLibrarySnapshot(t, s, "ONE", "2012-01-01T00:00:00Z", []map[string]any{libraryFixtureTrack("A", 7)})
	saveTestRules(t, s, countRules()...)
	aggregate := readAggregate(t, s)
	require.EqualValues(t, 7, aggregate.Available)
	require.EqualValues(t, 7, aggregate.Attributed)
	require.EqualValues(t, 7, aggregate.Undated)
	require.Zero(t, count(t, s, "listens"))
	before := count(t, s, "source_records")
	change := currentChange(t, s)
	change.Config.Rules[3].Enabled = false
	_, err := s.Save(context.Background(), change)
	require.NoError(t, err)
	require.Zero(t, count(t, s, "source_aggregates"))
	require.Equal(t, before, count(t, s, "source_records"))
	change = currentChange(t, s)
	change.Config.Rules[3].Enabled = true
	_, err = s.Save(context.Background(), change)
	require.NoError(t, err)
	require.EqualValues(t, 7, readAggregate(t, s).Undated)
}

func TestInferenceObservedOverlapDeterminismAndLineage(t *testing.T) {
	s := testStore(t)
	track := libraryFixtureTrack("A", 7)
	track["Date Added"] = "2020-01-01T00:00:00Z"
	track["Play Date UTC"] = "2020-01-08T00:00:00Z"
	importLibrarySnapshot(t, s, "ONE", "2020-01-09T00:00:00Z", []map[string]any{track})
	importLastfm(t, s, "["+lf("Song", "Album", time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC).Unix())+"]")
	rules := append(countRules(), Rule{ID: "library-time", Type: "inference", Subtype: "time", Enabled: true, Scope: RuleScope{Service: "itunes"}, Settings: RuleSettings{UseLastPlayed: true, EstimateTimes: true}})
	saveTestRules(t, s, rules...)
	aggregate := readAggregate(t, s)
	require.EqualValues(t, 1, aggregate.Observed)
	require.EqualValues(t, 1, aggregate.LastPlayed)
	require.EqualValues(t, 5, aggregate.Estimated)
	require.Zero(t, aggregate.Undated)
	require.Equal(t, 7, count(t, s, "listens"))
	preview, err := s.Preview(context.Background(), currentChange(t, s))
	require.NoError(t, err)
	require.Zero(t, preview.Changed)
	inspection, err := s.Inspect(context.Background(), 2)
	require.NoError(t, err)
	require.Len(t, inspection.Contributions, 1)
	require.EqualValues(t, 7, inspection.Contributions[0].Attributed)
	plays, err := s.Plays(context.Background(), 0, "", false)
	require.NoError(t, err)
	require.Len(t, plays, 7)
	for _, p := range plays {
		require.NotContains(t, p.Time, "0001")
		require.NotEmpty(t, p.Evidence)
	}
	rows, err := s.DB.Query(`SELECT p.play_key,p.listened_at,d.evidence_level FROM source_plays p JOIN source_play_details d ON d.play_key=p.play_key ORDER BY p.play_key`)
	require.NoError(t, err)
	fingerprint := []string{}
	for rows.Next() {
		var key, level string
		var unix int64
		require.NoError(t, rows.Scan(&key, &unix, &level))
		fingerprint = append(fingerprint, fmt.Sprintf("%s:%d:%s", key, unix, level))
	}
	require.NoError(t, rows.Err())
	rows.Close()
	require.Equal(t, 7, count(t, s, "source_play_records"))
	for i := 0; i < 2; i++ {
		_, err = s.Rebuild(context.Background())
		require.NoError(t, err)
		rows, err = s.DB.Query(`SELECT p.play_key,p.listened_at,d.evidence_level FROM source_plays p JOIN source_play_details d ON d.play_key=p.play_key ORDER BY p.play_key`)
		require.NoError(t, err)
		current := []string{}
		for rows.Next() {
			var key, level string
			var unix int64
			require.NoError(t, rows.Scan(&key, &unix, &level))
			current = append(current, fmt.Sprintf("%s:%d:%s", key, unix, level))
		}
		require.NoError(t, rows.Err())
		rows.Close()
		require.Equal(t, fingerprint, current)
	}
	change := currentChange(t, s)
	change.Config.Rules[5].Enabled = false
	_, err = s.Save(context.Background(), change)
	require.NoError(t, err)
	require.Equal(t, 1, count(t, s, "listens"))
	require.EqualValues(t, 6, readAggregate(t, s).Undated)
	require.Equal(t, 3, count(t, s, "source_records"))
}

func TestInferencePartialSharesAndInvalidDates(t *testing.T) {
	for _, last := range []string{"2011-01-01T00:00:00Z", "1904-01-01T00:00:00Z"} {
		s := testStore(t)
		track := libraryFixtureTrack("A", 7)
		track["Play Date UTC"] = last
		importLibrarySnapshot(t, s, "ONE", "2012-01-01T00:00:00Z", []map[string]any{track})
		percent := 50
		rules := countRules()[:1]
		rules = append(rules, Rule{ID: "share", Type: "attribution", Subtype: "aggregate", Enabled: true, Scope: RuleScope{Service: "itunes"}, Settings: RuleSettings{SharePercent: &percent}}, Rule{ID: "time", Type: "inference", Subtype: "time", Enabled: true, Scope: RuleScope{Service: "itunes"}, Settings: RuleSettings{UseLastPlayed: true}})
		saveTestRules(t, s, rules...)
		aggregate := readAggregate(t, s)
		require.EqualValues(t, 3, aggregate.Attributed)
		require.EqualValues(t, 3, aggregate.Undated)
		require.Zero(t, aggregate.LastPlayed)
		require.Zero(t, count(t, s, "listens"))
		require.Equal(t, "aggregate", aggregate.Attribution.Basis)
	}
}

func TestInferenceCountModesAndResetTotals(t *testing.T) {
	for mode, expected := range map[string]int64{"maximum": 10, "latest": 5, "reset_total": 15} {
		s := testStore(t)
		for i, count := range []int{7, 10, 2, 5} {
			importLibrarySnapshot(t, s, "ONE", fmt.Sprintf("201%d-01-01T00:00:00Z", i+2), []map[string]any{libraryFixtureTrack("A", count)})
		}
		rules := countRules()
		rules[0].Settings.CountMode = mode
		saveTestRules(t, s, rules...)
		aggregate := readAggregate(t, s)
		require.Equal(t, expected, aggregate.Available)
		require.Equal(t, expected, aggregate.Undated)
		require.EqualValues(t, 10, *aggregate.Reported)
	}
}

func TestInferenceUnknownAttributionAndUnsupportedIdentity(t *testing.T) {
	s := testStore(t)
	importLibrarySnapshot(t, s, "ONE", "2012-01-01T00:00:00Z", []map[string]any{libraryFixtureTrack("A", 7)})
	saveTestRules(t, s, countRules()[0])
	aggregate := readAggregate(t, s)
	require.Equal(t, "unknown", aggregate.Attribution.Listener)
	require.EqualValues(t, 7, aggregate.Available)
	require.Zero(t, aggregate.Attributed)
	require.Zero(t, count(t, s, "listens"))
}

func TestInferenceHeuristicsRequireDirectPersonalEvidence(t *testing.T) {
	s := testStore(t)
	importLibrarySnapshot(t, s, "ONE", "2012-01-01T00:00:00Z", []map[string]any{libraryFixtureTrack("A", 7)})
	importLastfm(t, s, "["+lf("Song", "Album", time.Date(2010, 2, 1, 0, 0, 0, 0, time.UTC).Unix())+"]")
	var source int64
	require.NoError(t, s.DB.QueryRow(`SELECT id FROM source_accounts WHERE service='lastfm'`).Scan(&source))
	percent := 100
	rules := []Rule{countRules()[0], {ID: "library-heuristic", Type: "attribution", Subtype: "heuristic", Enabled: true, Scope: RuleScope{Service: "itunes"}, Settings: RuleSettings{EvidenceSourceID: source, MinimumMatches: 1, SharePercent: &percent}}}
	saveTestRules(t, s, rules...)
	require.Zero(t, readAggregate(t, s).Attributed)
	saveTestRules(t, s, Rule{ID: "personal-lastfm", Type: "attribution", Subtype: "direct", Enabled: true, Scope: RuleScope{Service: "lastfm"}, Settings: RuleSettings{Listener: "self"}})
	aggregate := readAggregate(t, s)
	require.Equal(t, "heuristic", aggregate.Attribution.Basis)
	require.EqualValues(t, 7, aggregate.Attributed)
	require.EqualValues(t, 1, aggregate.Observed)
	require.EqualValues(t, 6, aggregate.Undated)
}

func TestInferenceFailureRollsBackCountsAndEvents(t *testing.T) {
	s := testStore(t)
	importLibrarySnapshot(t, s, "ONE", "2012-01-01T00:00:00Z", []map[string]any{libraryFixtureTrack("A", 7)})
	saveTestRules(t, s, countRules()...)
	before := readAggregate(t, s)
	require.NoError(t, func() error {
		_, err := s.DB.Exec(`CREATE TRIGGER reject_aggregate BEFORE INSERT ON source_aggregates BEGIN SELECT RAISE(ABORT,'test failure'); END`)
		return err
	}())
	change := currentChange(t, s)
	change.Config.Rules = append(change.Config.Rules, Rule{ID: "last", Type: "inference", Subtype: "time", Enabled: true, Scope: RuleScope{Service: "itunes"}, Settings: RuleSettings{UseLastPlayed: true}})
	_, err := s.Save(context.Background(), change)
	require.Error(t, err)
	require.Equal(t, before, readAggregate(t, s))
	require.Zero(t, count(t, s, "listens"))
	require.Equal(t, 2, count(t, s, "source_records"))
}

func TestInferenceInputArraysStayOriginal(t *testing.T) {
	s := testStore(t)
	raw := []byte(`[{"Track ID":[1],"Persistent ID":["A"],"Name":["Song"],"Artist":["Artist"],"Play Count":[7]}]`)
	_, err := s.Import(context.Background(), bytes.NewReader(raw), "converted.json", 1)
	require.NoError(t, err)
	rules := countRules()
	// An unknown library requires an explicit file scope.
	rules[0].Scope.FileHash = digest(raw)
	saveTestRules(t, s, rules...)
	require.EqualValues(t, 7, readAggregate(t, s).Undated)
}

func TestInferenceCursorPagesMultipleEventsFromOneRecord(t *testing.T) {
	s := testStore(t)
	importLibrarySnapshot(t, s, "ONE", "2012-01-01T00:00:00Z", []map[string]any{libraryFixtureTrack("A", 130)})
	rules := append(countRules(), Rule{ID: "time", Type: "inference", Subtype: "time", Enabled: true, Scope: RuleScope{Service: "itunes"}, Settings: RuleSettings{EstimateTimes: true}})
	saveTestRules(t, s, rules...)
	first, err := s.Plays(context.Background(), 0, "", false)
	require.NoError(t, err)
	require.Len(t, first, 100)
	last := first[len(first)-1]
	second, err := s.PlaysAfter(context.Background(), last.RecordID, last.Key, "", false)
	require.NoError(t, err)
	require.Len(t, second, 30)
	seen := map[string]bool{}
	for _, p := range append(first, second...) {
		require.False(t, seen[p.Key])
		seen[p.Key] = true
		require.Equal(t, "estimated", p.Evidence)
	}
}

func TestInferenceAvoidsCoarseObservedIntervals(t *testing.T) {
	s := testStore(t)
	track := libraryFixtureTrack("A", 5)
	track["Date Added"] = "2020-01-01T00:00:00Z"
	track["Play Date UTC"] = "2020-01-01T00:02:00Z"
	importLibrarySnapshot(t, s, "ONE", "2020-01-02T00:00:00Z", []map[string]any{track})
	_, err := s.Import(context.Background(), bytes.NewBufferString("Artist,Album,Song,\"01 Jan 2020, 00:00\"\n"), "lastfm.csv", 1)
	require.NoError(t, err)
	change := currentChange(t, s)
	change.Config.Rules[2].Settings.Timezone = "UTC"
	change.Config.Rules = append(change.Config.Rules, countRules()...)
	change.Config.Rules = append(change.Config.Rules, Rule{ID: "time", Type: "inference", Subtype: "time", Enabled: true, Scope: RuleScope{Service: "itunes"}, Settings: RuleSettings{EstimateTimes: true}})
	_, err = s.Save(context.Background(), change)
	require.NoError(t, err)
	planned, err := s.Plays(context.Background(), 0, "", false)
	require.NoError(t, err)
	for _, p := range planned {
		if p.Evidence == "estimated" {
			at, err := time.Parse(time.RFC3339Nano, p.Time)
			require.NoError(t, err)
			require.GreaterOrEqual(t, at.Minute(), 1)
		}
	}
	aggregate := readAggregate(t, s)
	require.EqualValues(t, 5, aggregate.Observed+aggregate.Estimated+aggregate.Undated)
}

func TestInferenceObservedEvidenceCoversOneCounter(t *testing.T) {
	s := testStore(t)
	importLibrarySnapshot(t, s, "ONE", "2012-01-01T00:00:00Z", []map[string]any{libraryFixtureTrack("A", 7), libraryFixtureTrack("B", 7)})
	importLastfm(t, s, "["+lf("Song", "Album", time.Date(2010, 2, 1, 0, 0, 0, 0, time.UTC).Unix())+"]")
	saveTestRules(t, s, countRules()...)
	var attributed, observed, undated int64
	require.NoError(t, s.DB.QueryRow(`SELECT SUM(attributed_count),SUM(observed_count),SUM(undated_count) FROM source_aggregates`).Scan(&attributed, &observed, &undated))
	require.EqualValues(t, 14, attributed)
	require.EqualValues(t, 1, observed)
	require.EqualValues(t, 13, undated)
}

func TestInferenceDateScopeContainsTheWholeCountInterval(t *testing.T) {
	s := testStore(t)
	importLibrarySnapshot(t, s, "ONE", "2012-01-01T00:00:00Z", []map[string]any{libraryFixtureTrack("A", 7)})
	rules := countRules()
	rules[1].Scope.From = "2010-01-01T00:00:00Z"
	rules[1].Scope.Until = "2011-01-01T00:00:00Z"
	saveTestRules(t, s, rules...)
	require.Zero(t, readAggregate(t, s).Attributed)
	change := currentChange(t, s)
	change.Config.Rules[4].Scope.Until = "2012-01-01T00:00:00Z"
	_, err := s.Save(context.Background(), change)
	require.NoError(t, err)
	require.EqualValues(t, 7, readAggregate(t, s).Attributed)
}

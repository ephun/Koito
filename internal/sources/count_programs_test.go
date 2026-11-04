package sources

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/gabehf/koito/internal/filters"
	"github.com/stretchr/testify/require"
)

func TestCountMigrationPreservesSnapshotSelectionAmountsAndTiming(t *testing.T) {
	for _, mode := range []string{"maximum", "latest", "reset_total"} {
		for _, scenario := range []string{"valid", "conflict", "missing", "undated", "unidentified"} {
			t.Run(mode+"/"+scenario, func(t *testing.T) {
				s := testStore(t)
				ctx := context.Background()
				library := "ONE"
				if scenario == "unidentified" {
					library = ""
				}
				importLibrarySnapshot(t, s, library, "2012-01-01T00:00:00Z", []map[string]any{libraryFixtureTrack("A", 7)})
				date := "2013-01-01T00:00:00Z"
				var count any = 10
				if scenario == "missing" {
					count = nil
				}
				if scenario == "undated" {
					date = ""
				}
				importLibrarySnapshot(t, s, library, date, []map[string]any{libraryFixtureTrack("A", count)})
				if scenario == "conflict" {
					importLibrarySnapshot(t, s, library, date, []map[string]any{libraryFixtureTrack("A", 2)})
				}
				if scenario == "valid" {
					importLibrarySnapshot(t, s, library, "2014-01-01T00:00:00Z", []map[string]any{libraryFixtureTrack("A", 2)})
				}
				state := currentChange(t, s)
				countRule := countRules()[0]
				countRule.Settings.CountMode = mode
				state.Config.Rules = append(state.Config.Rules, countRule, countRules()[1], Rule{ID: "dates", Type: "inference", Subtype: "time", Enabled: true, Scope: RuleScope{Service: "itunes"}, Settings: RuleSettings{UseLastPlayed: true}})
				libraries, err := libraryEvidence(ctx, s.DB)
				require.NoError(t, err)
				originalJSON, _ := json.Marshal(state.Config)
				migrated := migrateCountPrograms(state.Config)
				unchangedJSON, _ := json.Marshal(state.Config)
				require.Equal(t, originalJSON, unchangedJSON)
				plans := []matchPlan{}
				for _, config := range []RuleConfig{state.Config, migrated} {
					records, err := loadMatchingRecords(ctx, s.DB, config)
					require.NoError(t, err)
					plan, err := calculateMatches(ctx, records, config, nil)
					require.NoError(t, err)
					applyAttribution(&plan, config)
					require.NoError(t, inferLibraryContributions(ctx, &plan, libraries, config))
					plans = append(plans, plan)
				}
				require.Equal(t, len(plans[0].Aggregates), len(plans[1].Aggregates))
				for i, before := range plans[0].Aggregates {
					after := plans[1].Aggregates[i]
					after.FilterIssues = nil
					before.FilterIssues = nil
					require.Equal(t, before, after)
				}
				require.Equal(t, plans[0].Plays, plans[1].Plays)
			})
		}
	}
}

func TestCountMigrationPreservesFileAndRecordConditionsIncludingDisabledFilters(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	importLibrarySnapshot(t, s, "ONE", "2012-01-01T00:00:00Z", []map[string]any{libraryFixtureTrack("A", 7)})
	importLibrarySnapshot(t, s, "ONE", "2013-01-01T00:00:00Z", []map[string]any{libraryFixtureTrack("A", 10)})
	libraries, err := libraryEvidence(ctx, s.DB)
	require.NoError(t, err)
	require.Len(t, libraries, 1)
	track := libraries[0].Tracks[0]
	first := track.Snapshots[0]
	var source int64
	require.NoError(t, s.DB.QueryRow(`SELECT source_id FROM source_records WHERE id=?`, first.RecordID).Scan(&source))
	for _, scope := range []RuleScope{{Service: "itunes"}, {Service: "itunes", FileHash: first.FileHash}, {Service: "itunes", RecordID: first.RecordID}, {Service: "itunes", SourceID: source}, {Service: "itunes", TrackID: "A"}, {Service: "itunes", Artist: "Artist"}, {Service: "itunes", Artist: "Other"}, {Service: "itunes", LibraryID: "ONE"}} {
		for _, enabled := range []bool{false, true} {
			config := RuleConfig{Rules: []Rule{{ID: "count", Type: "inference", Subtype: "count", Enabled: enabled, Scope: scope, Settings: RuleSettings{CountMode: "maximum"}}}}
			migrated := migrateCountPrograms(config)
			require.Equal(t, enabled, migrated.Rules[0].Enabled)
			results := []*countSelection{}
			for _, candidate := range []RuleConfig{config, migrated} {
				records, err := loadMatchingRecords(ctx, s.DB, candidate)
				require.NoError(t, err)
				result, conflict, err := chooseCountRule(ctx, candidate, libraries[0], track, recordIndex(records))
				require.NoError(t, err)
				require.False(t, conflict)
				results = append(results, result)
			}
			if results[0] == nil {
				require.Nil(t, results[1])
			} else {
				require.NotNil(t, results[1])
				require.Equal(t, results[0].Amount, results[1].Amount)
				require.Equal(t, results[0].Selected, results[1].Selected)
				require.Equal(t, results[0].Snapshots, results[1].Snapshots)
			}
		}
	}
}

func TestCountProgramsCanSelectRawMetadataAndApplyArithmeticReversibly(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	first := libraryFixtureTrack("A", 7)
	first["Genre"] = "Jazz"
	second := libraryFixtureTrack("A", 10)
	second["Genre"] = "Rock"
	importLibrarySnapshot(t, s, "ONE", "2012-01-01T00:00:00Z", []map[string]any{first})
	importLibrarySnapshot(t, s, "ONE", "2013-01-01T00:00:00Z", []map[string]any{second})
	program := namingFixture(t, `{"steps":[{"set":"count.snapshots","value":{"op":"where","args":[{"op":"field","field":"library.snapshots"},{"op":"eq","args":[{"op":"get","args":[{"op":"get","args":[{"op":"get","args":[{"op":"field","field":"item"},{"op":"literal","value":"raw"}]},{"op":"literal","value":"fields"}]},{"op":"literal","value":"Genre"}]},{"op":"literal","value":"Jazz"}]}]}},{"set":"count.total","value":{"op":"multiply","args":[{"op":"max","args":[{"op":"field","field":"library.counts"}]},{"op":"literal","value":2}]}},{"set":"count.snapshot","value":{"op":"field","field":"library.maximum_snapshot"}}]}`)
	saveTestRules(t, s, Rule{ID: "math", Type: "inference", Subtype: "count", Enabled: true, Program: program}, countRules()[1])
	aggregate := readAggregate(t, s)
	require.EqualValues(t, 14, aggregate.Available)
	require.EqualValues(t, 14, aggregate.Attributed)
	require.Equal(t, 1, len(aggregate.Records))
	for _, enabled := range []bool{false, true} {
		change := currentChange(t, s)
		for i := range change.Config.Rules {
			if change.Config.Rules[i].ID == "math" {
				change.Config.Rules[i].Enabled = enabled
			}
		}
		_, err := s.Save(ctx, change)
		require.NoError(t, err)
		if enabled {
			require.EqualValues(t, 14, readAggregate(t, s).Available)
		} else {
			require.Zero(t, count(t, s, "source_aggregates"))
		}
	}
	require.Equal(t, 4, count(t, s, "source_records"))
	_, err := s.Rebuild(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 14, readAggregate(t, s).Available)
}

func TestCountProgramUnresolvedValuesRemainInspectable(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	importLibrarySnapshot(t, s, "ONE", "2012-01-01T00:00:00Z", []map[string]any{libraryFixtureTrack("A", 7)})
	program := namingFixture(t, `{"steps":[{"set":"count.total","value":{"op":"divide","args":[{"op":"literal","value":7},{"op":"literal","value":2}]}}]}`)
	saveTestRules(t, s, Rule{ID: "fraction", Name: "Count check", Type: "inference", Subtype: "count", Enabled: true, Program: program})
	aggregate := readAggregate(t, s)
	require.Zero(t, aggregate.Available)
	require.Equal(t, "unresolved cumulative count", aggregate.Reason)
	inspection, err := s.Inspect(ctx, aggregate.Representative)
	require.NoError(t, err)
	require.Len(t, inspection.FilterIssues, 1)
	require.Equal(t, "Count check", inspection.FilterIssues[0].Name)
	_, err = filters.CompileProgram(*countProgram("maximum", RuleScope{}), countSchema())
	require.NoError(t, err)
}

func TestCountProgramWithoutSelectedDateRecordContributesOnlyUndatedCounts(t *testing.T) {
	s := testStore(t)
	importLibrarySnapshot(t, s, "ONE", "2012-01-01T00:00:00Z", []map[string]any{libraryFixtureTrack("A", 7)})
	saveTestRules(t, s, Rule{ID: "manual", Type: "inference", Subtype: "count", Enabled: true, Program: namingFixture(t, `{"steps":[{"set":"count.total","value":{"op":"max","args":[{"op":"field","field":"library.counts"}]}}]}`)}, countRules()[1], Rule{ID: "dates", Type: "inference", Subtype: "time", Enabled: true, Scope: RuleScope{Service: "itunes"}, Settings: RuleSettings{UseLastPlayed: true, EstimateTimes: true}})
	aggregate := readAggregate(t, s)
	require.EqualValues(t, 7, aggregate.Undated)
	require.Zero(t, aggregate.LastPlayed)
	require.Zero(t, aggregate.Estimated)
	require.Zero(t, count(t, s, "listens"))
}

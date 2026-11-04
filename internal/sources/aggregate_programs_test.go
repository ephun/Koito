package sources

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAggregateMigrationPreservesConditionsDisabledSettingsAndPrecedence(t *testing.T) {
	share := 37
	r := matchingRecord{ID: 12, SourceID: 3, Service: "itunes", FileHash: "file", LibraryID: "library", TrackID: "track", Values: recordValues{Artist: "Artist", Title: "Song"}}
	for _, scope := range []RuleScope{{Service: "itunes"}, {Service: "itunes", SourceID: 3}, {Service: "itunes", SourceID: 9}, {Service: "itunes", FileHash: "file"}, {Service: "itunes", FileHash: "other"}, {Service: "itunes", LibraryID: "library"}, {Service: "itunes", LibraryID: "other"}, {Service: "itunes", TrackID: "track"}, {Service: "itunes", RecordID: 12}, {Service: "itunes", RecordID: 99}, {Service: "itunes", Artist: "Artist"}} {
		for _, enabled := range []bool{true, false} {
			config := RuleConfig{Rules: []Rule{{ID: "share", Type: "attribution", Subtype: "aggregate", Enabled: enabled, Scope: scope, Settings: RuleSettings{SharePercent: &share}}}}
			before, _ := json.Marshal(config)
			migrated := migrateAggregatePrograms(config)
			require.NoError(t, prepareAggregatePrograms(migrated))
			after, _ := json.Marshal(config)
			require.Equal(t, before, after)
			require.Equal(t, enabled, migrated.Rules[0].Enabled)
			require.Equal(t, share, *migrated.Rules[0].Settings.SharePercent)
			for _, record := range []matchingRecord{r, {ID: 13, SourceID: 8, Service: "itunes", Values: recordValues{Artist: "Other"}}} {
				original, conflict := chooseRule(config, "attribution", "aggregate", func(rule Rule) bool { return scopeContains(rule.Scope, record, time.Time{}, time.Time{}) })
				converted, convertedConflict := chooseAggregateRule(migrated, record, time.Time{}, time.Time{})
				require.Equal(t, conflict, convertedConflict)
				if original == nil {
					require.Nil(t, converted)
				} else {
					require.NotNil(t, converted)
					require.Equal(t, original.Settings.SharePercent, converted.Settings.SharePercent)
					require.Equal(t, scopeRank(original.Scope), rulePriority(*converted))
				}
			}
		}
	}
	otherShare := 50
	config := RuleConfig{Rules: []Rule{{ID: "a", Type: "attribution", Subtype: "aggregate", Enabled: true, Scope: RuleScope{Service: "itunes"}, Settings: RuleSettings{SharePercent: &share}}, {ID: "b", Type: "attribution", Subtype: "aggregate", Enabled: true, Scope: RuleScope{Service: "itunes"}, Settings: RuleSettings{SharePercent: &otherShare}}}}
	migrated := migrateAggregatePrograms(config)
	_, conflict := chooseAggregateRule(migrated, r, time.Time{}, time.Time{})
	require.True(t, conflict)
	migrated.Rules[1].Priority = new(int)
	*migrated.Rules[1].Priority = 100
	selected, conflict := chooseAggregateRule(migrated, r, time.Time{}, time.Time{})
	require.False(t, conflict)
	require.Equal(t, "b", selected.ID)
}

func TestAggregateProgramPublishesReversibleWholeSharesFromActualLibraryRecords(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	importLibrarySnapshot(t, s, "ONE", "2012-01-01T00:00:00Z", []map[string]any{libraryFixtureTrack("A", 7)})
	program := namingFixture(t, `{"steps":[{"if":{"op":"eq","args":[{"op":"field","field":"raw:/fields/Persistent ID"},{"op":"literal","value":"A"}]},"then":[{"set":"share.percent","value":{"op":"divide","args":[{"op":"literal","value":100},{"op":"literal","value":2}]}}]}]}`)
	countRule := countRules()[0]
	saveTestRules(t, s, countRule, Rule{ID: "program-share", Name: "Library share", Type: "attribution", Subtype: "aggregate", Enabled: true, Program: program})
	var original string
	require.NoError(t, s.DB.QueryRow(`SELECT payload FROM source_records WHERE kind='library_track'`).Scan(&original))
	aggregate := readAggregate(t, s)
	require.EqualValues(t, 7, aggregate.Available)
	require.EqualValues(t, 3, aggregate.Attributed)
	require.EqualValues(t, 3, aggregate.Undated)
	require.Equal(t, "aggregate", aggregate.Attribution.Basis)
	for _, enabled := range []bool{false, true} {
		change := currentChange(t, s)
		for i := range change.Config.Rules {
			if change.Config.Rules[i].ID == "program-share" {
				change.Config.Rules[i].Enabled = enabled
			}
		}
		_, err := s.Save(ctx, change)
		require.NoError(t, err)
		if enabled {
			require.EqualValues(t, 3, readAggregate(t, s).Attributed)
		} else {
			require.Zero(t, readAggregate(t, s).Attributed)
		}
		_, err = s.Rebuild(ctx)
		require.NoError(t, err)
		var after string
		require.NoError(t, s.DB.QueryRow(`SELECT payload FROM source_records WHERE kind='library_track'`).Scan(&after))
		require.Equal(t, original, after)
	}
	change := currentChange(t, s)
	for i := range change.Config.Rules {
		if change.Config.Rules[i].ID == "program-share" {
			change.Config.Rules[i].Program = namingFixture(t, `{"steps":[{"set":"share.percent","value":{"op":"number","args":[{"op":"literal","value":"101"}]}}]}`)
		}
	}
	_, err := s.Save(ctx, change)
	require.NoError(t, err)
	aggregate = readAggregate(t, s)
	require.Zero(t, aggregate.Attributed)
	require.Equal(t, "unresolved aggregate filter", aggregate.Attribution.Basis)
	var id int64
	require.NoError(t, s.DB.QueryRow(`SELECT id FROM source_records WHERE kind='library_track'`).Scan(&id))
	inspection, err := s.Inspect(ctx, id)
	require.NoError(t, err)
	require.Len(t, inspection.FilterIssues, 1)
	require.Contains(t, inspection.FilterIssues[0].Reason, "allowed range")
}

func TestAggregateProgramBranchesRetainDirectAttributionPrecedence(t *testing.T) {
	r := matchingRecord{Service: "itunes", Values: recordValues{Artist: "Artist"}}
	program := namingFixture(t, `{"steps":[{"if":{"op":"eq","args":[{"op":"field","field":"original.artist"},{"op":"literal","value":"Artist"}]},"then":[{"set":"share.percent","value":{"op":"literal","value":25}}],"else":[{"set":"share.percent","value":{"op":"literal","value":75}}]}]}`)
	config := RuleConfig{Rules: []Rule{{ID: "share", Type: "attribution", Subtype: "aggregate", Enabled: true, Program: program}}}
	for _, test := range []struct {
		artist string
		want   int
	}{{"Artist", 25}, {"Other", 75}} {
		r.Values.Artist = test.artist
		out := aggregateAttribution(r, config, time.Time{}, time.Time{}, nil)
		require.Equal(t, "partial", out.Listener)
		require.Equal(t, test.want, *out.Share)
	}
	config.Rules = append(config.Rules, Rule{ID: "direct", Type: "attribution", Subtype: "direct", Enabled: true, Scope: RuleScope{Service: "itunes"}, Settings: RuleSettings{Listener: "other"}})
	out := aggregateAttribution(r, config, time.Time{}, time.Time{}, nil)
	require.Equal(t, "other", out.Listener)
	require.Equal(t, "direct", out.Basis)
}

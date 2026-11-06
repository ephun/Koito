package sources

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTimeMigrationPreservesFormatsZonesPrecisionAndSpotifyBasis(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	mustImport(t, s, []byte("["+complete+"]"), "spotify.json", "")
	for _, name := range []string{"lastfm-headerless.csv", "lastfm-named.csv", "lastfm-pages.json", "lastfm-normalized.jsonl"} {
		data, err := os.ReadFile("testdata/" + name)
		require.NoError(t, err)
		_, err = s.Import(ctx, bytes.NewReader(data), name, 1)
		require.NoError(t, err)
	}
	base := currentChange(t, s).Config
	for _, zone := range []string{"", "UTC", "America/Denver"} {
		for _, enabled := range []bool{false, true} {
			for _, basis := range []string{"end", "estimated_start"} {
				config := RuleConfig{Rules: append([]Rule(nil), base.Rules...)}
				config.Rules[1].Settings.SpotifyTime = basis
				config.Rules[2].Settings.Timezone = zone
				config.Rules[2].Enabled = enabled
				before, _ := json.Marshal(config)
				migrated := migrateTimePrograms(config)
				unchanged, _ := json.Marshal(config)
				require.Equal(t, before, unchanged)
				old, err := loadMatchingRecords(ctx, s.DB, config)
				require.NoError(t, err)
				next, err := loadMatchingRecords(ctx, s.DB, migrated)
				require.NoError(t, err)
				require.Len(t, next, len(old))
				for i := range old {
					require.Equal(t, old[i].ID, next[i].ID)
					require.Equal(t, old[i].Start, next[i].Start, "zone=%s enabled=%v basis=%s record=%d", zone, enabled, basis, old[i].ID)
					require.Equal(t, old[i].Basis, next[i].Basis)
					require.Equal(t, old[i].Values, next[i].Values)
				}
			}
		}
	}
}

func TestTimeMigrationPreservesFileOverridesAndLibraryDates(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	data, err := os.ReadFile("testdata/lastfm-named.csv")
	require.NoError(t, err)
	_, err = s.Import(ctx, bytes.NewReader(data), "history.csv", 1)
	require.NoError(t, err)
	var hash string
	require.NoError(t, s.DB.QueryRow(`SELECT file_hash FROM source_records LIMIT 1`).Scan(&hash))
	change := currentChange(t, s)
	change.Config.Rules[2].Settings.Timezone = "UTC"
	change.Config.Rules = append(change.Config.Rules, Rule{ID: "file-time", Type: "inference", Subtype: "time", Enabled: true, Scope: RuleScope{Service: "lastfm", FileHash: hash}, Settings: RuleSettings{Timezone: "America/Denver"}})
	old, err := loadMatchingRecords(ctx, s.DB, change.Config)
	require.NoError(t, err)
	next, err := loadMatchingRecords(ctx, s.DB, migrateTimePrograms(change.Config))
	require.NoError(t, err)
	for i := range old {
		require.Equal(t, old[i].Start, next[i].Start)
	}
	for _, last := range []bool{false, true} {
		for _, estimate := range []bool{false, true} {
			library := testStore(t)
			importLibrarySnapshot(t, library, "ONE", "2012-01-01T00:00:00Z", []map[string]any{libraryFixtureTrack("A", 7)})
			config := currentChange(t, library).Config
			config.Rules = append(config.Rules, countRules()...)
			config.Rules = append(config.Rules, Rule{ID: "dates", Type: "inference", Subtype: "time", Enabled: true, Scope: RuleScope{Service: "itunes"}, Settings: RuleSettings{UseLastPlayed: last, EstimateTimes: estimate}})
			plans := []matchPlan{}
			for _, candidate := range []RuleConfig{config, migrateTimePrograms(config)} {
				records, err := loadMatchingRecords(ctx, library.DB, candidate)
				require.NoError(t, err)
				plan, err := calculateView(ctx, library.DB, records, candidate, nil)
				require.NoError(t, err)
				plans = append(plans, plan)
			}
			require.Equal(t, plans[0].Aggregates, plans[1].Aggregates)
			require.Equal(t, plans[0].Plays, plans[1].Plays)
		}
	}
}

func TestTimeOffsetUsesPreStageValuesAndKeepsNanoseconds(t *testing.T) {
	s := testStore(t)
	config := migrateTimePrograms(currentChange(t, s).Config)
	program := namingFixture(t, `{"steps":[{"if":{"op":"eq","args":[{"op":"field","field":"record.time"},{"op":"date","args":[{"op":"literal","value":"2020-01-02T03:04:05Z"}]}]},"then":[{"set":"time.offset","value":{"op":"milliseconds","args":[{"op":"literal","value":0.000001}]}}]}]}`)
	config.Rules = append(config.Rules, Rule{ID: "adjust", Type: "inference", Subtype: "time", Enabled: true, Program: program})
	r := matchingRecord{Service: "spotify", Parser: Parser, Payload: complete}
	interpretMatchingRecord(&r, config)
	want := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC).Add(-180001 * time.Millisecond).Add(time.Nanosecond)
	require.Equal(t, want, r.Start)
	require.Empty(t, r.TimeIssues)
}

func TestTimeProgramsResolveRawConditionsAndConflictsWithoutPartialActions(t *testing.T) {
	s := testStore(t)
	config := migrateTimePrograms(currentChange(t, s).Config)
	program := namingFixture(t, `{"steps":[{"if":{"op":"eq","args":[{"op":"field","field":"raw:/unknown/keep"},{"op":"literal","value":true}]},"then":[{"set":"time.offset","value":{"op":"seconds","args":[{"op":"literal","value":2}]}}]}]}`)
	config.Rules = append(config.Rules, Rule{ID: "adjust", Type: "inference", Subtype: "time", Enabled: true, Program: program})
	r := matchingRecord{Service: "spotify", Parser: Parser, Payload: complete}
	require.NoError(t, retainRawFields(&r, requestedRawFields(config)))
	interpretMatchingRecord(&r, config)
	want := time.Date(2020, 1, 2, 3, 1, 6, 999000000, time.UTC)
	require.Equal(t, want, r.Start)
	invalid := namingFixture(t, `{"steps":[{"set":"time.zone","value":{"op":"literal","value":"not/a/zone"}},{"set":"time.offset","value":{"op":"seconds","args":[{"op":"literal","value":100}]}}]}`)
	config.Rules = append(config.Rules, Rule{ID: "invalid", Type: "inference", Subtype: "time", Enabled: true, Program: invalid})
	interpretMatchingRecord(&r, config)
	require.Equal(t, want, r.Start)
	require.NotEmpty(t, r.TimeIssues)
	config.Rules = append(config.Rules, Rule{ID: "conflict", Type: "inference", Subtype: "time", Enabled: true, Program: namingFixture(t, `{"steps":[{"set":"time.offset","value":{"op":"seconds","args":[{"op":"literal","value":3}]}}]}`)})
	interpretMatchingRecord(&r, config)
	require.Equal(t, want.Add(-2*time.Second), r.Start)
	found := false
	for _, issue := range r.TimeIssues {
		found = found || strings.Contains(issue.Reason, "disagree")
	}
	require.True(t, found)
}

func TestRemovingOrDisablingMigratedSpotifyTimeDoesNotReactivateLegacySetting(t *testing.T) {
	s := testStore(t)
	config := migrateTimePrograms(currentChange(t, s).Config)
	for i := range config.Rules {
		if config.Rules[i].ID == "spotify-time" {
			config.Rules[i].Enabled = false
		}
	}
	r := matchingRecord{Service: "spotify", Parser: Parser, Payload: complete}
	interpretMatchingRecord(&r, config)
	require.Equal(t, time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC), r.Start)
	rules := []Rule{}
	for _, rule := range config.Rules {
		if rule.ID != "spotify-time" {
			rules = append(rules, rule)
		}
	}
	config.Rules = rules
	config = migrateTimePrograms(config)
	for _, rule := range config.Rules {
		require.NotEqual(t, "spotify-time", rule.ID)
	}
	interpretMatchingRecord(&r, config)
	require.Equal(t, time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC), r.Start)
}

package sources

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestHeuristicMigrationPreservesEvidenceWindowsAndDisabledSettings(t *testing.T) {
	s := testStore(t)
	base := currentChange(t, s).Config
	stamp := func(year int) time.Time { return time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC) }
	records := []matchingRecord{{ID: 1, SourceID: 1, Service: "spotify", UserID: 1, Start: stamp(2020), Values: recordValues{Title: "Song", Artist: "Artist", Album: "Album", Reason: "counted", Precision: int64(time.Second)}}, {ID: 2, SourceID: 2, Service: "lastfm", UserID: 1, Start: stamp(2019), Values: recordValues{Title: "Song", Artist: "Artist", Album: "Album", Reason: "counted", Precision: int64(time.Second)}}}
	for _, scope := range []RuleScope{{Service: "spotify"}, {Service: "spotify", Artist: "Artist"}, {Service: "spotify", Artist: "Other"}, {Service: "spotify", SourceID: 1}, {Service: "spotify", SourceID: 99}, {Service: "spotify", RecordID: 1}, {Service: "spotify", From: "2018-01-01T00:00:00Z", Until: "2021-01-01T00:00:00Z"}, {Service: "spotify", From: "2020-01-01T00:00:00Z", Until: "2021-01-01T00:00:00Z"}} {
		for _, minimum := range []int{1, 2} {
			for _, enabled := range []bool{true, false} {
				config := RuleConfig{Rules: append(append([]Rule(nil), base.Rules...), Rule{ID: "mine", Type: "attribution", Subtype: "direct", Enabled: true, Scope: RuleScope{Service: "lastfm"}, Settings: RuleSettings{Listener: "self"}}, Rule{ID: "guess", Type: "attribution", Subtype: "heuristic", Enabled: enabled, Scope: scope, Settings: RuleSettings{EvidenceSourceID: 2, MinimumMatches: minimum}})}
				before, _ := json.Marshal(config)
				migrated := migrateHeuristicPrograms(config)
				require.NoError(t, prepareHeuristicPrograms(migrated))
				after, _ := json.Marshal(config)
				require.Equal(t, before, after)
				plans := []matchPlan{}
				for _, candidate := range []RuleConfig{config, migrated} {
					plan, err := calculateMatches(context.Background(), records, candidate, nil)
					require.NoError(t, err)
					applyAttribution(&plan, candidate)
					plans = append(plans, plan)
				}
				require.Equal(t, len(plans[0].Plays), len(plans[1].Plays), "scope=%+v minimum=%d enabled=%v", scope, minimum, enabled)
				for key, attribution := range plans[0].Attributions {
					require.Equal(t, attribution.Listener, plans[1].Attributions[key].Listener)
				}
			}
		}
	}
}

func TestHeuristicProgramsUseOnlyDirectEvidenceAndExposeSourceFailures(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	mustImport(t, s, []byte("["+complete+"]"), "spotify.json", "")
	importLastfm(t, s, "["+lf("Song", "Album", time.Date(2019, 1, 1, 0, 0, 0, 0, time.UTC).Unix())+"]")
	var lastfm int64
	require.NoError(t, s.DB.QueryRow(`SELECT id FROM source_accounts WHERE service='lastfm'`).Scan(&lastfm))
	legacy := Rule{ID: "guess", Type: "attribution", Subtype: "heuristic", Enabled: true, Scope: RuleScope{Service: "spotify"}, Settings: RuleSettings{EvidenceSourceID: lastfm, MinimumMatches: 1}}
	guess := migrateHeuristicPrograms(RuleConfig{Rules: []Rule{legacy}}).Rules[0]
	saveTestRules(t, s, guess)
	require.Equal(t, 1, count(t, s, "listens"))
	saveTestRules(t, s, Rule{ID: "mine", Type: "attribution", Subtype: "direct", Enabled: true, Scope: RuleScope{Service: "lastfm"}, Settings: RuleSettings{Listener: "self"}})
	require.Equal(t, 2, count(t, s, "listens"))
	var spotifyID int64
	require.NoError(t, s.DB.QueryRow(`SELECT r.id FROM source_records r JOIN source_accounts a ON a.id=r.source_id WHERE a.service='spotify'`).Scan(&spotifyID))
	inspection, err := s.Inspect(ctx, spotifyID)
	require.NoError(t, err)
	require.Empty(t, inspection.FilterIssues)
	change := currentChange(t, s)
	for i := range change.Config.Rules {
		if change.Config.Rules[i].ID == "guess" {
			change.Config.Rules[i].Program = namingFixture(t, `{"steps":[{"if":{"op":"ge","args":[{"op":"count","args":[{"op":"field","field":"evidence:999"}]},{"op":"literal","value":1}]},"then":[{"set":"listener","value":{"op":"literal","value":"self"}}]}]}`)
		}
	}
	_, err = s.Save(ctx, change)
	require.NoError(t, err)
	inspection, err = s.Inspect(ctx, spotifyID)
	require.NoError(t, err)
	require.Len(t, inspection.FilterIssues, 1)
	require.Contains(t, inspection.FilterIssues[0].Reason, "source is unavailable")
	for i := range change.Config.Rules {
		if change.Config.Rules[i].ID == "guess" {
			change.Config.Rules[i].Enabled = false
		}
	}
	change.BaseVersion = currentChange(t, s).BaseVersion
	_, err = s.Save(ctx, change)
	require.NoError(t, err)
	require.Equal(t, 2, count(t, s, "listens"))
	require.Equal(t, 2, count(t, s, "source_records"))
}

func TestHeuristicLibraryMigrationRetainsWholeShareAndIntervalEvidence(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	importLibrarySnapshot(t, s, "ONE", "2012-01-01T00:00:00Z", []map[string]any{libraryFixtureTrack("A", 7)})
	importLastfm(t, s, "["+lf("Song", "Album", time.Date(2011, 1, 1, 0, 0, 0, 0, time.UTC).Unix())+"]")
	var lastfm int64
	require.NoError(t, s.DB.QueryRow(`SELECT id FROM source_accounts WHERE service='lastfm'`).Scan(&lastfm))
	share := 50
	saveTestRules(t, s, countRules()[0], Rule{ID: "mine", Type: "attribution", Subtype: "direct", Enabled: true, Scope: RuleScope{Service: "lastfm"}, Settings: RuleSettings{Listener: "self"}}, Rule{ID: "guess", Type: "attribution", Subtype: "heuristic", Enabled: true, Scope: RuleScope{Service: "itunes"}, Settings: RuleSettings{EvidenceSourceID: lastfm, MinimumMatches: 1, SharePercent: &share}})
	before := readAggregate(t, s)
	require.EqualValues(t, 3, before.Attributed)
	change := currentChange(t, s)
	change.Config = migrateHeuristicPrograms(change.Config)
	_, err := s.Save(ctx, change)
	require.NoError(t, err)
	after := readAggregate(t, s)
	require.Equal(t, before.Attributed, after.Attributed)
	require.Equal(t, before.Undated, after.Undated)
	require.Equal(t, before.Observed, after.Observed)
	_, err = s.Rebuild(ctx)
	require.NoError(t, err)
	require.Equal(t, after.Attributed, readAggregate(t, s).Attributed)
}

func TestHeuristicPercentagesRequireCumulativeRecords(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	mustImport(t, s, []byte("["+complete+"]"), "spotify.json", "")
	saveTestRules(t, s, Rule{ID: "share", Type: "attribution", Subtype: "heuristic", Enabled: true, Program: namingFixture(t, `{"steps":[{"set":"share.percent","value":{"op":"literal","value":50}}]}`)})
	require.Zero(t, count(t, s, "listens"))
	inspection, err := s.Inspect(ctx, 1)
	require.NoError(t, err)
	require.Len(t, inspection.FilterIssues, 1)
	require.Equal(t, "Percentages require cumulative listening counts.", inspection.FilterIssues[0].Reason)
}

func TestPersonalEvidenceCacheRetainsOwnerAndRollsBackWithPublication(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	importLastfm(t, s, "["+lf("Song", "Album", time.Date(2019, 1, 1, 0, 0, 0, 0, time.UTC).Unix())+"]")
	_, err := s.DB.Exec(`INSERT INTO users(id,username,role,password) VALUES(2,'second','admin',x'00')`)
	require.NoError(t, err)
	_, err = s.Import(ctx, bytes.NewBufferString("["+lf("Song", "Album", time.Date(2019, 2, 1, 0, 0, 0, 0, time.UTC).Unix())+"]"), "second.json", 2)
	require.NoError(t, err)
	saveTestRules(t, s, Rule{ID: "mine", Type: "attribution", Subtype: "direct", Enabled: true, Scope: RuleScope{Service: "lastfm"}, Settings: RuleSettings{Listener: "self"}})
	config := currentChange(t, s)
	rows, err := inspectedRecords(ctx, s.DB, config.Config, `WHERE a.user_id=1 LIMIT 1`)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	index, err := inspectionEvidence(ctx, s.DB, config.Config, config.BaseVersion, rows[0].matchingRecord)
	require.NoError(t, err)
	require.NotNil(t, index)
	seen := map[int64]bool{}
	for _, values := range index.byMusic {
		for _, value := range values {
			require.EqualValues(t, 1, value.record.UserID)
			seen[value.record.ID] = true
		}
	}
	require.Len(t, seen, 1)
	before := count(t, s, "source_personal_evidence")
	require.Equal(t, 2, before)
	_, err = s.DB.Exec(`CREATE TRIGGER reject_evidence BEFORE INSERT ON source_personal_evidence BEGIN SELECT RAISE(ABORT,'cache failed'); END`)
	require.NoError(t, err)
	_, err = s.Save(ctx, config)
	require.ErrorContains(t, err, "cache failed")
	require.Equal(t, before, count(t, s, "source_personal_evidence"))
	require.Equal(t, config.BaseVersion, currentChange(t, s).BaseVersion)
	require.Equal(t, 2, count(t, s, "source_records"))
	require.Equal(t, 2, count(t, s, "listens"))
}

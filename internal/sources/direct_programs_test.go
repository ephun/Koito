package sources

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDirectProgramMigrationPreservesScopeAndPriority(t *testing.T) {
	start := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	r := matchingRecord{ID: 12, SourceID: 3, Service: "spotify", FileHash: "file", LibraryID: "library", TrackID: "track", Device: "device", Start: start, Values: recordValues{Artist: "Artist", Title: "Song"}}
	for _, scope := range []RuleScope{{}, {Service: "spotify"}, {Service: "lastfm"}, {SourceID: 3}, {SourceID: 9}, {FileHash: "file"}, {FileHash: "other"}, {LibraryID: "library"}, {TrackID: "track"}, {RecordID: 12}, {RecordID: 99}, {Artist: "Artist"}, {Device: "device"}, {Device: "missing"}, {From: "2020-01-01T00:00:00Z", Until: "2020-01-03T00:00:00Z"}, {From: "2021-01-01T00:00:00Z"}} {
		for _, enabled := range []bool{true, false} {
			config := RuleConfig{Rules: []Rule{{ID: "test", Type: "attribution", Subtype: "direct", Enabled: enabled, Scope: scope, Settings: RuleSettings{Listener: "other"}}}}
			original, _ := json.Marshal(config)
			migrated := migrateDirectPrograms(config)
			require.NoError(t, prepareDirectPrograms(migrated))
			unchanged, _ := json.Marshal(config)
			require.Equal(t, string(original), string(unchanged))
			for _, record := range []matchingRecord{r, {ID: 13, Service: "spotify", Start: start, Values: recordValues{Artist: "Other"}}, {ID: 14, Service: "itunes"}} {
				from, until := recordInterval(record)
				require.Equal(t, directAttribution(record, config, from, until), directAttribution(record, migrated, from, until), "scope=%+v enabled=%v", scope, enabled)
			}
		}
	}
	conflicts := RuleConfig{Rules: []Rule{{ID: "a", Type: "attribution", Subtype: "direct", Enabled: true, Settings: RuleSettings{Listener: "self"}}, {ID: "b", Type: "attribution", Subtype: "direct", Enabled: true, Settings: RuleSettings{Listener: "other"}}}}
	for _, config := range []RuleConfig{conflicts, migrateDirectPrograms(conflicts)} {
		require.NoError(t, prepareDirectPrograms(config))
		a := directAttribution(r, config, start, start.Add(time.Second))
		require.Equal(t, "unknown", a.Listener)
		require.Equal(t, "conflicting direct rules", a.Basis)
	}
}

func TestDirectBlocksApplyElseAndRestoreExistingHistory(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	mustImport(t, s, []byte("["+complete+"]"), "spotify.json", "")
	program := namingFixture(t, `{"steps":[{"if":{"op":"eq","args":[{"op":"field","field":"source.service"},{"op":"literal","value":"spotify"}]},"then":[{"set":"listener","value":{"op":"literal","value":"other"}}],"else":[{"set":"listener","value":{"op":"literal","value":"self"}}]}]}`)
	for _, enabled := range []bool{true, false} {
		change := currentChange(t, s)
		if len(change.Config.Rules) == 3 {
			change.Config.Rules = append(change.Config.Rules, Rule{ID: "listener", Type: "attribution", Subtype: "direct", Enabled: enabled, Program: program})
		} else {
			change.Config.Rules[len(change.Config.Rules)-1].Enabled = enabled
		}
		_, err := s.Save(ctx, change)
		require.NoError(t, err)
		want := 0
		if !enabled {
			want = 1
		}
		require.Equal(t, want, count(t, s, "listens"))
		require.Equal(t, 1, count(t, s, "source_records"))
	}
}

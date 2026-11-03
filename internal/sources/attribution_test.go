package sources

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAttributionSourceRecordDeviceAndDateScopes(t *testing.T) {
	s := testStore(t)
	one := strings.TrimSuffix(complete, "}") + `,"platform":"Desktop"}`
	two := strings.ReplaceAll(one, "2020-01-02T03:04:05Z", "2020-01-03T03:04:05Z")
	two = strings.ReplaceAll(two, "Desktop", "Phone")
	mustImport(t, s, []byte("["+one+","+two+"]"), "history.json", "")
	saveTestRules(t, s, Rule{ID: "shared", Type: "attribution", Subtype: "direct", Enabled: true, Scope: RuleScope{Service: "spotify"}, Settings: RuleSettings{Listener: "other"}}, Rule{ID: "desktop", Type: "attribution", Subtype: "direct", Enabled: true, Scope: RuleScope{Service: "spotify", Device: "Desktop", From: "2020-01-01T00:00:00Z", Until: "2020-01-03T00:00:00Z"}, Settings: RuleSettings{Listener: "self"}})
	require.Equal(t, 1, count(t, s, "listens"))
	saveTestRules(t, s, Rule{ID: "record", Type: "attribution", Subtype: "direct", Enabled: true, Scope: RuleScope{RecordID: 2}, Settings: RuleSettings{Listener: "self"}})
	require.Equal(t, 2, count(t, s, "listens"))
	change := currentChange(t, s)
	change.Config.Rules[5].Settings.Listener = "unknown"
	_, err := s.Save(context.Background(), change)
	require.NoError(t, err)
	require.Equal(t, 1, count(t, s, "listens"))
	require.Equal(t, 2, count(t, s, "source_records"))
}

func TestAttributionMatchedListenerConflictsRemainInspectable(t *testing.T) {
	s := testStore(t)
	mustImport(t, s, []byte("["+complete+"]"), "spotify.json", "")
	importLastfm(t, s, "["+lf("Song", "Album", 1577934065)+"]")
	saveTestRules(t, s, Rule{ID: "spotify-other", Type: "attribution", Subtype: "direct", Enabled: true, Scope: RuleScope{Service: "spotify"}, Settings: RuleSettings{Listener: "other"}}, Rule{ID: "lastfm-self", Type: "attribution", Subtype: "direct", Enabled: true, Scope: RuleScope{Service: "lastfm"}, Settings: RuleSettings{Listener: "self"}})
	require.Zero(t, count(t, s, "listens"))
	require.Equal(t, 2, count(t, s, "source_play_records"))
	var text string
	require.NoError(t, s.DB.QueryRow(`SELECT attribution_json FROM source_play_details`).Scan(&text))
	var a Attribution
	require.NoError(t, json.Unmarshal([]byte(text), &a))
	require.Equal(t, "unknown", a.Listener)
	require.Equal(t, "conflicting listeners", a.Basis)
	inspection, err := s.Inspect(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, inspection.Contributors, 2)
	change := currentChange(t, s)
	change.Config.Rules[3].Enabled = false
	_, err = s.Save(context.Background(), change)
	require.NoError(t, err)
	require.Equal(t, 1, count(t, s, "listens"))
}

func TestAttributionHeuristicUsesDistinctPersonallyAttributedPlays(t *testing.T) {
	s := testStore(t)
	mustImport(t, s, []byte("["+complete+"]"), "spotify.json", "")
	importLastfm(t, s, "["+lf("Song", "Album", time.Date(2019, 1, 1, 0, 0, 0, 0, time.UTC).Unix())+"]")
	var source int64
	require.NoError(t, s.DB.QueryRow(`SELECT id FROM source_accounts WHERE service='lastfm'`).Scan(&source))
	saveTestRules(t, s, Rule{ID: "mine", Type: "attribution", Subtype: "direct", Enabled: true, Scope: RuleScope{Service: "lastfm"}, Settings: RuleSettings{Listener: "self"}}, Rule{ID: "shared", Type: "attribution", Subtype: "heuristic", Enabled: true, Scope: RuleScope{Service: "spotify"}, Settings: RuleSettings{EvidenceSourceID: source, MinimumMatches: 2}})
	require.Equal(t, 1, count(t, s, "listens"))
	change := currentChange(t, s)
	change.Config.Rules[4].Settings.MinimumMatches = 1
	_, err := s.Save(context.Background(), change)
	require.NoError(t, err)
	require.Equal(t, 2, count(t, s, "listens"))
	saveTestRules(t, s, Rule{ID: "explicit-unknown", Type: "attribution", Subtype: "direct", Enabled: true, Scope: RuleScope{Service: "spotify"}, Settings: RuleSettings{Listener: "unknown"}})
	require.Equal(t, 1, count(t, s, "listens"))
}

func TestAttributionEqualSpecificityConflictsAreUnknown(t *testing.T) {
	s := testStore(t)
	mustImport(t, s, []byte("["+complete+"]"), "history.json", "")
	saveTestRules(t, s, Rule{ID: "mine", Type: "attribution", Subtype: "direct", Enabled: true, Scope: RuleScope{Service: "spotify"}, Settings: RuleSettings{Listener: "self"}}, Rule{ID: "other", Type: "attribution", Subtype: "direct", Enabled: true, Scope: RuleScope{Service: "spotify"}, Settings: RuleSettings{Listener: "other"}})
	require.Zero(t, count(t, s, "listens"))
	require.Equal(t, 1, count(t, s, "source_records"))
}

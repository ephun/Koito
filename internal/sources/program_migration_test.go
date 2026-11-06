package sources

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestUneditedMigrationsKeepLegacyConflictsAndEditsUseTheirAssignments(t *testing.T) {
	r := matchingRecord{Service: "spotify"}
	left := Rule{ID: "left", Type: "attribution", Subtype: "direct", Enabled: true, Settings: RuleSettings{Listener: "self"}}
	right := left
	right.ID = "right"
	right.Settings.CaseSensitive = true
	legacy := RuleConfig{Rules: []Rule{left, right}}
	_, conflict := chooseDirectRule(legacy, r, time.Time{}, time.Time{})
	require.True(t, conflict)
	migrated := migrateDirectPrograms(legacy)
	require.NotNil(t, migrated.Rules[0].Migration)
	_, conflict = chooseDirectRule(migrated, r, time.Time{}, time.Time{})
	require.True(t, conflict)
	original := migrated.Rules[0].Program
	migrated.Rules[0].Program = namingFixture(t, `{"steps":[{"set":"listener","value":{"op":"literal","value":"self"}}]}`)
	_, conflict = chooseDirectRule(migrated, r, time.Time{}, time.Time{})
	require.False(t, conflict)
	migrated.Rules[0].Program = original
	_, conflict = chooseDirectRule(migrated, r, time.Time{}, time.Time{})
	require.True(t, conflict)
	mixed := RuleConfig{Rules: []Rule{legacy.Rules[0], migrated.Rules[1]}}
	_, conflict = chooseDirectRule(mixed, r, time.Time{}, time.Time{})
	require.True(t, conflict)
}

func TestNewAttributionProgramsCompareEffectiveAssignments(t *testing.T) {
	for _, subtype := range []string{"direct", "aggregate", "heuristic"} {
		body := `{"steps":[{"set":"listener","value":{"op":"literal","value":"self"}}]}`
		service := "spotify"
		if subtype == "aggregate" {
			body = `{"steps":[{"set":"share.percent","value":{"op":"literal","value":50}}]}`
			service = "itunes"
		}
		left := Rule{ID: "left", Type: "attribution", Subtype: subtype, Enabled: true, Program: namingFixture(t, body), Settings: RuleSettings{MinimumMatches: 1}}
		right := left
		right.ID = "right"
		right.Settings.MinimumMatches = 100
		right.Settings.CaseSensitive = true
		config := RuleConfig{Rules: []Rule{left, right}}
		r := matchingRecord{Service: service}
		var conflict bool
		switch subtype {
		case "direct":
			_, conflict = chooseDirectRule(config, r, time.Time{}, time.Time{})
		case "aggregate":
			_, conflict = chooseAggregateRule(config, r, time.Time{}, time.Time{})
		case "heuristic":
			_, conflict = chooseHeuristicRule(config, r, time.Time{}, time.Time{}, nil, time.Time{}, time.Time{})
		}
		require.False(t, conflict, subtype)
	}
}

package sources

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/gabehf/koito/internal/filters"
	"github.com/stretchr/testify/require"
)

func repeatedInput(t *testing.T, devices ...string) []byte {
	t.Helper()
	rows := []map[string]any{}
	for i, device := range devices {
		var row map[string]any
		require.NoError(t, json.Unmarshal([]byte(complete), &row))
		row["ts"] = time.Date(2020, 1, 2, 3, 4+i, 5, 0, time.UTC).Format(time.RFC3339)
		row["platform"] = device
		rows = append(rows, row)
	}
	raw, err := json.Marshal(rows)
	require.NoError(t, err)
	return raw
}

func limitRepeated(maximum int) Rule {
	window := operation("hours", literalExpr(1))
	p := assignWhen(operation("ge", operation("count", fieldExpr("repeat.plays")), literalExpr(maximum)), "repetition.count", literalExpr(false))
	p.Steps = append([]filters.Statement{{Set: "repeat.window", Value: &window}}, p.Steps...)
	return Rule{ID: "repeats", Type: "qualification", Subtype: "repetition", Enabled: true, Program: p}
}

func TestRepetitionCountsDistinctPlaysAndReverses(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	mustImport(t, s, repeatedInput(t, "Me", "Me", "Me", "Me", "Me"), "listening.json", "")
	saveTestRules(t, s, limitRepeated(2))
	require.Equal(t, 5, count(t, s, "source_records"))
	require.Equal(t, 2, count(t, s, "listens"))
	change := currentChange(t, s)
	testRule(t, &change.Config, "repeats").Enabled = false
	_, err := s.Save(ctx, change)
	require.NoError(t, err)
	require.Equal(t, 5, count(t, s, "listens"))
	require.Equal(t, 5, count(t, s, "source_records"))
	_, err = filters.CompileProgram(*repetitionProgram(), repetitionSchema())
	require.NoError(t, err)
}

func TestRepetitionFollowsAttribution(t *testing.T) {
	s := testStore(t)
	mustImport(t, s, repeatedInput(t, "Parent", "Parent", "Parent", "Me", "Me"), "shared.json", "")
	other := Rule{ID: "parent", Type: "attribution", Subtype: "direct", Enabled: true, Program: assignWhen(operation("eq", fieldExpr("record.device"), literalExpr("Parent")), "listener", literalExpr("other"))}
	saveTestRules(t, s, other, limitRepeated(2))
	require.Equal(t, 2, count(t, s, "listens"), "Other people's listening must not exhaust the personal repeat limit")
	require.Equal(t, 5, count(t, s, "source_records"))
}

func TestRepetitionFollowsMatching(t *testing.T) {
	s := testStore(t)
	raw := repeatedInput(t, "Me", "Me")
	var rows []json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &rows))
	rows = append(rows, rows[0], rows[1])
	raw, err := json.Marshal(rows)
	require.NoError(t, err)
	mustImport(t, s, raw, "copies.json", "")
	saveTestRules(t, s, limitRepeated(2))
	require.Equal(t, 4, count(t, s, "source_records"))
	require.Equal(t, 2, count(t, s, "listens"), "Supporting records must not become separate repetitions")
}

func TestRepetitionDoesNotReplaceExcludedListeningWithEstimates(t *testing.T) {
	s := testStore(t)
	track := libraryFixtureTrack("A", 10)
	track["Play Date UTC"] = "2020-01-02T04:00:00Z"
	importLibrarySnapshot(t, s, "ONE", "2021-01-01T00:00:00Z", []map[string]any{track})
	mustImport(t, s, repeatedInput(t, "Me", "Me", "Me", "Me", "Me"), "observed.json", "")
	rules := append(countRules(), limitRepeated(2))
	saveTestRules(t, s, rules...)
	aggregate := readAggregate(t, s)
	require.EqualValues(t, 5, aggregate.Observed)
	require.EqualValues(t, 5, aggregate.Undated)
	require.Equal(t, 2, count(t, s, "listens"))
}

func TestRepetitionWindowResetsAndReportsMissingWindow(t *testing.T) {
	s := testStore(t)
	raw := repeatedInput(t, "Me", "Me", "Me")
	var rows []map[string]any
	require.NoError(t, json.Unmarshal(raw, &rows))
	rows[2]["ts"] = "2020-01-03T03:04:05Z"
	raw, err := json.Marshal(rows)
	require.NoError(t, err)
	mustImport(t, s, raw, "days.json", "")
	saveTestRules(t, s, limitRepeated(1))
	require.Equal(t, 2, count(t, s, "listens"))
	change := currentChange(t, s)
	rule := testRule(t, &change.Config, "repeats")
	rule.Program.Steps = rule.Program.Steps[1:]
	_, err = s.Save(context.Background(), change)
	require.NoError(t, err)
	require.Equal(t, 3, count(t, s, "listens"), "Missing lookback must report uncertainty instead of dropping listening")
}

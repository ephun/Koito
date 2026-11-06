package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func derivedSnapshot(t *testing.T, s *Store) map[string][]string {
	t.Helper()
	tables := strings.Fields(`listens source_plays source_interpretations source_projection source_play_details source_play_records source_play_context source_play_tags source_play_groupings source_personal_evidence source_personal_evidence_music source_match_candidates source_aggregates source_aggregate_records track_histories music_history_canonicals track_history_overrides track_catalog_comparisons source_history_variants listen_scope track_scope track_scope_issues aggregate_scope source_record_dependencies source_record_music_keys source_record_dependency_state`)
	snapshot := map[string][]string{}
	for _, table := range tables {
		rows, err := s.DB.Query(`SELECT * FROM ` + table)
		require.NoError(t, err)
		columns, err := rows.Columns()
		require.NoError(t, err)
		var entries []string
		for rows.Next() {
			values := make([]any, len(columns))
			dest := make([]any, len(columns))
			for i := range dest {
				dest[i] = &values[i]
			}
			require.NoError(t, rows.Scan(dest...))
			raw, err := json.Marshal(values)
			require.NoError(t, err)
			entries = append(entries, string(raw))
		}
		require.NoError(t, rows.Err())
		rows.Close()
		sort.Strings(entries)
		snapshot[table] = entries
	}
	return snapshot
}

func assertIncrementalMatchesFull(t *testing.T, s *Store) {
	t.Helper()
	used, err := s.tryIncrementalLive(context.Background())
	require.NoError(t, err)
	require.True(t, used, "Expected connected-component publication")
	partial := derivedSnapshot(t, s)
	_, err = s.Rebuild(context.Background())
	require.NoError(t, err)
	require.Equal(t, partial, derivedSnapshot(t, s))
}

func TestIncrementalLiveMatchesFullPublication(t *testing.T) {
	for _, mode := range []string{"new play", "same second", "repetition", "inclusion", "direct evidence", "other listener", "native retry", "overlapping precision", "manual match", "gameplay context", "estimated time"} {
		t.Run(mode, func(t *testing.T) {
			s := testStore(t)
			ctx := context.Background()
			for i, stamp := range []int64{1000, 1002, 1004} {
				captureExample(t, s, dependencyInput(stamp, fmt.Sprint(i), "Desktop", "Song"), "")
			}
			captureExample(t, s, dependencyInput(1000, "unrelated", "Desktop", "Other"), "")
			require.NoError(t, s.ProcessLive(ctx))
			stamp := int64(2000)
			event, device := "new", "Desktop"
			switch mode {
			case "same second":
				stamp = 1000
				priority := 100
				different := operation("and", operation("present", fieldExpr("left.record.event_id")), operation("present", fieldExpr("right.record.event_id")), operation("ne", fieldExpr("left.record.event_id"), fieldExpr("right.record.event_id")))
				saveTestRules(t, s, Rule{ID: "distinct", Type: "matching", Subtype: "play", Enabled: true, Priority: &priority, Program: assignWhen(different, "match.play", literalExpr(false))})
			case "repetition":
				stamp = 1001
				saveTestRules(t, s, limitRepeated(2))
			case "inclusion":
				saveTestRules(t, s, Rule{ID: "excluded", Type: "inclusion", Subtype: "evidence", Enabled: true, Program: assignWhen(operation("ge", fieldExpr("play.time"), operation("date", literalExpr("1970-01-01T00:30:00Z"))), "view.include", literalExpr(false))})
			case "direct evidence", "other listener":
				priority := 1
				saveTestRules(t, s, Rule{ID: "self", Type: "attribution", Subtype: "direct", Enabled: true, Priority: &priority, Program: assignWhen(literalExpr(true), "listener", literalExpr("self"))})
				if mode == "other listener" {
					device = "Parent"
					priority = 2
					saveTestRules(t, s, Rule{ID: "parent", Type: "attribution", Subtype: "direct", Enabled: true, Priority: &priority, Program: assignWhen(operation("eq", fieldExpr("record.device"), literalExpr("Parent")), "listener", literalExpr("other"))})
				}
			case "native retry":
				event = "0"
			case "overlapping precision":
				stamp = 1001
				saveTestRules(t, s, Rule{ID: "precision", Type: "inference", Subtype: "time", Enabled: true, Program: assignWhen(literalExpr(true), "time.precision", operation("seconds", literalExpr(3)))})
			case "manual match":
				_, err := s.DB.Exec(`INSERT INTO match_decisions(left_record,right_record,action,created_at) VALUES(1,4,'match',1)`)
				require.NoError(t, err)
				_, err = s.Rebuild(ctx)
				require.NoError(t, err)
			case "gameplay context":
				saveTestRules(t, s, Rule{ID: "context", Type: "inference", Subtype: "context", Enabled: true, Program: assignWhen(literalExpr(true), "context.kind", literalExpr("gameplay"))}, Rule{ID: "self", Type: "attribution", Subtype: "direct", Enabled: true, Program: assignWhen(literalExpr(true), "listener", literalExpr("self"))})
			case "estimated time":
				saveTestRules(t, s, Rule{ID: "estimate", Type: "inference", Subtype: "time", Enabled: true, Program: assignWhen(literalExpr(true), "time.at", operation("date", literalExpr("1970-01-02T00:00:00Z")))})
			}
			unpublishedDependency(t, s, dependencyInput(stamp, event, device, "Song"))
			assertIncrementalMatchesFull(t, s)
		})
	}
}

func TestIncrementalLiveFailureRollsBackAndNewCatalogFallsBack(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	captureExample(t, s, dependencyInput(1000, "old", "Desktop", "Song"), "")
	require.NoError(t, s.ProcessLive(ctx))
	unpublishedDependency(t, s, dependencyInput(2000, "new", "Desktop", "Song"))
	before := derivedSnapshot(t, s)
	_, err := s.DB.Exec(`CREATE TRIGGER reject_partial BEFORE INSERT ON source_plays BEGIN SELECT RAISE(ABORT,'publication failed'); END`)
	require.NoError(t, err)
	used, err := s.tryIncrementalLive(ctx)
	require.Error(t, err)
	require.False(t, used)
	require.Equal(t, before, derivedSnapshot(t, s))
	require.Equal(t, 2, count(t, s, "source_records"))
	_, err = s.DB.Exec(`DROP TRIGGER reject_partial`)
	require.NoError(t, err)
	unpublishedDependency(t, s, dependencyInput(3000, "catalog", "Desktop", "New song"))
	used, err = s.tryIncrementalLive(ctx)
	require.NoError(t, err)
	require.False(t, used)
	require.Equal(t, before, derivedSnapshot(t, s))
	require.NoError(t, s.ProcessLive(ctx))
	require.Equal(t, 3, count(t, s, "listens"))
}

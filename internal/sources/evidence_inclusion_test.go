package sources

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestEvidenceInclusionKeepsObservedAndRetainsInferredListening(t *testing.T) {
	s := testStore(t)
	track := libraryFixtureTrack("A", 7)
	track["Play Date UTC"] = "2020-01-03T00:00:00Z"
	importLibrarySnapshot(t, s, "ONE", "2021-01-01T00:00:00Z", []map[string]any{track})
	importLastfm(t, s, "["+lf("Song", "Album", time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC).Unix())+"]")
	rules := append(countRules(), Rule{ID: "dates", Type: "inference", Subtype: "time", Enabled: true, Scope: RuleScope{Service: "itunes"}, Settings: RuleSettings{UseLastPlayed: true, EstimateTimes: true}})
	rules = append(rules, Rule{ID: "observed", Type: "inclusion", Subtype: "evidence", Enabled: true, Program: assignWhen(operation("ne", fieldExpr("play.evidence_level"), literalExpr("observed")), "view.include", literalExpr(false))})
	saveTestRules(t, s, rules...)
	var visible, stored int
	require.NoError(t, s.DB.QueryRow(`SELECT COUNT(*) FROM displayed_listens`).Scan(&visible))
	require.NoError(t, s.DB.QueryRow(`SELECT COUNT(*) FROM listens`).Scan(&stored))
	require.Equal(t, 1, visible)
	require.Equal(t, 7, stored)
	original := count(t, s, "source_records")
	change := currentChange(t, s)
	testRule(t, &change.Config, "observed").Enabled = false
	_, err := s.Save(context.Background(), change)
	require.NoError(t, err)
	require.NoError(t, s.DB.QueryRow(`SELECT COUNT(*) FROM displayed_listens`).Scan(&visible))
	require.Equal(t, 7, visible)
	require.Equal(t, original, count(t, s, "source_records"))
}

func TestEvidenceInclusionAppliesToUndatedCounts(t *testing.T) {
	s := testStore(t)
	importLibrarySnapshot(t, s, "ONE", "2021-01-01T00:00:00Z", []map[string]any{libraryFixtureTrack("A", 7)})
	importLastfm(t, s, "["+lf("Song", "Album", time.Date(2010, 2, 1, 0, 0, 0, 0, time.UTC).Unix())+"]")
	rules := append(countRules(), Rule{ID: "observed", Type: "inclusion", Subtype: "evidence", Enabled: true, Program: assignWhen(operation("ne", fieldExpr("play.evidence_level"), literalExpr("observed")), "view.include", literalExpr(false))})
	saveTestRules(t, s, rules...)
	for _, view := range []string{"listening_counts", "album_report_counts"} {
		var total int
		require.NoError(t, s.DB.QueryRow(`SELECT COALESCE(SUM(amount),0) FROM `+view).Scan(&total))
		require.Equal(t, 1, total, view)
	}
	var rawUndated int
	require.NoError(t, s.DB.QueryRow(`SELECT SUM(undated_count) FROM source_aggregates`).Scan(&rawUndated))
	require.Equal(t, 6, rawUndated)
	change := currentChange(t, s)
	testRule(t, &change.Config, "observed").Enabled = false
	_, err := s.Save(context.Background(), change)
	require.NoError(t, err)
	var total int
	require.NoError(t, s.DB.QueryRow(`SELECT SUM(amount) FROM listening_counts`).Scan(&total))
	require.Equal(t, 7, total)
}

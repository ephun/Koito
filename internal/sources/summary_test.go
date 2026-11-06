package sources

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSummaryCountsDistinctSourceSupportAndLegacyFallback(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	mustImport(t, s, []byte("["+complete+","+complete+"]"), "personal.json", "")
	mustImport(t, s, []byte("["+strings.Replace(complete, "2020-01-02", "2020-01-03", 1)+"]"), "family.json", "family")
	importLastfm(t, s, "["+lf("Song", "Album", 1577934065)+"]")
	var family int64
	require.NoError(t, s.DB.QueryRow(`SELECT id FROM source_accounts WHERE account='family'`).Scan(&family))
	saveTestRules(t, s, Rule{ID: "family", Type: "attribution", Subtype: "direct", Enabled: true, Scope: RuleScope{SourceID: family}, Settings: RuleSettings{Listener: "unknown"}})
	_, err := s.DB.Exec(`INSERT INTO source_aggregates(aggregate_key,library_key,track_key,representative_record,track_id,user_id,available_count,attributed_count,observed_count,overlap_count,last_played_count,estimated_count,undated_count,details_json,version_id) VALUES('summary','library','track',1,NULL,1,2,2,0,0,0,0,2,'{}',(SELECT version_id FROM filter_state WHERE id=1))`)
	require.NoError(t, err)
	before, err := s.Summary(ctx)
	require.NoError(t, err)
	require.Len(t, before, 3)
	require.Equal(t, 2, before[0].Records)
	require.Equal(t, 1, before[0].Plays, "two duplicate records support one play")
	require.EqualValues(t, 2, before[0].Undated)
	require.Equal(t, 1, before[1].Records)
	require.Zero(t, before[1].Plays)
	require.Empty(t, before[1].First)
	require.Empty(t, before[1].Last)
	require.Equal(t, 1, before[2].Plays, "Last.fm supports the same play")
	require.Equal(t, before[0].First, before[2].First)
	require.NotEmpty(t, before[0].First)

	// A partially migrated play must retain support from both linked and
	// interpretation-only sources. An entirely old projection works too.
	for _, deletion := range []string{
		`DELETE FROM source_play_records WHERE record_id IN (SELECT id FROM source_records WHERE parser LIKE 'lastfm-%')`,
		`DELETE FROM source_play_records`,
	} {
		_, err = s.DB.Exec(deletion)
		require.NoError(t, err)
		after, err := s.Summary(ctx)
		require.NoError(t, err)
		require.Equal(t, before, after)
	}
}

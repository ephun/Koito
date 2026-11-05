package sources

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// Opt-in acceptance never imports private files into the running application.
func TestLocalPhase3Libraries(t *testing.T) {
	root := os.Getenv("KOITO_TEST_ITUNES_DIR")
	if root == "" {
		t.Skip("set KOITO_TEST_ITUNES_DIR to the private library directory")
	}
	s := testStore(t)
	ctx := context.Background()
	for _, input := range []struct {
		name    string
		records int
	}{
		{"familyITunes_library.xml", 14265},
		{"familyITunes_library.JSON", 14020},
		{"familyITunes_plays.json", 56210},
	} {
		data, err := os.ReadFile(filepath.Join(root, input.name))
		require.NoError(t, err)
		result, err := s.Import(ctx, bytes.NewReader(data), input.name, 1)
		require.NoError(t, err)
		require.Equal(t, input.records, result.Added)
		repeated, err := s.Import(ctx, bytes.NewReader(data), "renamed.data", 1)
		require.NoError(t, err)
		require.Zero(t, repeated.Added)
		require.Equal(t, result.Records, repeated.Reused)
		path, err := s.FilePath(result.Hash)
		require.NoError(t, err)
		stored, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, digest(data), digest(stored))
		t.Logf("%s: %d records, repeated import reused all records", input.name, result.Added)
	}
	for _, parser := range []string{ITunesXML, ITunesJSON} {
		var tracks, total int
		err := s.DB.QueryRow(`SELECT COUNT(*),SUM(COALESCE(json_extract(payload,'$.fields."Play Count"[0]'),json_extract(payload,'$.fields."Play Count"'),0)) FROM source_records WHERE parser=? AND kind='library_track'`, parser).Scan(&tracks, &total)
		require.NoError(t, err)
		require.Equal(t, 14020, tracks)
		require.Equal(t, 47748, total)
	}
	libraries, err := s.Libraries(ctx)
	require.NoError(t, err)
	require.Len(t, libraries, 1)
	require.Len(t, libraries[0].Files, 2)
	require.Len(t, libraries[0].Tracks, 14020)
	require.EqualValues(t, 47748, libraries[0].Reported)
	equivalent := 0
	for _, file := range libraries[0].Files {
		if file.Equivalent {
			equivalent++
		}
	}
	require.Equal(t, 1, equivalent)
	t.Logf("XML and JSON reconcile to %d tracks and %d reported cumulative plays", len(libraries[0].Tracks), libraries[0].Reported)
	for i := 0; i < 2; i++ {
		result, err := s.Rebuild(ctx)
		require.NoError(t, err)
		require.Equal(t, 84495, result.Records)
		require.Zero(t, result.Plays)
		require.Zero(t, count(t, s, "listens"))
	}
	fingerprint := func(query string) string {
		rows, err := s.DB.Query(query)
		require.NoError(t, err)
		defer rows.Close()
		h := sha256.New()
		for rows.Next() {
			var text string
			require.NoError(t, rows.Scan(&text))
			fmt.Fprintf(h, "%d:%s", len(text), text)
		}
		require.NoError(t, rows.Err())
		return hex.EncodeToString(h.Sum(nil))
	}
	rawQuery := `SELECT json_array(id,source_id,file_hash,ordinal,kind,payload,parser) FROM source_records ORDER BY id`
	before := fingerprint(rawQuery)
	saveTestRules(t, s, countRules()[0])
	var available, attributed int64
	require.NoError(t, s.DB.QueryRow(`SELECT COALESCE(SUM(available_count),0),COALESCE(SUM(attributed_count),0) FROM source_aggregates`).Scan(&available, &attributed))
	require.EqualValues(t, 47748, available)
	require.Zero(t, attributed)
	// Explicit acceptance settings exercise mechanics. They are not saved as
	// personal attribution defaults in the application.
	saveTestRules(t, s, countRules()[1], Rule{ID: "test-time", Type: "inference", Subtype: "time", Enabled: true, Scope: RuleScope{Service: "itunes"}, Settings: RuleSettings{UseLastPlayed: true, EstimateTimes: true}})
	var last, estimated, undated, overlap int64
	require.NoError(t, s.DB.QueryRow(`SELECT SUM(available_count),SUM(attributed_count),SUM(last_played_count),SUM(estimated_count),SUM(undated_count),SUM(overlap_count) FROM source_aggregates`).Scan(&available, &attributed, &last, &estimated, &undated, &overlap))
	require.EqualValues(t, 47748, available)
	require.LessOrEqual(t, attributed, available)
	require.Greater(t, estimated, int64(0))
	require.EqualValues(t, attributed-overlap, last+estimated+undated)
	require.EqualValues(t, last+estimated, count(t, s, "listens"))
	var invalidTimes int
	require.NoError(t, s.DB.QueryRow(`SELECT COUNT(*) FROM source_plays p JOIN source_play_details d ON d.play_key=p.play_key WHERE d.evidence_level='estimated' AND (p.listened_at<CAST(strftime('%s',d.interval_start) AS INTEGER) OR p.listened_at>CAST(strftime('%s',d.interval_end) AS INTEGER))`).Scan(&invalidTimes))
	require.Zero(t, invalidTimes)
	viewQuery := `SELECT json_array(p.play_key,p.listened_at,d.evidence_level,d.interval_start,d.interval_end,d.attribution_json) FROM source_plays p JOIN source_play_details d ON d.play_key=p.play_key ORDER BY p.play_key`
	view := fingerprint(viewQuery)
	_, err = s.Rebuild(ctx)
	require.NoError(t, err)
	require.Equal(t, view, fingerprint(viewQuery))
	require.Equal(t, before, fingerprint(rawQuery))
	t.Logf("Explicit test attribution: available=%d attributed=%d last_played=%d estimated=%d undated=%d matched_endpoints=%d", available, attributed, last, estimated, undated, overlap)
	change := currentChange(t, s)
	change.Config.Rules[len(change.Config.Rules)-1].Enabled = false
	_, err = s.Save(ctx, change)
	require.NoError(t, err)
	require.Zero(t, count(t, s, "listens"))
	change = currentChange(t, s)
	testRule(t, &change.Config, "library-count").Enabled = false
	_, err = s.Save(ctx, change)
	require.NoError(t, err)
	require.Zero(t, count(t, s, "source_aggregates"))
	require.Equal(t, before, fingerprint(rawQuery))
}

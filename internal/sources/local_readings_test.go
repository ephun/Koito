package sources

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// Opt-in acceptance reads large private archives through the production reader.
// No private paths, payloads, or artifacts belong in the repository.
func TestLocalPhase6Readings(t *testing.T) {
	input := os.Getenv("KOITO_TEST_PHASE6_READINGS")
	if input == "" {
		t.Skip("set KOITO_TEST_PHASE6_READINGS to a JSON list of source paths")
	}
	var paths []string
	require.NoError(t, json.Unmarshal([]byte(input), &paths))
	s := testStore(t)
	ctx := context.Background()
	for _, path := range paths {
		func() {
			stream, err := os.Open(path)
			require.NoError(t, err)
			file, err := s.SaveFile(ctx, stream)
			stream.Close()
			require.NoError(t, err)
			parts, err := s.readUpload(ctx, file, filepath.Base(path))
			require.NoError(t, err)
			defer cleanupParsedFiles(parts)
			for _, part := range parts {
				reasons := map[string]int{}
				kinds := map[string]int{}
				records := 0
				err = part.eachRecord(func(ordinal int, raw json.RawMessage) error {
					records++
					v := normalizeRecord(raw, part.parser, "UTC")
					reasons[v.Reason]++
					var row map[string]any
					if err := json.Unmarshal(raw, &row); err != nil {
						return err
					}
					kinds[valueText(row["kind"])]++
					if records%250000 == 0 {
						t.Logf("%s: checked %d records", filepath.Base(path), records)
					}
					return nil
				})
				require.NoError(t, err)
				require.Equal(t, part.countRecords(), records)
				t.Logf("%s (%s): %d records, kinds %v, interpretations %v", filepath.Base(path), part.parser, records, kinds, reasons)
			}
			stored, _ := s.FilePath(file.Hash)
			copy, err := os.Open(stored)
			require.NoError(t, err)
			hash := sha256.New()
			_, err = io.Copy(hash, copy)
			copy.Close()
			require.NoError(t, err)
			require.Equal(t, file.Hash, hex.EncodeToString(hash.Sum(nil)), "reader preserved original bytes")
		}()
	}
}

func TestLocalPhase6NavidromeCounts(t *testing.T) {
	path := os.Getenv("KOITO_TEST_PHASE6_NAVIDROME")
	if path == "" {
		t.Skip("set KOITO_TEST_PHASE6_NAVIDROME to the actual Navidrome snapshot")
	}
	s := testStore(t)
	ctx := context.Background()
	input, err := os.Open(path)
	require.NoError(t, err)
	_, err = s.Import(ctx, input, filepath.Base(path), 1)
	input.Close()
	require.NoError(t, err)
	before, err := s.Rebuild(ctx)
	require.NoError(t, err)
	var reported int64
	require.NoError(t, s.DB.QueryRow(`SELECT SUM(json_extract(payload,'$.fields.play_count')) FROM source_records WHERE kind='library_track'`).Scan(&reported))
	saveTestRules(t, s, Rule{ID: "navidrome-count", Type: "inference", Subtype: "count", Enabled: true, Program: countProgram("maximum", RuleScope{Service: "navidrome"})})
	var available, observed, undated int64
	require.NoError(t, s.DB.QueryRow(`SELECT SUM(available_count),SUM(observed_count),SUM(undated_count) FROM source_aggregates`).Scan(&available, &observed, &undated))
	var repeats int64
	require.NoError(t, s.DB.QueryRow(`SELECT COALESCE(SUM(n-1),0) FROM (SELECT COUNT(*) n FROM source_records WHERE kind='play' GROUP BY json_extract(payload,'$.fields.user_id'),json_extract(payload,'$.fields.media_file_id'),json_extract(payload,'$.fields.submission_time'))`).Scan(&repeats))
	require.Equal(t, reported-repeats, available)
	require.EqualValues(t, before.Plays, observed)
	require.Equal(t, reported-repeats-observed, undated)
	require.Equal(t, before.Plays, count(t, s, "listens"))
	t.Logf("Actual Navidrome: %d reported increments minus %d matched repeats reconcile with %d observed plays and %d undated plays", reported, repeats, observed, undated)
}

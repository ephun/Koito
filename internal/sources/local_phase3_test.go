package sources

import (
	"bytes"
	"context"
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
}

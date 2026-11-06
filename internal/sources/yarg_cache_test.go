package sources

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLocalPhase7YARGCache(t *testing.T) {
	root := os.Getenv("KOITO_TEST_PHASE7_YARG_ROOT")
	if root == "" {
		t.Skip("set KOITO_TEST_PHASE7_YARG_ROOT to the YARG export directory")
	}
	s := testStore(t)
	ctx := context.Background()
	for _, edition := range []string{"release", "nightly"} {
		data, err := os.ReadFile(filepath.Join(root, edition, "songcache.bin"))
		require.NoError(t, err)
		first, err := s.Import(ctx, bytes.NewReader(data), "renamed.data", 1)
		require.NoError(t, err)
		again, err := s.Import(ctx, bytes.NewReader(data), "copy.bin", 1)
		require.NoError(t, err)
		require.Zero(t, again.Added)
		require.Equal(t, first.Records, again.Reused)
		t.Logf("%s cache: %d records; repeated import adds zero", edition, first.Records)
		_, err = s.Import(ctx, bytes.NewReader(data[:len(data)-1]), "truncated.bin", 1)
		require.Error(t, err)
	}
	result, err := s.Rebuild(ctx)
	require.NoError(t, err)
	require.Zero(t, result.Plays)
}

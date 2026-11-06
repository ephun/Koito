package sources

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPublicationRestoresConnectionCacheOnCommitAndRollback(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	mustImport(t, s, []byte("["+complete+"]"), "listening.json", "")
	_, err := s.DB.Exec(`PRAGMA cache_size=-256`)
	require.NoError(t, err)
	for _, fail := range []bool{false, true} {
		if fail {
			_, err = s.DB.Exec(`CREATE TRIGGER reject_publication BEFORE INSERT ON source_plays BEGIN SELECT RAISE(ABORT,'publication failure'); END`)
			require.NoError(t, err)
		}
		_, err = s.Rebuild(ctx)
		if fail {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
		}
		var cache int
		require.NoError(t, s.DB.QueryRow(`PRAGMA cache_size`).Scan(&cache))
		require.Equal(t, -256, cache)
		require.Equal(t, 1, count(t, s, "listens"))
		require.Equal(t, 1, count(t, s, "source_records"))
	}
}

package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/gabehf/koito/internal/db"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestExportUsesDisplayedListening(t *testing.T) {
	s := musicStore(t)
	ctx := context.Background()
	track := musicAppearance(t, s, "Song", 1, uuid.Nil)
	for _, timestamp := range []int64{1577934065, 1577934125} {
		require.NoError(t, s.SaveListen(ctx, db.SaveListenOpts{TrackID: track.ID, Time: time.Unix(timestamp, 0), UserID: 1}))
	}
	require.NoError(t, s.Exec(`INSERT INTO listen_scope(track_id,listened_at,play_key,included) SELECT track_id,listened_at,play_key,0 FROM listens WHERE listened_at=1577934065`))
	page, err := s.GetExportPage(ctx, db.GetExportPageOpts{UserID: 1, ListenedAt: time.Unix(0, 0), Limit: 100})
	require.NoError(t, err)
	require.Len(t, page, 1)
	require.EqualValues(t, 1577934125, page[0].ListenedAt.Unix())
	total, err := s.Count(`SELECT COUNT(*) FROM listens`)
	require.NoError(t, err)
	require.Equal(t, 2, total)
}

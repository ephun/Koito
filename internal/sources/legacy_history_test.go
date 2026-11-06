package sources

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLegacyHistoryImportsPreserveAndMatch(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	inputs := []struct{ name, body string }{
		{"listenbrainz.jsonl", `{"listened_at":1577934245,"track_metadata":{"artist_name":"Artist","track_name":"Song","release_name":"Album","additional_info":{"unknown":{"keep":true}}}}` + "\n" + `{"listened_at":1577937845,"track_metadata":{"artist_name":"Artist","track_name":"Other","release_name":"Album"}}`},
		{"maloja.json", `{"scrobbles":[{"time":1577934245,"track":{"artists":["Artist"],"title":"Song","album":{"albumtitle":"Album"}},"extra":"kept"}]}`},
		{"koito.json", `{"version":"1","exported_at":"2026-01-01T00:00:00Z","user":"test","listens":[{"listened_at":"2020-01-02T03:04:05Z","client":"lastfm","track":{"aliases":[{"alias":"Song","is_primary":true}]},"album":{"aliases":[{"alias":"Album","is_primary":true}]},"artists":[{"is_primary":true,"aliases":[{"alias":"Artist","is_primary":true}]}],"extra":"kept"}]}`},
	}
	for _, input := range inputs {
		first, err := s.Import(ctx, bytes.NewBufferString(input.body), input.name, 1)
		require.NoError(t, err)
		again, err := s.Import(ctx, bytes.NewBufferString(input.body), input.name, 1)
		require.NoError(t, err)
		require.Zero(t, again.Added)
		require.Equal(t, first.Records, again.Reused)
	}
	result, err := s.Rebuild(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, result.Plays)
	require.Equal(t, 4, count(t, s, "source_records"))
	require.Equal(t, 4, count(t, s, "source_play_records"))
	_, err = s.Import(ctx, bytes.NewBufferString(`{"version":"99","listens":[]}`), "future.json", 1)
	require.Error(t, err)
}

func TestLegacyHistoryRetainsUnidentifiedEntries(t *testing.T) {
	for _, input := range []struct{ name, body string }{
		{"maloja.json", `{"scrobbles":[{"time":1577934245,"track":{"artists":["Artist"],"title":"Song"}},{"unknown":{"keep":true}}]}`},
		{"listens.jsonl", `{"listened_at":1577934245,"track_metadata":{"artist_name":"Artist","track_name":"Song"}}` + "\n" + `{"unknown":{"keep":true}}`},
	} {
		t.Run(input.name, func(t *testing.T) {
			s := testStore(t)
			result, err := s.Import(context.Background(), bytes.NewBufferString(input.body), input.name, 1)
			require.NoError(t, err)
			require.Equal(t, 2, result.Records)
			view, err := s.Rebuild(context.Background())
			require.NoError(t, err)
			require.Equal(t, 1, view.Plays)
			var payload string
			require.NoError(t, s.DB.QueryRow(`SELECT payload FROM source_records ORDER BY id DESC LIMIT 1`).Scan(&payload))
			require.Contains(t, payload, `"unknown":{"keep":true}`)
		})
	}
}

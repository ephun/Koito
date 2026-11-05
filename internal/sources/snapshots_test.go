package sources

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSnapshotPreservationAndReuse(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	playlist := `{"playlists":[{"name":"Repeated","lastModifiedDate":"2020-01-01","items":[{"track":{"trackName":"Song","artistName":"Artist","albumName":"Album","trackUri":"spotify:track:one"},"addedDate":"2019-01-01"},{"track":{"trackName":"Song","artistName":"Artist","albumName":"Album","trackUri":"spotify:track:one"},"addedDate":"2019-01-02"}]}]}`
	for i := 0; i < 3; i++ {
		r, err := s.Import(ctx, bytes.NewBufferString(playlist), "Playlist1.json", 1)
		require.NoError(t, err)
		require.Equal(t, 2, r.Records)
		if i > 0 {
			require.Zero(t, r.Added)
		}
	}
	require.Equal(t, 2, count(t, s, "source_snapshots"))
	require.Equal(t, 2, count(t, s, "source_snapshot_items"))
	result, err := s.Rebuild(ctx)
	require.NoError(t, err)
	require.Zero(t, result.Plays)
	collection := "Catalog#,Artist,Title,Label,Format,Rating,Released,release_id,CollectionFolder,Date Added,Collection Media Condition,Collection Sleeve Condition,Collection Notes\nABC,Artist,Album,Label,LP,5,0,100,All,2020-01-01,Mint,Mint,Copy\nABC,Artist,Album,Label,LP,5,0,100,All,2020-01-01,Mint,Mint,Copy\n"
	r, err := s.Import(ctx, bytes.NewBufferString(collection), "collection.csv", 1)
	require.NoError(t, err)
	require.Equal(t, 2, r.Added)
	wantlist := "Catalog#,Artist,Title,Label,Format,Rating,Released,release_id,Notes\nABC,Artist,Album,Label,LP,5,0,100,Note,2020-01-02\n"
	_, err = s.Import(ctx, bytes.NewBufferString(wantlist), "wantlist.csv", 1)
	require.NoError(t, err)
	var payload string
	require.NoError(t, s.DB.QueryRow(`SELECT payload FROM source_records WHERE kind='wanted'`).Scan(&payload))
	var row csvRecord
	require.NoError(t, json.Unmarshal([]byte(payload), &row))
	require.Len(t, row.Columns, 9)
	require.Len(t, row.Values, 10)
	require.Equal(t, "2020-01-02", row.Values[9])
	header := "listing_id,artist,title,label,catno,format,release_id,status,price,listed,comments,media_condition,sleeve_condition,accept_offer,external_id,weight,format_quantity,location,quantity\n"
	r, err = s.Import(ctx, bytes.NewBufferString(header), "inventory.csv", 1)
	require.NoError(t, err)
	require.Zero(t, r.Added)
	require.Equal(t, 5, count(t, s, "source_snapshots"))
	require.Equal(t, 5, count(t, s, "source_records"))
	_, err = s.DB.Exec(`UPDATE source_snapshots SET name='Changed'`)
	require.Error(t, err)
	_, err = s.DB.Exec(`DELETE FROM source_snapshot_items`)
	require.Error(t, err)
}

func TestIdenticalPlaylistCSVFilesKeepDistinctIdentity(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	data := "Track URI,Track Name,Artist Name(s),Album Name,Added At\nspotify:track:one,Song,Artist,Album,2020-01-01\n"
	for _, name := range []string{"First.csv", "Second.csv", "First.csv"} {
		_, err := s.Import(ctx, bytes.NewBufferString(data), name, 1)
		require.NoError(t, err)
	}
	require.Equal(t, 2, count(t, s, "source_snapshots"))
	require.Equal(t, 2, count(t, s, "source_records"))
}

func TestPlaylistRemovalAndDuplicateNames(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	data := `{"playlists":[{"name":"Same","items":[{"track":{"trackName":"Song","artistName":"Artist","trackUri":"spotify:track:one"}}]},{"name":"Same","items":[{"track":{"trackName":"Other","artistName":"Artist","trackUri":"spotify:track:two"}}]}]}`
	_, err := s.Import(ctx, bytes.NewBufferString(data), "Playlist1.json", 1)
	require.NoError(t, err)
	_, err = s.Rebuild(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, count(t, s, "source_music_states"))
	_, err = s.Import(ctx, bytes.NewBufferString(`{"playlists":[]}`), "Playlist1.json", 1)
	require.NoError(t, err)
	_, err = s.Import(ctx, bytes.NewBufferString(data), "Playlist1.json", 1)
	require.NoError(t, err)
	result, err := s.Rebuild(ctx)
	require.NoError(t, err)
	require.Zero(t, result.Plays)
	require.Zero(t, count(t, s, "source_music_states"))
	require.Equal(t, 2, count(t, s, "source_records"))
}

func TestSpotifyContextAndEmptyHistory(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	for _, input := range []struct{ name, data string }{{"Spotify Technical Log Information/AudioRouteSegmentEnd.json", `[{"platform":"test","duration":5}]`}, {"PlaylistInABottle.json", `{"context":true}`}, {"StreamingHistory_music_0.json", `[]`}} {
		_, err := s.Import(ctx, bytes.NewBufferString(input.data), input.name, 1)
		require.NoError(t, err)
	}
	require.Equal(t, 2, count(t, s, "source_records"))
	var parser string
	require.NoError(t, s.DB.QueryRow(`SELECT parser FROM source_extractions WHERE record_count=0`).Scan(&parser))
	require.Equal(t, SpotifyStandard, parser)
	result, err := s.Rebuild(ctx)
	require.NoError(t, err)
	require.Zero(t, result.Plays)
}

// Private acceptance imports actual dump files without copying their contents
// into public fixtures. Repeats must reuse all records and snapshots.
func TestLocalPhase5Sources(t *testing.T) {
	pathsJSON := os.Getenv("KOITO_TEST_PHASE5_FILES")
	if pathsJSON == "" {
		t.Skip("set KOITO_TEST_PHASE5_FILES to a JSON array of source paths")
	}
	var paths []string
	require.NoError(t, json.Unmarshal([]byte(pathsJSON), &paths))
	s := testStore(t)
	ctx := context.Background()
	for _, path := range paths {
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		first, err := s.Import(ctx, bytes.NewReader(data), filepath.Base(path), 1)
		require.NoError(t, err)
		before := count(t, s, "source_snapshots")
		second, err := s.Import(ctx, bytes.NewReader(data), filepath.Base(path), 1)
		require.NoError(t, err)
		require.Zero(t, second.Added)
		require.Equal(t, first.Records, second.Reused)
		require.Equal(t, before, count(t, s, "source_snapshots"))
		stored, _ := s.FilePath(first.Hash)
		original, err := os.ReadFile(stored)
		require.NoError(t, err)
		require.True(t, bytes.Equal(data, original))
		t.Logf("%s: records=%d, snapshots=%d", filepath.Base(path), first.Records, before)
	}
}

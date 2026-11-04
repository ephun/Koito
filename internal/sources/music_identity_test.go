package sources

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestImportedAlbumAppearancesKeepOneHistoryAndTheirOwnPlays(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	second := strings.ReplaceAll(strings.ReplaceAll(complete, "2020-01-02T03:04:05Z", "2020-01-03T03:04:05Z"), `"Album"`, `"Compilation"`)
	mustImport(t, s, []byte("["+complete+","+second+"]"), "spotify.json", "")
	for repeat := 0; repeat < 2; repeat++ {
		result, err := s.Rebuild(ctx)
		require.NoError(t, err)
		require.Equal(t, 2, result.Plays)
		var appearances, histories, recordings int
		err = s.DB.QueryRow(`SELECT COUNT(DISTINCT p.track_id),COUNT(DISTINCT h.music_track_id),COUNT(DISTINCT r.recording_key)
		    FROM source_plays p JOIN track_histories h ON h.track_id=p.track_id JOIN track_recordings r ON r.track_id=p.track_id`).Scan(&appearances, &histories, &recordings)
		require.NoError(t, err)
		require.Equal(t, 2, appearances)
		require.Equal(t, 1, histories)
		require.Equal(t, 1, recordings)
		var correct int
		err = s.DB.QueryRow(`SELECT COUNT(*) FROM source_plays p JOIN source_records r ON r.id=p.representative_record
		    JOIN tracks t ON t.id=p.track_id JOIN releases_with_title a ON a.id=t.release_id
		    WHERE a.title=json_extract(r.payload,'$.master_metadata_album_album_name')`).Scan(&correct)
		require.NoError(t, err)
		require.Equal(t, 2, correct)
		require.Equal(t, 2, count(t, s, "source_records"))
		require.Equal(t, 2, count(t, s, "source_play_records"))
	}
}

func TestSavedCaseFilterRegroupsHistoryAndRestoresOriginals(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	second := strings.ReplaceAll(strings.ReplaceAll(complete, "2020-01-02T03:04:05Z", "2020-01-03T03:04:05Z"), `"Song"`, `"song"`)
	mustImport(t, s, []byte("["+complete+","+second+"]"), "spotify.json", "")
	for _, sensitive := range []bool{true, false, true} {
		change := currentChange(t, s)
		change.Config.Rules[0].Settings.CaseSensitive = sensitive
		_, err := s.Save(ctx, change)
		require.NoError(t, err)
		var histories int
		err = s.DB.QueryRow(`SELECT COUNT(DISTINCT h.music_track_id) FROM source_plays p JOIN track_histories h ON h.track_id=p.track_id`).Scan(&histories)
		require.NoError(t, err)
		want := 2
		if !sensitive {
			want = 1
		}
		require.Equal(t, want, histories)
		require.Equal(t, 2, count(t, s, "listens"))
	}
	var original, other string
	require.NoError(t, s.DB.QueryRow(`SELECT payload FROM source_records WHERE ordinal=0`).Scan(&original))
	require.NoError(t, s.DB.QueryRow(`SELECT payload FROM source_records WHERE ordinal=1`).Scan(&other))
	require.JSONEq(t, complete, original)
	require.JSONEq(t, second, other)
}

func TestConflictingImportedRecordingsRetainSeparateAppearances(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	second := strings.ReplaceAll(strings.ReplaceAll(complete, "2020-01-02T03:04:05Z", "2020-01-03T03:04:05Z"), "spotify:track:one", "spotify:track:two")
	mustImport(t, s, []byte("["+complete+","+second+"]"), "spotify.json", "")
	_, err := s.Rebuild(ctx)
	require.NoError(t, err)
	var identities int
	err = s.DB.QueryRow(`SELECT COUNT(DISTINCT tr.recording_key) FROM source_plays p JOIN track_recordings tr ON tr.track_id=p.track_id`).Scan(&identities)
	require.NoError(t, err)
	require.Equal(t, 2, identities)
	var exact int
	err = s.DB.QueryRow(`SELECT COUNT(*) FROM source_plays p JOIN source_records r ON r.id=p.representative_record
	    JOIN track_recordings tr ON tr.track_id=p.track_id JOIN recordings rec ON rec.recording_key=tr.recording_key
	    WHERE rec.spotify_uri=json_extract(r.payload,'$.spotify_track_uri')`).Scan(&exact)
	require.NoError(t, err)
	require.Equal(t, 2, exact)
}

package sources

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func importLibrarySnapshot(t *testing.T, s *Store, library, date string, tracks []map[string]any) {
	t.Helper()
	byID := map[string]any{}
	for i, track := range tracks {
		byID[fmt.Sprint(i+1)] = track
	}
	raw, err := json.Marshal(map[string]any{"Library Persistent ID": library, "Date": date, "Tracks": byID})
	require.NoError(t, err)
	_, err = s.Import(context.Background(), bytes.NewReader(raw), "library.json", 1)
	require.NoError(t, err)
}

func libraryFixtureTrack(id string, count any) map[string]any {
	track := map[string]any{"Persistent ID": id, "Track ID": 1, "Name": "Song", "Artist": "Artist", "Album": "Album", "Date Added": "2010-01-01T00:00:00Z", "Play Date UTC": "2011-01-01T00:00:00Z"}
	if count != nil {
		track["Play Count"] = count
	}
	return track
}

func TestLibrarySnapshotsAndDecreases(t *testing.T) {
	s := testStore(t)
	importLibrarySnapshot(t, s, "ONE", "2012-01-01T00:00:00Z", []map[string]any{libraryFixtureTrack("A", 7)})
	importLibrarySnapshot(t, s, "ONE", "2013-01-01T00:00:00Z", []map[string]any{libraryFixtureTrack("A", 10)})
	libraries, err := s.Libraries(context.Background())
	require.NoError(t, err)
	require.Len(t, libraries, 1)
	require.EqualValues(t, 10, libraries[0].Reported)
	require.Len(t, libraries[0].Tracks, 1)
	track := libraries[0].Tracks[0]
	require.EqualValues(t, 10, *track.Latest)
	require.Empty(t, track.Issues)
	importLibrarySnapshot(t, s, "ONE", "2014-01-01T00:00:00Z", []map[string]any{libraryFixtureTrack("A", 2)})
	libraries, err = s.Libraries(context.Background())
	require.NoError(t, err)
	track = libraries[0].Tracks[0]
	require.EqualValues(t, 10, libraries[0].Reported)
	require.EqualValues(t, 2, *track.Latest)
	require.Contains(t, track.Issues, "count decreased")
	require.Len(t, track.Snapshots, 3)
}

func TestLibraryIdentityAndExactConvertedCopies(t *testing.T) {
	s := testStore(t)
	fields := libraryFixtureTrack("A", 7)
	importLibrarySnapshot(t, s, "ONE", "2012-01-01T00:00:00Z", []map[string]any{fields})
	converted := map[string]any{}
	for key, value := range fields {
		converted[key] = []string{fmt.Sprint(value)}
	}
	raw, err := json.Marshal([]any{converted})
	require.NoError(t, err)
	_, err = s.Import(context.Background(), bytes.NewReader(raw), "converted.json", 1)
	require.NoError(t, err)
	libraries, err := s.Libraries(context.Background())
	require.NoError(t, err)
	require.Len(t, libraries, 1)
	require.Len(t, libraries[0].Files, 2)
	require.EqualValues(t, 7, libraries[0].Reported)
	require.Len(t, libraries[0].Tracks[0].Snapshots, 2)
	equivalent := 0
	for _, file := range libraries[0].Files {
		if file.Equivalent {
			equivalent++
		}
	}
	require.Equal(t, 1, equivalent)
	// Same persistent track ID in another library is independent evidence.
	importLibrarySnapshot(t, s, "TWO", "2012-01-01T00:00:00Z", []map[string]any{libraryFixtureTrack("A", 9)})
	libraries, err = s.Libraries(context.Background())
	require.NoError(t, err)
	require.Len(t, libraries, 2)
	// An exact conversion shared by two libraries cannot establish ownership.
	importLibrarySnapshot(t, s, "TWO", "2013-01-01T00:00:00Z", []map[string]any{fields})
	libraries, err = s.Libraries(context.Background())
	require.NoError(t, err)
	require.Len(t, libraries, 3)
	unknown := 0
	for _, library := range libraries {
		if library.LibraryID == "" {
			unknown++
			require.Contains(t, library.Files[0].Issues, "ambiguous library identity")
		}
	}
	require.Equal(t, 1, unknown)
}

func TestLibraryCountAndDateValidity(t *testing.T) {
	s := testStore(t)
	invalid := libraryFixtureTrack("NEGATIVE", -1)
	invalid["Play Date UTC"] = "1904-01-01T00:00:00Z"
	reversed := libraryFixtureTrack("REVERSED", 2)
	reversed["Date Added"] = "2012-01-01T00:00:00Z"
	future := libraryFixtureTrack("FUTURE", 3)
	future["Play Date UTC"] = "2014-01-01T00:00:00Z"
	zero := libraryFixtureTrack("ZERO", 0)
	importLibrarySnapshot(t, s, "ONE", "2013-01-01T00:00:00Z", []map[string]any{invalid, reversed, future, zero, libraryFixtureTrack("UNKNOWN", nil), libraryFixtureTrack("FRACTION", 1.5), libraryFixtureTrack("OVERFLOW", "9223372036854775808")})
	libraries, err := s.Libraries(context.Background())
	require.NoError(t, err)
	library := libraries[0]
	require.EqualValues(t, 5, library.Reported)
	require.Equal(t, 3, library.Invalid)
	require.Equal(t, 1, library.Unknown)
	tracks := map[string]LibraryTrack{}
	for _, track := range library.Tracks {
		tracks[track.PersistentID] = track
	}
	require.Contains(t, tracks["NEGATIVE"].Issues, "invalid play count")
	require.Contains(t, tracks["NEGATIVE"].Issues, "invalid last played")
	require.Contains(t, tracks["REVERSED"].Issues, "last played precedes date added")
	require.Contains(t, tracks["FUTURE"].Issues, "track date exceeds snapshot")
	require.Contains(t, tracks["ZERO"].Issues, "last played with zero count")
	require.NotNil(t, tracks["ZERO"].Reported)
	require.Nil(t, tracks["UNKNOWN"].Reported)
	result, err := s.Rebuild(context.Background())
	require.NoError(t, err)
	require.Zero(t, result.Plays)
}

func TestLibrarySnapshotConflictAndUnknownOrder(t *testing.T) {
	s := testStore(t)
	for _, count := range []int{7, 10} {
		importLibrarySnapshot(t, s, "ONE", "2012-01-01T00:00:00Z", []map[string]any{libraryFixtureTrack("A", count)})
	}
	libraries, err := s.Libraries(context.Background())
	require.NoError(t, err)
	track := libraries[0].Tracks[0]
	require.Contains(t, track.Issues, "conflicting counts at one snapshot")
	require.Nil(t, track.Latest)
	importLibrarySnapshot(t, s, "ONE", "", []map[string]any{libraryFixtureTrack("A", 20)})
	libraries, err = s.Libraries(context.Background())
	require.NoError(t, err)
	require.EqualValues(t, 20, libraries[0].Reported)
	require.Contains(t, libraries[0].Tracks[0].Issues, "unknown snapshot order")
}

func TestLibraryRepeatedPersistentIdentity(t *testing.T) {
	s := testStore(t)
	importLibrarySnapshot(t, s, "ONE", "2012-01-01T00:00:00Z", []map[string]any{libraryFixtureTrack("A", 7), libraryFixtureTrack("A", 9)})
	libraries, err := s.Libraries(context.Background())
	require.NoError(t, err)
	require.EqualValues(t, 9, libraries[0].Reported)
	require.Contains(t, libraries[0].Tracks[0].Issues, "repeated persistent identity")
	require.Len(t, libraries[0].Tracks[0].Snapshots, 2)
}

func TestLibraryNewestUnknownCountAndUploadOrder(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		s := testStore(t)
		counts := []any{7, nil}
		dates := []string{"2012-01-01T00:00:00Z", "2013-01-01T00:00:00Z"}
		order := []int{0, 1}
		if reverse {
			order = []int{1, 0}
		}
		for _, i := range order {
			importLibrarySnapshot(t, s, "ONE", dates[i], []map[string]any{libraryFixtureTrack("A", counts[i])})
		}
		libraries, err := s.Libraries(context.Background())
		require.NoError(t, err)
		require.EqualValues(t, 7, libraries[0].Reported)
		require.Nil(t, libraries[0].Tracks[0].Latest)
		require.Contains(t, libraries[0].Tracks[0].Issues, "latest snapshot has unknown count")
	}
}

func TestLibraryUnknownTrackIdentityKeepsAmountsUnresolved(t *testing.T) {
	s := testStore(t)
	track := libraryFixtureTrack("", 7)
	delete(track, "Persistent ID")
	for _, date := range []string{"2012-01-01T00:00:00Z", "2013-01-01T00:00:00Z"} {
		importLibrarySnapshot(t, s, "ONE", date, []map[string]any{track})
	}
	libraries, err := s.Libraries(context.Background())
	require.NoError(t, err)
	require.Zero(t, libraries[0].Reported)
	require.Equal(t, 2, libraries[0].Unidentified)
	for _, track := range libraries[0].Tracks {
		require.EqualValues(t, 7, *track.Reported)
		require.Contains(t, track.Issues, "unknown persistent identity")
	}
}

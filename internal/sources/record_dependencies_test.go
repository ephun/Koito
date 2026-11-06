package sources

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func dependencyInput(timestamp int64, event, device, title string) string {
	input := strings.ReplaceAll(liveExample, "1793937600", strconv.FormatInt(timestamp, 10))
	input = strings.Replace(input, `"unknown_nested":`, `"event_id":`+strconv.Quote(event)+`,"unknown_nested":`, 1)
	input = strings.Replace(input, `"Desktop"`, strconv.Quote(device), 1)
	return strings.Replace(input, `"Song"`, strconv.Quote(title), 1)
}

func unpublishedDependency(t *testing.T, s *Store, input string) (RuleConfig, []matchingRecord) {
	t.Helper()
	entry := captureExample(t, s, input, "")
	require.NoError(t, s.StoreLive(context.Background(), entry))
	change := currentChange(t, s)
	var maximum int64
	require.NoError(t, s.DB.QueryRow(`SELECT max_record_id FROM source_record_dependency_state`).Scan(&maximum))
	seeds, err := loadMatchingRecordsWhere(context.Background(), s.DB, change.Config, `WHERE r.id>?`, []any{maximum})
	require.NoError(t, err)
	return change.Config, seeds
}

func TestRecordDependenciesIncludeRelatedMusicAcrossTime(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	for _, input := range []string{
		dependencyInput(1000, "one", "Desktop", "Song"), dependencyInput(1002, "two", "Desktop", "Song"), dependencyInput(1004, "three", "Desktop", "Song"),
		dependencyInput(2000, "far", "Desktop", "Song"), dependencyInput(1002, "unrelated", "Desktop", "Other"),
	} {
		captureExample(t, s, input, "")
	}
	require.NoError(t, s.ProcessLive(ctx))
	change := currentChange(t, s)
	for i := range change.Config.Rules {
		if r := &change.Config.Rules[i]; r.Type == "matching" && r.Subtype == "play" {
			r.Program = defaultPlayProgram(2)
		}
	}
	version, err := s.Save(ctx, change)
	require.NoError(t, err)
	var indexedVersion int64
	require.NoError(t, s.DB.QueryRow(`SELECT version_id FROM source_record_dependency_state`).Scan(&indexedVersion))
	require.Equal(t, version.ID, indexedVersion)
	config, seeds := unpublishedDependency(t, s, dependencyInput(1003, "new", "Desktop", "Song"))
	require.Len(t, seeds, 1)
	tx, err := s.DB.BeginTx(ctx, nil)
	require.NoError(t, err)
	ids, bounded, err := affectedRecordIDs(ctx, tx, seeds, config)
	require.NoError(t, err)
	require.True(t, bounded)
	require.Equal(t, []int64{1, 2, 3, 4, 6}, ids)
	require.NoError(t, tx.Rollback())
	var indexed int
	require.NoError(t, s.DB.QueryRow(`SELECT COUNT(*) FROM source_record_dependencies WHERE record_id=6`).Scan(&indexed))
	require.Zero(t, indexed, "Candidate lookup must roll back with failed publication")
}

func TestRecordDependenciesFollowNativeIdentifiersAndShiftedExactCopies(t *testing.T) {
	for _, shifted := range []bool{false, true} {
		t.Run(strconv.FormatBool(shifted), func(t *testing.T) {
			s := testStore(t)
			ctx := context.Background()
			captureExample(t, s, dependencyInput(1000, "native", "Desktop", "Song"), "")
			require.NoError(t, s.ProcessLive(ctx))
			stamp := int64(100000)
			if shifted {
				stamp = 1000
				offset := operation("hours", literalExpr(24))
				saveTestRules(t, s, Rule{ID: "shift", Type: "inference", Subtype: "time", Enabled: true, Program: assignWhen(operation("eq", fieldExpr("record.device"), literalExpr("Other")), "time.offset", offset)})
			}
			event := "native"
			if shifted {
				event = "another-event"
			}
			config, seeds := unpublishedDependency(t, s, dependencyInput(stamp, event, "Other", "Song"))
			tx, err := s.DB.BeginTx(ctx, nil)
			require.NoError(t, err)
			defer tx.Rollback()
			ids, bounded, err := affectedRecordIDs(ctx, tx, seeds, config)
			require.NoError(t, err)
			require.True(t, bounded)
			require.Equal(t, []int64{1, 2}, ids)
		})
	}
}

func TestRecordDependenciesCoverUnboundedMusicMatching(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	captureExample(t, s, dependencyInput(1000, "old", "Desktop", "Song"), "")
	require.NoError(t, s.ProcessLive(ctx))
	config, seeds := unpublishedDependency(t, s, dependencyInput(2000, "new", "Desktop", "Song"))
	for i := range config.Rules {
		if r := &config.Rules[i]; r.Type == "matching" && r.Subtype == "play" {
			r.Program = assignWhen(literalExpr(true), "match.play", literalExpr(true))
		}
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer tx.Rollback()
	ids, bounded, err := affectedRecordIDs(ctx, tx, seeds, config)
	require.NoError(t, err)
	require.True(t, bounded)
	require.Equal(t, []int64{1, 2}, ids)
}

func TestWarmDependenciesKeepsPendingNativeRecordsOutsidePublishedBoundary(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	captureExample(t, s, dependencyInput(1000, "old", "Desktop", "Song"), "")
	require.NoError(t, s.ProcessLive(ctx))
	entry := captureExample(t, s, dependencyInput(2000, "new", "Desktop", "Song"), "")
	require.NoError(t, s.StoreLive(ctx, entry))
	_, err := s.DB.Exec(`DELETE FROM source_record_music_keys; DELETE FROM source_record_dependencies; DELETE FROM source_record_dependency_state; PRAGMA cache_size=-256`)
	require.NoError(t, err)
	require.NoError(t, s.WarmRecordDependencies(ctx))
	var maximum, cache int64
	require.NoError(t, s.DB.QueryRow(`SELECT max_record_id FROM source_record_dependency_state`).Scan(&maximum))
	require.EqualValues(t, 1, maximum)
	require.NoError(t, s.DB.QueryRow(`PRAGMA cache_size`).Scan(&cache))
	require.EqualValues(t, -256, cache)
	require.Equal(t, 1, count(t, s, "listens"))
	assertIncrementalMatchesFull(t, s)
}

func TestEmptySnapshotInvalidatesIncrementalDependencies(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	captureExample(t, s, dependencyInput(1000, "old", "Desktop", "Song"), "")
	require.NoError(t, s.ProcessLive(ctx))
	unpublishedDependency(t, s, dependencyInput(2000, "new", "Desktop", "Song"))
	result, err := s.Import(ctx, strings.NewReader(`{"playlists":[]}`), "Playlist1.json", 1)
	require.NoError(t, err)
	require.Zero(t, result.Added)
	before := derivedSnapshot(t, s)
	used, err := s.tryIncrementalLive(ctx)
	require.NoError(t, err)
	require.False(t, used)
	require.Equal(t, before, derivedSnapshot(t, s))
	require.Error(t, s.WarmRecordDependencies(ctx))
	require.NoError(t, s.ProcessLive(ctx))
	require.Equal(t, 2, count(t, s, "listens"))
}

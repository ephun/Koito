package sources

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMusicMigrationRetainsMatchingAndSharedHistoryWithoutChangingNames(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, sensitive := range []bool{false, true} {
			for _, missing := range []bool{false, true} {
				s := testStore(t)
				ctx := context.Background()
				second := strings.ReplaceAll(strings.ReplaceAll(complete, "2020-01-02T03:04:05Z", "2020-01-03T03:04:05Z"), `"Song"`, `"song"`)
				mustImport(t, s, []byte("["+complete+","+second+"]"), "spotify.json", "")
				importLastfm(t, s, "["+lf("song", "", 1577934065)+"]")
				change := currentChange(t, s)
				change.Config.Rules[0].Enabled = enabled
				change.Config.Rules[0].Settings.CaseSensitive = sensitive
				change.Config.Rules[0].Settings.MatchMissingAlbum = missing
				_, err := s.Save(ctx, change)
				require.NoError(t, err)
				var oldHistories int
				require.NoError(t, s.DB.QueryRow(`SELECT COUNT(DISTINCT h.music_track_id) FROM source_plays p JOIN track_histories h ON h.track_id=p.track_id`).Scan(&oldHistories))
				oldRecords, err := loadMatchingRecords(ctx, s.DB, change.Config)
				require.NoError(t, err)
				oldPlan, err := calculateMatches(ctx, oldRecords, change.Config, nil)
				require.NoError(t, err)
				current := currentChange(t, s)
				before, _ := json.Marshal(current.Config)
				current.Config = migrateMusicPrograms(current.Config)
				unchanged, _ := json.Marshal(change.Config)
				require.Equal(t, before, unchanged)
				newRecords, err := loadMatchingRecords(ctx, s.DB, current.Config)
				require.NoError(t, err)
				newPlan, err := calculateMatches(ctx, newRecords, current.Config, nil)
				require.NoError(t, err)
				require.Equal(t, oldPlan.Plays, newPlan.Plays)
				require.Equal(t, oldPlan.Keys, newPlan.Keys)
				_, err = s.Save(ctx, current)
				require.NoError(t, err)
				var histories int
				require.NoError(t, s.DB.QueryRow(`SELECT COUNT(DISTINCT h.music_track_id) FROM source_plays p JOIN track_histories h ON h.track_id=p.track_id`).Scan(&histories))
				require.Equal(t, oldHistories, histories)
				var titles int
				require.NoError(t, s.DB.QueryRow(`SELECT COUNT(DISTINCT t.title) FROM source_plays p JOIN tracks_with_title t ON t.id=p.track_id`).Scan(&titles))
				require.GreaterOrEqual(t, titles, 2)
			}
		}
	}
}

func TestMusicAliasKeepsDisplayAndRecordingIdentitySeparateAndReverses(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	second := strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(complete, "2020-01-02T03:04:05Z", "2020-01-03T03:04:05Z"), `"Song"`, `"Other spelling"`), "spotify:track:one", "spotify:track:two")
	mustImport(t, s, []byte("["+complete+","+second+"]"), "spotify.json", "")
	change := currentChange(t, s)
	change.Config = migrateMusicPrograms(change.Config)
	_, err := s.Save(ctx, change)
	require.NoError(t, err)
	program := namingFixture(t, `{"steps":[{"if":{"op":"eq","args":[{"op":"field","field":"original.title"},{"op":"literal","value":"Other spelling"}]},"then":[{"set":"music.title","value":{"op":"literal","value":"Song"}}]}]}`)
	change = currentChange(t, s)
	change.Config.Rules = append(change.Config.Rules, Rule{ID: "alias", Type: "matching", Subtype: "music", Enabled: true, Program: program})
	preview, err := s.Preview(ctx, change)
	require.NoError(t, err)
	require.Contains(t, preview.MusicExamples, NamingExample{RecordID: 2, Field: "Track", Before: "Other spelling", After: "Song"})
	for _, enabled := range []bool{true, false, true} {
		for i := range change.Config.Rules {
			if change.Config.Rules[i].ID == "alias" {
				change.Config.Rules[i].Enabled = enabled
			}
		}
		change.BaseVersion = currentChange(t, s).BaseVersion
		_, err = s.Save(ctx, change)
		require.NoError(t, err)
		var histories, recordings, titles int
		require.NoError(t, s.DB.QueryRow(`SELECT COUNT(DISTINCT h.music_track_id),COUNT(DISTINCT r.recording_key),COUNT(DISTINCT t.title) FROM source_plays p JOIN track_histories h ON h.track_id=p.track_id JOIN track_recordings r ON r.track_id=p.track_id JOIN tracks_with_title t ON t.id=p.track_id`).Scan(&histories, &recordings, &titles))
		if enabled {
			require.Equal(t, 1, histories)
		} else {
			require.Equal(t, 2, histories)
		}
		require.Equal(t, 2, recordings)
		require.Equal(t, 2, titles)
		require.Equal(t, 2, count(t, s, "listens"))
	}
	_, err = s.Rebuild(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, count(t, s, "source_records"))
}

func TestSourceScopedMatchingVariantsSeparateEqualDisplayNamesAndRestoreOriginalIDs(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	second := strings.ReplaceAll(complete, "2020-01-02T03:04:05Z", "2020-01-03T03:04:05Z")
	mustImport(t, s, []byte("["+complete+","+second+"]"), "spotify.json", "")
	change := currentChange(t, s)
	change.Config = migrateMusicPrograms(change.Config)
	_, err := s.Save(ctx, change)
	require.NoError(t, err)
	var original int64
	require.NoError(t, s.DB.QueryRow(`SELECT track_id FROM source_plays LIMIT 1`).Scan(&original))
	change = currentChange(t, s)
	change.Config.Rules = append(change.Config.Rules, Rule{ID: "specific", Type: "matching", Subtype: "music", Enabled: true, Program: namingFixture(t, `{"steps":[{"if":{"op":"eq","args":[{"op":"field","field":"record.id"},{"op":"literal","value":1}]},"then":[{"set":"music.title","value":{"op":"literal","value":"Separate history"}}]}]}`)})
	for _, enabled := range []bool{true, false, true, false} {
		for i := range change.Config.Rules {
			if change.Config.Rules[i].ID == "specific" {
				change.Config.Rules[i].Enabled = enabled
			}
		}
		change.BaseVersion = currentChange(t, s).BaseVersion
		_, err = s.Save(ctx, change)
		require.NoError(t, err)
		var appearances, histories int
		require.NoError(t, s.DB.QueryRow(`SELECT COUNT(DISTINCT p.track_id),COUNT(DISTINCT h.music_track_id) FROM source_plays p JOIN track_histories h ON h.track_id=p.track_id`).Scan(&appearances, &histories))
		if enabled {
			require.Equal(t, 2, appearances)
			require.Equal(t, 2, histories)
		} else {
			require.Equal(t, 1, appearances)
			require.Equal(t, 1, histories)
			var after int64
			require.NoError(t, s.DB.QueryRow(`SELECT track_id FROM source_plays LIMIT 1`).Scan(&after))
			require.Equal(t, original, after)
		}
		var display int
		require.NoError(t, s.DB.QueryRow(`SELECT COUNT(*) FROM source_plays p JOIN tracks_with_title t ON t.id=p.track_id WHERE title='Song'`).Scan(&display))
		require.Equal(t, 2, display)
	}
}

func TestMusicProgramIssuesAreInspectableAndDiscardAllActions(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	mustImport(t, s, []byte("["+complete+"]"), "spotify.json", "")
	change := currentChange(t, s)
	change.Config = migrateMusicPrograms(change.Config)
	change.Config.Rules = append(change.Config.Rules, Rule{ID: "missing", Name: "Alias check", Type: "matching", Subtype: "music", Enabled: true, Program: namingFixture(t, `{"steps":[{"set":"music.title","value":{"op":"literal","value":"Changed"}},{"if":{"op":"eq","args":[{"op":"field","field":"raw:/absent"},{"op":"literal","value":"x"}]},"then":[{"set":"music.artist","value":{"op":"literal","value":"Changed"}}]}]}`)})
	_, err := s.Save(ctx, change)
	require.NoError(t, err)
	record, err := s.Inspect(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, "Song", record.Record.Values.Music.Title)
	require.Len(t, record.FilterIssues, 1)
	require.Equal(t, "Alias check", record.FilterIssues[0].Name)
	require.JSONEq(t, complete, record.Record.Payload)
}

func TestEmptyFilterConfigurationRemainsEmptyAcrossRebuildAndReload(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	mustImport(t, s, []byte("["+complete+","+complete+"]"), "history.json", "")
	change := currentChange(t, s)
	originalVersion := change.BaseVersion
	change.Config = RuleConfig{Rules: []Rule{}}
	_, err := s.Save(ctx, change)
	require.NoError(t, err)
	require.Equal(t, 2, count(t, s, "listens"))
	state, err := s.Filters(ctx)
	require.NoError(t, err)
	require.Empty(t, state.Current.Config.Rules)
	require.Empty(t, state.Blocks.Additions)
	require.Empty(t, state.Blocks.Legacy)
	_, err = s.Rebuild(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, count(t, s, "listens"))
	_, err = s.Save(ctx, FilterChange{BaseVersion: state.Current.ID, RestoreVersion: originalVersion})
	require.NoError(t, err)
	require.Equal(t, 1, count(t, s, "listens"))
	require.Equal(t, 2, count(t, s, "source_records"))
}

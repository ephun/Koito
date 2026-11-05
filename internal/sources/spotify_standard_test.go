package sources

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/gabehf/koito/internal/filters"
	"github.com/stretchr/testify/require"
)

func TestStandardHistoryCompletionAndOverlap(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	standard := `[{"endTime":"2020-01-02 03:04","artistName":"Artist","trackName":"Song","msPlayed":180001},{"endTime":"2020-01-02 04:04","artistName":"Artist","trackName":"Short","msPlayed":29999},{"endTime":"2020-01-02 05:04","episodeName":"Episode","podcastName":"Show","msPlayed":50000},{"endTime":"2020-01-02 06:04","chapterName":"Chapter","audiobookName":"Book","msPlayed":50000}]`
	first, err := s.Import(ctx, bytes.NewBufferString(standard), "StreamingHistory_music_0.json", 1)
	require.NoError(t, err)
	require.Equal(t, 4, first.Added)
	again, err := s.Import(ctx, bytes.NewBufferString(standard), "renamed.json", 1)
	require.NoError(t, err)
	require.Zero(t, again.Added)
	require.Equal(t, 4, again.Reused)
	_, err = s.Import(ctx, bytes.NewBufferString("["+complete+"]"), "extended.json", 1)
	require.NoError(t, err)
	version, err := readRuleVersion(ctx, s.DB, 0)
	require.NoError(t, err)
	records, err := loadMatchingRecords(ctx, s.DB, version.Config)
	require.NoError(t, err)
	require.Equal(t, int64(time.Minute), records[0].Values.Precision)
	require.Equal(t, "", records[0].Values.URI)
	_, exists := recordEnvironment(records[0])["record.completed"]
	require.False(t, exists, "missing completion flags must stay missing")
	require.Equal(t, "excluded by completion", records[1].Values.Reason)
	result, err := s.Rebuild(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, result.Plays)
	var representative int64
	require.NoError(t, s.DB.QueryRow(`SELECT representative_record FROM source_plays`).Scan(&representative))
	require.Equal(t, records[4].ID, representative, "retain extended precision and metadata")
	for i := range version.Config.Rules {
		if version.Config.Rules[i].Type == "qualification" {
			version.Config.Rules[i].Enabled = false
		}
	}
	records, err = loadMatchingRecords(ctx, s.DB, version.Config)
	require.NoError(t, err)
	require.Equal(t, "counted", records[1].Values.Reason)
	// A user can change this same filter to qualify incomplete extended plays.
	value := literalExpr(true)
	version.Config.Rules = append(version.Config.Rules, Rule{ID: "all-music", Type: "qualification", Subtype: "completion", Enabled: true, Program: &filters.Program{Steps: []filters.Statement{{Set: "completion.count", Value: &value}}}})
	require.NoError(t, validateRules(version.Config))
	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(skipped), &payload))
	r := matchingRecord{Service: "spotify", Parser: Parser, Payload: skipped}
	interpretMatchingRecord(&r, version.Config)
	require.Equal(t, "counted", r.Values.Reason)
	for _, raw := range []string{`{"endTime":"bad","artistName":"Artist","trackName":"Song","msPlayed":50000}`, `{"endTime":"2020-01-02 03:04","episodeName":"Episode","msPlayed":50000}`} {
		r := matchingRecord{Service: "spotify", Parser: SpotifyStandard, Payload: raw}
		interpretMatchingRecord(&r, version.Config)
		require.NotEqual(t, "counted", r.Values.Reason)
	}
}

func TestStandardMinuteMultiplicity(t *testing.T) {
	s := testStore(t)
	data := `[{"endTime":"2020-01-02 03:04","artistName":"Artist","trackName":"Song","msPlayed":30000},{"endTime":"2020-01-02 03:04","artistName":"Artist","trackName":"Song","msPlayed":30000}]`
	_, err := s.Import(context.Background(), bytes.NewBufferString(data), "history.json", 1)
	require.NoError(t, err)
	result, err := s.Rebuild(context.Background())
	require.NoError(t, err)
	require.Equal(t, 2, result.Plays, "minute precision cannot prove duplicate rows describe one playback")
}

func TestCompletionDisableRemoveAndRebuild(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	data := `[{"endTime":"2020-01-02 03:04","artistName":"Artist","trackName":"Song","msPlayed":29999}]`
	_, err := s.Import(ctx, bytes.NewBufferString(data), "history.json", 1)
	require.NoError(t, err)
	result, err := s.Rebuild(ctx)
	require.NoError(t, err)
	require.Zero(t, result.Plays)
	change := currentChange(t, s)
	testRule(t, &change.Config, "spotify-account-completion").Enabled = false
	_, err = s.Save(ctx, change)
	require.NoError(t, err)
	require.Equal(t, 1, count(t, s, "listens"))
	change = currentChange(t, s)
	for i, r := range change.Config.Rules {
		if r.ID == "spotify-account-completion" {
			change.Config.Rules = append(change.Config.Rules[:i], change.Config.Rules[i+1:]...)
			break
		}
	}
	_, err = s.Save(ctx, change)
	require.NoError(t, err)
	result, err = s.Rebuild(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, result.Plays)
	state, err := s.Filters(ctx)
	require.NoError(t, err)
	for _, r := range state.Current.Config.Rules {
		require.NotEqual(t, "spotify-account-completion", r.ID)
	}
	require.Equal(t, 1, count(t, s, "source_records"))
}

package sources

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gabehf/koito/internal/filters"
	"github.com/stretchr/testify/require"
)

func TestPlayMigrationPreservesExactCopiesTimingAmbiguityAndDisabledSettings(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	duplicate := strings.ReplaceAll(complete, `"unknown":{"keep":true}`, `"unknown":{"keep":false}`)
	mustImport(t, s, []byte("["+complete+","+duplicate+"]"), "spotify.json", "")
	importLastfm(t, s, "["+lf("Song", "Album", 1577934065)+","+lf("Song", "Album", 1577934069)+"]")
	base := currentChange(t, s).Config
	for _, enabled := range []bool{false, true} {
		for _, seconds := range []int{0, 5, 3600} {
			config := RuleConfig{Rules: append([]Rule(nil), base.Rules...)}
			config.Rules[1].Enabled = enabled
			config.Rules[1].Settings.ToleranceSeconds = seconds
			before, _ := json.Marshal(config)
			migrated := migratePlayPrograms(config)
			after, _ := json.Marshal(config)
			require.Equal(t, before, after)
			records, err := loadMatchingRecords(ctx, s.DB, config)
			require.NoError(t, err)
			original, err := calculateMatches(ctx, records, config, nil)
			require.NoError(t, err)
			converted, err := calculateMatches(ctx, records, migrated, nil)
			require.NoError(t, err)
			require.Equal(t, original, converted)
			if enabled {
				require.Equal(t, time.Duration(seconds)*time.Second, playSearchGap(migrated))
			} else {
				require.Zero(t, playSearchGap(migrated))
			}
		}
	}
}

func TestPairExpressionsUseStableSidesExactDuplicatesAndRawMetadata(t *testing.T) {
	s := testStore(t)
	config := migratePlayPrograms(currentChange(t, s).Config)
	mustImport(t, s, []byte("["+complete+"]"), "spotify.json", "")
	importLastfm(t, s, "["+lf("Song", "Album", 1577934065)+"]")
	condition := operation("and", operation("eq", fieldExpr("left.source.service"), literalExpr("spotify")), operation("eq", fieldExpr("right.source.service"), literalExpr("lastfm")), operation("eq", fieldExpr("left.raw:/unknown/keep"), literalExpr(true)))
	match := literalExpr(true)
	reject := literalExpr(false)
	config.Rules[1].Program = &filters.Program{Steps: []filters.Statement{{If: &condition, Then: []filters.Statement{{Set: "match.play", Value: &match}}, Else: []filters.Statement{{Set: "match.play", Value: &reject}}}}}
	records, err := loadMatchingRecords(context.Background(), s.DB, config)
	require.NoError(t, err)
	left, right := records[0], records[1]
	require.True(t, resolvePlayPair(context.Background(), config, right, left).Match)
	plan, err := calculateMatches(context.Background(), records, config, nil)
	require.NoError(t, err)
	require.Len(t, plan.Plays, 1)
	left.RawFields["raw:/unknown/keep"] = filters.Bool(false)
	require.False(t, resolvePlayPair(context.Background(), config, left, right).Match)
}

func TestPlayConditionsCanUseUnboundedTimeWithoutDroppingDistantCandidates(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	mustImport(t, s, []byte("["+complete+"]"), "spotify.json", "")
	importLastfm(t, s, "["+lf("Song", "Album", time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC).Unix())+"]")
	config := migratePlayPrograms(currentChange(t, s).Config)
	config.Rules[1].Program = namingFixture(t, `{"steps":[{"set":"match.play","value":{"op":"eq","args":[{"op":"field","field":"left.original.title"},{"op":"field","field":"right.original.title"}]}}]}`)
	require.Equal(t, unlimitedPairGap, playSearchGap(config))
	records, err := loadMatchingRecords(ctx, s.DB, config)
	require.NoError(t, err)
	plan, err := calculateMatches(ctx, records, config, nil)
	require.NoError(t, err)
	require.Len(t, plan.Plays, 1)
	config.Rules[1].Program = namingFixture(t, `{"steps":[{"if":{"op":"le","args":[{"op":"field","field":"pair.gap"},{"op":"hours","args":[{"op":"literal","value":1}]}]},"then":[{"set":"match.play","value":{"op":"literal","value":true}}]}]}`)
	require.Equal(t, time.Hour, playSearchGap(config))
	plan, err = calculateMatches(ctx, records, config, nil)
	require.NoError(t, err)
	require.Len(t, plan.Plays, 2)
}

func TestPlayProgramsRespectPrioritiesConflictsAndUnknownConditions(t *testing.T) {
	r := matchingRecord{ID: 1, SourceID: 1, Values: recordValues{Title: "Song"}}
	other := r
	other.ID = 2
	other.SourceID = 2
	accept := Rule{ID: "accept", Type: "matching", Subtype: "play", Enabled: true, Program: namingFixture(t, `{"steps":[{"set":"match.play","value":{"op":"literal","value":true}}]}`)}
	reject := accept
	reject.ID = "reject"
	reject.Program = namingFixture(t, `{"steps":[{"set":"match.play","value":{"op":"literal","value":false}}]}`)
	config := RuleConfig{Rules: []Rule{accept, reject}}
	result := resolvePlayPair(context.Background(), config, r, other)
	require.True(t, result.Conflict)
	require.False(t, result.Match)
	priority := 10
	config.Rules[0].Priority = &priority
	require.True(t, resolvePlayPair(context.Background(), config, r, other).Match)
	config.Rules[0].Program = namingFixture(t, `{"steps":[{"set":"match.play","value":{"op":"literal","value":true}},{"if":{"op":"eq","args":[{"op":"field","field":"left.raw:/missing"},{"op":"literal","value":"X"}]},"then":[{"set":"match.play","value":{"op":"literal","value":true}}]}]}`)
	result = resolvePlayPair(context.Background(), config, r, other)
	require.False(t, result.Match)
	require.NotEmpty(t, result.Issues)
}

func TestPlayProgramCanRejectExactSourceDuplicatesAndUndoWithoutChangingRecords(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	mustImport(t, s, []byte("["+complete+","+complete+"]"), "history.json", "")
	change := currentChange(t, s)
	change.Config = migratePlayPrograms(change.Config)
	_, err := s.Save(ctx, change)
	require.NoError(t, err)
	require.Equal(t, 1, count(t, s, "listens"))
	change = currentChange(t, s)
	change.Config.Rules[1].Program = namingFixture(t, `{"steps":[{"set":"match.play","value":{"op":"not","args":[{"op":"field","field":"pair.exact"}]}}]}`)
	_, err = s.Save(ctx, change)
	require.NoError(t, err)
	require.Equal(t, 2, count(t, s, "listens"))
	change = currentChange(t, s)
	change.Config.Rules[1].Program = defaultPlayProgram(0)
	_, err = s.Save(ctx, change)
	require.NoError(t, err)
	require.Equal(t, 1, count(t, s, "listens"))
	require.Equal(t, 2, count(t, s, "source_records"))
	_, err = s.Rebuild(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, count(t, s, "listens"))
}

func TestPairInspectionExplainsMissingFieldsAndDoesNotChangeView(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	mustImport(t, s, []byte("["+complete+","+complete+"]"), "history.json", "")
	change := currentChange(t, s)
	change.Config = migratePlayPrograms(change.Config)
	change.Config.Rules[1].Program = namingFixture(t, `{"steps":[{"set":"match.play","value":{"op":"eq","args":[{"op":"field","field":"left.raw:/absent"},{"op":"literal","value":"x"}]}}]}`)
	_, err := s.Save(ctx, change)
	require.NoError(t, err)
	result, err := s.InspectPair(ctx, 2, 1)
	require.NoError(t, err)
	require.EqualValues(t, 1, result.Left)
	require.EqualValues(t, 2, result.Right)
	require.True(t, result.Music)
	require.False(t, result.Play)
	require.Len(t, result.Issues, 1)
	require.Contains(t, result.Issues[0].Reason, "left.raw:/absent")
	require.Equal(t, 2, count(t, s, "listens"))
	require.Equal(t, 2, count(t, s, "source_records"))
	_, err = s.InspectPair(ctx, 1, 1)
	require.Error(t, err)
	_, err = s.InspectPair(ctx, 1, 99)
	require.Error(t, err)
}

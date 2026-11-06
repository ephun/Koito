package sources

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gabehf/koito/internal/filters"
	"github.com/stretchr/testify/require"
)

func TestLiveEvidenceDistinguishesMissingZeroAndCompletion(t *testing.T) {
	for _, test := range []struct {
		extra string
		known bool
		want  int64
	}{
		{`"duration_ms":180000`, false, 0},
		{`"duration_ms":180000,"played_ms":null`, false, 0},
		{`"duration_ms":180000,"played_ms":"invalid"`, false, 0},
		{`"duration_ms":180000,"played_ms":0`, true, 0},
		{`"duration_ms":180000,"played_ms":1200`, true, 1200},
	} {
		raw := json.RawMessage(`{"listened_at":1793937600,"track_metadata":{"artist_name":"Artist","track_name":"Song","additional_info":{` + test.extra + `}}}`)
		v := normalizeRecord(raw, LiveListenBrainz, "UTC")
		env := recordEnvironment(matchingRecord{Values: v})
		value, present := env["record.played_ms"]
		require.Equal(t, test.known, present, test.extra)
		if present {
			actual, known := value.Int64()
			require.True(t, known)
			require.Equal(t, test.want, actual)
		}
		_, present = env["record.completed"]
		require.False(t, present)
		require.EqualValues(t, 180000, v.DurationMS)
	}
	raw := json.RawMessage(`{"listened_at":1793937600,"track_metadata":{"artist_name":"Artist","track_name":"Song","additional_info":{"completed":false,"skipped":true,"played_ms":0,"event_id":9007199254740993,"music_service_name":"Spotify","media_player":"Player"}}}`)
	v := normalizeRecord(raw, LiveListenBrainz, "UTC")
	env := recordEnvironment(matchingRecord{Values: v})
	complete, known := env["record.completed"].Bool()
	require.True(t, known)
	require.False(t, complete)
	skipped, known := env["record.skipped"].Bool()
	require.True(t, known)
	require.True(t, skipped)
	require.Equal(t, "event_id:Spotify:9007199254740993", v.EventID)
	require.Equal(t, "Player", v.MediaPlayer)
}

func TestValidityIsReversibleAndRetainsRecords(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	long := strings.ReplaceAll(strings.ReplaceAll(complete, "180001", "300001"), "03:04:05", "03:14:05")
	mustImport(t, s, []byte("["+complete+","+long+","+skipped+","+podcast+"]"), "music.json", "")
	change := currentChange(t, s)
	program := assignWhen(operation("and", operation("eq", fieldExpr("record.media_type"), literalExpr("music")), operation("lt", fieldExpr("record.played_ms"), literalExpr(200000))), "validity.count", literalExpr(false))
	change.Config.Rules = append(change.Config.Rules, Rule{ID: "usable-duration", Name: "Played time", Type: "qualification", Subtype: "validity", Enabled: true, Program: program})
	_, err := s.Save(ctx, change)
	require.NoError(t, err)
	require.Equal(t, 4, count(t, s, "source_records"))
	require.Equal(t, 1, count(t, s, "listens"))
	change = currentChange(t, s)
	for i := range change.Config.Rules {
		if change.Config.Rules[i].ID == "usable-duration" {
			change.Config.Rules[i].Enabled = false
		}
	}
	_, err = s.Save(ctx, change)
	require.NoError(t, err)
	require.Equal(t, 4, count(t, s, "source_records"))
	require.Equal(t, 2, count(t, s, "listens"))
}

func TestValidityKeepsStructuralErrorsAndUnconfiguredDefaults(t *testing.T) {
	config := RuleConfig{Rules: []Rule{{ID: "allow", Type: "qualification", Subtype: "validity", Enabled: true, Program: assignWhen(literalExpr(true), "validity.count", literalExpr(true))}}}
	require.NoError(t, prepareValidityPrograms(config))
	for _, reason := range []string{"invalid timestamp", "invalid played duration", "not an identified music track", "now playing"} {
		r := matchingRecord{Values: recordValues{Reason: reason}}
		applyValidity(&r, config)
		require.Equal(t, reason, r.Values.Reason)
	}
	r := matchingRecord{Values: recordValues{Reason: "source duplicate"}}
	applyValidity(&r, RuleConfig{})
	require.Equal(t, "source duplicate", r.Values.Reason)
	applyValidity(&r, config)
	require.Equal(t, "counted", r.Values.Reason)
	_, err := filters.CompileProgram(*config.Rules[0].Program, validitySchema())
	require.NoError(t, err)
}

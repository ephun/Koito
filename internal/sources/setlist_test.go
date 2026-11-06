package sources

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLegacyConcertDateAndReferences(t *testing.T) {
	for _, tc := range []struct {
		album, date, venue, id, reason string
	}{
		{"Live at  (03-10-2014)", "2014-10-03", "", "", "counted"},
		{"Live at Venue (03-10-2014)", "2014-10-03", "Venue", "", "counted"},
		{"SetlistFM Concert 4b4b1756", "", "", "4b4b1756", "unresolved concert date"},
		{"Studio Album", "", "", "", "unresolved concert date"},
	} {
		t.Run(tc.album, func(t *testing.T) {
			v, ok := normalizeLegacyConcert(map[string]any{"title": "Song", "artist": "Performer"}, map[string]any{"native_original_fields": map[string]any{"client": "SetlistFM", "release_title": tc.album, "listened_at": 1700000000, "track_id": 3, "user_id": 1}})
			require.True(t, ok)
			require.Equal(t, tc.reason, v.Reason)
			require.Equal(t, tc.date, v.Performance.LocalDate)
			require.Equal(t, tc.venue, v.Performance.Venue)
			require.Equal(t, tc.id, v.Performance.SetlistID)
			require.True(t, v.Performance.DateOnly)
			if tc.date == "" {
				require.True(t, v.Time.IsZero(), "generated legacy timestamps do not establish observed concert dates")
			} else {
				require.Equal(t, tc.date, v.Time.Format(time.DateOnly))
			}
		})
	}
}

func TestExplicitEstimatedTimeCanResolveAnUndatedLegacyConcert(t *testing.T) {
	row := map[string]any{"title": "Song", "artist": "Performer"}
	payload := map[string]any{"native_original_fields": map[string]any{"client": "SetlistFM", "release_title": "SetlistFM Concert 4b4b1756", "listened_at": 1700000000, "track_id": 3, "user_id": 1}}
	v, ok := normalizeLegacyConcert(row, payload)
	require.True(t, ok)
	require.Equal(t, "unresolved concert date", v.Reason)
	r := matchingRecord{Service: "setlistfm", Values: v}
	config := RuleConfig{Rules: []Rule{{ID: "estimated-date", Type: "inference", Subtype: "time", Enabled: true, Program: assignWhen(contextKindCondition("concert"), "time.at", operation("date", literalExpr("2020-01-02T20:00:00Z")))}}}
	require.NoError(t, prepareTimePrograms(config))
	applyObservedTime(&r, config)
	require.Empty(t, r.TimeIssues)
	require.Equal(t, "counted", r.Values.Reason)
	require.True(t, r.Values.TimeEstimated)
	require.True(t, r.Values.Performance.TimeEstimated)
	require.False(t, r.Values.Performance.DateOnly)
	require.Equal(t, "2020-01-02T20:00:00Z", r.Start.Format(time.RFC3339))
	require.Equal(t, "4b4b1756", r.Values.Performance.SetlistID)
	require.Equal(t, 1700000000, object(payload["native_original_fields"])["listened_at"])
}

func setlistFixture(id, revision, updated, title string) []byte {
	return []byte(fmt.Sprintf(`{"type":"setlist","id":%q,"versionId":%q,"lastUpdated":%q,"eventDate":"02-01-2020","artist":{"name":"Performer","mbid":"artist-id"},"venue":{"name":"Venue","city":{"name":"City"}},"sets":{"set":[{"song":[{"name":%q,"cover":{"name":"Original Artist"},"unknown":"keep"}]},{"encore":1,"song":[{"name":%q}]}]},"unknown":{"keep":true}}`, id, revision, updated, title, title))
}

func addConcertListener(t *testing.T, change *FilterChange) {
	change.Config.Rules = append(change.Config.Rules, Rule{ID: "concert-attendance", Type: "attribution", Subtype: "direct", Enabled: true, Program: namingFixture(t, `{"steps":[{"if":{"op":"and","args":[{"op":"present","args":[{"op":"field","field":"event.kind"}]},{"op":"eq","args":[{"op":"field","field":"event.kind"},{"op":"literal","value":"concert"}]}]},"then":[{"set":"listener","value":{"op":"literal","value":"self"}}]}]}`)})
}

func TestSetlistRevisionsAndRepeatedEncores(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	firstData := setlistFixture("concert-one", "version-one", "2020-01-03T12:00:00.000+0000", "First Name")
	first, err := s.Import(ctx, bytes.NewReader(firstData), "setlist.json", 1)
	require.NoError(t, err)
	require.Equal(t, 3, first.Records)
	path, err := s.FilePath(first.Hash)
	require.NoError(t, err)
	original, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, firstData, original)
	result, err := s.Rebuild(ctx)
	require.NoError(t, err)
	require.Zero(t, result.Plays, "attendance attribution is explicit")
	change := currentChange(t, s)
	addConcertListener(t, &change)
	_, err = s.Save(ctx, change)
	require.NoError(t, err)
	require.Equal(t, 2, count(t, s, "listens"), "an encore repeat is a separate occurrence")
	updated := setlistFixture("concert-one", "version-two", "2020-01-04T12:00:00.000+0000", "Corrected Name")
	_, err = s.Import(ctx, bytes.NewReader(updated), "corrected.json", 1)
	require.NoError(t, err)
	repeat, err := s.Import(ctx, bytes.NewReader(updated), "renamed.json", 1)
	require.NoError(t, err)
	require.Zero(t, repeat.Added)
	result, err = s.Rebuild(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, result.Plays)
	require.Equal(t, 6, count(t, s, "source_records"))
	var superseded int
	require.NoError(t, s.DB.QueryRow(`SELECT COUNT(*) FROM source_interpretations WHERE reason='superseded setlist revision'`).Scan(&superseded))
	require.Equal(t, 3, superseded)
	var raw string
	require.NoError(t, s.DB.QueryRow(`SELECT context_json FROM source_play_context LIMIT 1`).Scan(&raw))
	require.Contains(t, raw, `"date_only":true`)
	require.Contains(t, raw, `"local_date":"2020-01-02"`)
	require.Contains(t, raw, `"performer":"Performer"`)
	secondConcert := setlistFixture("concert-two", "version-three", "2020-01-05T12:00:00.000+0000", "Corrected Name")
	_, err = s.Import(ctx, bytes.NewReader(secondConcert), "other-show.json", 1)
	require.NoError(t, err)
	result, err = s.Rebuild(ctx)
	require.NoError(t, err)
	require.Equal(t, 4, result.Plays, "two performances on one day remain distinct")
}

func TestSetlistUnknownRevisionsRemainUnresolved(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	for _, version := range []string{"one", "two"} {
		_, err := s.Import(ctx, bytes.NewReader(setlistFixture("concert", version, "", "Song")), "unknown.json", 1)
		require.NoError(t, err)
	}
	change := currentChange(t, s)
	addConcertListener(t, &change)
	_, err := s.Save(ctx, change)
	require.NoError(t, err)
	require.Zero(t, count(t, s, "listens"))
	var unresolved int
	require.NoError(t, s.DB.QueryRow(`SELECT COUNT(*) FROM source_interpretations WHERE reason='conflicting setlist revisions'`).Scan(&unresolved))
	require.Equal(t, 6, unresolved)
}

func TestSetlistPageKeepsAttendanceAndCoverCredits(t *testing.T) {
	page := []byte(`{"type":"setlists","page":1,"itemsPerPage":20,"total":1,"attendance":{"attended":true,"userId":"person"},"setlist":[` + string(setlistFixture("concert", "version", "2020-01-03T00:00:00Z", "Song")) + `]}`)
	part, err := parseSetlists(File{}, page)
	require.NoError(t, err)
	require.Len(t, part.records, 4)
	v := normalizeRecord(part.records[2], SetlistJSON, "")
	require.Equal(t, "counted", v.Reason)
	require.Equal(t, "date", v.TimeMeaning)
	require.Equal(t, "Performer", v.Artist)
	require.Equal(t, "Original Artist", v.Performance.CoverArtist)
	require.NotNil(t, v.Performance.Attended)
	require.True(t, *v.Performance.Attended)
	require.Contains(t, string(part.records[2]), `"unknown":"keep"`)
	_, err = parseSetlists(File{}, []byte(`{"type":"setlists","setlist":[]}`))
	require.NoError(t, err)
	bad := strings.Replace(string(page), "02-01-2020", "not-a-date", 1)
	part, err = parseSetlists(File{}, []byte(bad))
	require.NoError(t, err)
	require.Equal(t, "invalid concert date", normalizeRecord(part.records[2], SetlistJSON, "").Reason)
}

func TestLegacyConcertKeepsCalendarDateAndOriginalPayload(t *testing.T) {
	row := map[string]any{"title": "Song", "artist": "Performer", "timestamp": "2020-01-02T20:00:00Z", "payload_json": `{"native_event_id":"legacy-event","native_original_fields":{"client":"SetlistFM","release_title":"Live at Venue (02-01-2020)","listened_at":1577995200,"user_id":1,"track_id":123}}`}
	v := normalizeHistoryRow(row, "")
	require.Equal(t, "counted", v.Reason)
	require.Equal(t, "date", v.TimeMeaning)
	require.Equal(t, "derived", v.Origin)
	require.True(t, v.Performance.DateOnly)
	require.True(t, v.Performance.Legacy)
	require.Equal(t, "2020-01-02", v.Performance.LocalDate)
	require.Empty(t, v.Performance.SetlistID)
	require.Nil(t, v.Performance.Attended)
	require.Equal(t, "legacy-event", v.EventID)
}

func TestConcertTimeEstimateRemainsExplicit(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	_, err := s.Import(ctx, bytes.NewReader(setlistFixture("concert", "version", "2020-01-03T00:00:00Z", "Song")), "concert.json", 1)
	require.NoError(t, err)
	change := currentChange(t, s)
	addConcertListener(t, &change)
	change.Config.Rules = append(change.Config.Rules, Rule{ID: "time", Type: "inference", Subtype: "time", Enabled: true, Program: assignWhen(contextKindCondition("concert"), "time.at", operation("date", literalExpr("2020-01-02T20:00:00Z")))})
	_, err = s.Save(ctx, change)
	require.NoError(t, err)
	require.Equal(t, 2, count(t, s, "listens"))
	var marked int
	require.NoError(t, s.DB.QueryRow(`SELECT COUNT(*) FROM source_play_context WHERE json_extract(context_json,'$.time_estimated')=1 AND COALESCE(json_extract(context_json,'$.date_only'),0)=0`).Scan(&marked))
	require.Equal(t, 2, marked)
	change = currentChange(t, s)
	testRule(t, &change.Config, "time").Enabled = false
	_, err = s.Save(ctx, change)
	require.NoError(t, err)
	require.NoError(t, s.DB.QueryRow(`SELECT COUNT(*) FROM source_play_context WHERE json_extract(context_json,'$.date_only')=1`).Scan(&marked))
	require.Equal(t, 2, marked)
}

package sources

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gabehf/koito/internal/filters"
	"github.com/stretchr/testify/require"
)

func TestLocalPhase7CombinedHistory(t *testing.T) {
	for _, sensitive := range []bool{false, true} {
		t.Run(fmt.Sprintf("case-sensitive-%t", sensitive), func(t *testing.T) {
			testLocalPhase7CombinedHistory(t, sensitive)
		})
	}
}

func testLocalPhase7CombinedHistory(t *testing.T, sensitive bool) {
	root := os.Getenv("KOITO_TEST_PHASE7_ROOT")
	if root == "" {
		t.Skip("set KOITO_TEST_PHASE7_ROOT to the workspace for private-data acceptance")
	}
	s := testStore(t)
	ctx := context.Background()
	for _, path := range []string{
		"data_dump/YARG and Clone Hero Export 11-2-26/YARG/release/scores/scores.db",
		"data_dump/YARG and Clone Hero Export 11-2-26/YARG/release/songcache.bin",
		".local/phase7-data/concerts.jsonl", ".local/phase7-data/yargfm.jsonl",
		".local/phase7-data/lastfm-games.csv",
	} {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		require.NoError(t, err)
		first, err := s.Import(ctx, bytes.NewReader(data), filepath.Base(path), 1)
		require.NoError(t, err)
		again, err := s.Import(ctx, bytes.NewReader(data), "renamed.data", 1)
		require.NoError(t, err)
		require.Zero(t, again.Added)
		require.Equal(t, first.Records, again.Reused)
	}
	change := currentChange(t, s)
	for i := range change.Config.Rules {
		if change.Config.Rules[i].Type == "matching" && change.Config.Rules[i].Subtype == "music" {
			change.Config.Rules[i].Settings.CaseSensitive = sensitive
		}
	}
	if sensitive {
		change.Config = migrateMusicPrograms(change.Config)
	}
	addYARGTime(t, &change)
	addYARGListener(t, &change, "ephun")
	addConcertListener(t, &change)
	change.Config.Rules = append(change.Config.Rules,
		Rule{ID: "fm-self", Type: "attribution", Subtype: "direct", Enabled: true, Program: assignWhen(operation("and", operation("eq", fieldExpr("source.service"), literalExpr("yargfm")), operation("eq", fieldExpr("source.format"), literalExpr(NormalizedJSONL))), "listener", literalExpr("self"))},
		Rule{ID: "fm-context", Type: "inference", Subtype: "context", Enabled: true, Program: assignWhen(operation("and", operation("eq", fieldExpr("source.service"), literalExpr("yargfm")), operation("eq", fieldExpr("source.format"), literalExpr(NormalizedJSONL))), "context.kind", literalExpr("gameplay"))})
	fmCondition := operation("and", operation("eq", fieldExpr("source.service"), literalExpr("yargfm")), operation("eq", fieldExpr("source.format"), literalExpr(NormalizedJSONL)))
	meaning, precision := literalExpr("result"), operation("seconds", literalExpr(1))
	change.Config.Rules = append(change.Config.Rules, Rule{ID: "fm-result-time", Type: "inference", Subtype: "time", Enabled: true, Program: &filters.Program{Steps: []filters.Statement{{If: &fmCondition, Then: []filters.Statement{{Set: "time.meaning", Value: &meaning}, {Set: "time.precision", Value: &precision}}}}}})
	for i := range change.Config.Rules {
		rule := &change.Config.Rules[i]
		if rule.Type == "inference" && rule.Subtype == "time" && rule.Scope.Service == "lastfm" {
			rule.Settings.Timezone = "UTC"
		}
	}
	state, err := s.Filters(ctx)
	require.NoError(t, err)
	priority := 1
	for _, template := range state.Blocks.Templates["matching:play"] {
		if template.Name == "game-scrobble" {
			change.Config.Rules = append(change.Config.Rules, Rule{ID: "game-scrobble", Type: "matching", Subtype: "play", Priority: &priority, Enabled: true, Program: template.Program})
		}
	}
	records, err := loadMatchingRecords(ctx, s.DB, change.Config)
	require.NoError(t, err)
	if sensitive {
		// Derive local acceptance aliases from actual case-only differences.
		// Private names stay in the local input rather than the checked-in test.
		aliases := map[string]bool{}
		priority := 1
		for _, lastfm := range records {
			if lastfm.Parser != LastfmCSV {
				continue
			}
			for _, fm := range records {
				if fm.Service != "yargfm" || !strings.EqualFold(fm.Values.Artist, lastfm.Values.Artist) || !strings.EqualFold(fm.Values.Title, lastfm.Values.Title) {
					continue
				}
				for field, values := range map[string][2]string{"artist": {fm.Values.Artist, lastfm.Values.Artist}, "title": {fm.Values.Title, lastfm.Values.Title}, "album": {fm.Values.Album, lastfm.Values.Album}} {
					if values[0] == "" || values[1] == "" || values[0] == values[1] || !strings.EqualFold(values[0], values[1]) {
						continue
					}
					key := field + ":" + lastfm.Values.Artist + ":" + values[1]
					if aliases[key] {
						continue
					}
					aliases[key] = true
					condition := operation("and", operation("eq", operation("casefold", fieldExpr("original.artist")), operation("casefold", literalExpr(lastfm.Values.Artist))), operation("eq", operation("casefold", fieldExpr("original."+field)), operation("casefold", literalExpr(values[1]))))
					if field != "album" {
						condition = operation("and", condition, operation("eq", operation("casefold", fieldExpr("original.title")), operation("casefold", literalExpr(lastfm.Values.Title))))
					}
					change.Config.Rules = append(change.Config.Rules, Rule{ID: fmt.Sprintf("case-%d", len(aliases)), Name: "Metadata capitalization", Type: "matching", Subtype: "music", Priority: &priority, Enabled: true, Program: assignWhen(condition, "music."+field, literalExpr(values[1]))})
				}
			}
		}
		require.Len(t, aliases, 2)
	}
	reasons := map[string]int{}
	for _, r := range records {
		if r.Service == "setlistfm" {
			reasons[r.Values.Reason]++
		}
	}
	t.Logf("Recovered concert record reasons: %v", reasons)
	matched, covered := 0, 0
	for _, fm := range records {
		if fm.Parser != NormalizedJSONL || fm.Service != "yargfm" {
			continue
		}
		var nearest *matchingRecord
		gap := math.MaxFloat64
		for i := range records {
			game := &records[i]
			if databaseFormat(game.Parser) != YARGDB || !strings.EqualFold(game.Values.Title, fm.Values.Title) || !strings.EqualFold(game.Values.Artist, fm.Values.Artist) || game.Start.IsZero() {
				continue
			}
			if delta := math.Abs(game.Start.Sub(fm.Start).Seconds()); delta < gap {
				nearest, gap = game, delta
			}
		}
		if nearest == nil || gap > 3600 {
			continue
		}
		matched++
		if nearest.GameCatalog != nil && nearest.GameCatalog.DurationMS != nil {
			covered++
			delay := nearest.Start.Sub(fm.Start) - time.Duration(*nearest.GameCatalog.DurationMS)*time.Millisecond
			t.Logf("YARGfm title=%q result minus scrobble=%s catalog duration=%dms remainder=%s", fm.Values.Title, nearest.Start.Sub(fm.Start), *nearest.GameCatalog.DurationMS, delay)
		}
	}
	t.Logf("Nearby native YARG/YARGfm title-and-artist pairs: %d; known catalog durations: %d", matched, covered)
	require.Greater(t, matched, 0)
	_, err = s.Save(ctx, change)
	require.NoError(t, err)
	var concerts, games int
	require.NoError(t, s.DB.QueryRow(`SELECT COUNT(*) FROM source_play_context c JOIN source_plays p ON p.play_key=c.play_key WHERE json_extract(c.context_json,'$.kind')='concert'`).Scan(&concerts))
	require.NoError(t, s.DB.QueryRow(`SELECT COUNT(*) FROM source_play_context c JOIN source_plays p ON p.play_key=c.play_key WHERE json_extract(c.context_json,'$.kind')='gameplay'`).Scan(&games))
	require.Equal(t, 1059, concerts)
	require.Equal(t, 73, reasons["unresolved concert date"])
	var fmMatched int
	require.NoError(t, s.DB.QueryRow(`SELECT COUNT(*) FROM source_play_records f JOIN source_records r ON r.id=f.record_id JOIN source_accounts s ON s.id=r.source_id WHERE s.service='yargfm' AND r.parser=? AND EXISTS(SELECT 1 FROM source_play_records g JOIN source_records gr ON gr.id=g.record_id WHERE g.play_key=f.play_key AND gr.parser=?)`, NormalizedJSONL, YARGDB).Scan(&fmMatched))
	if fmMatched != 29 {
		rows, err := s.DB.Query(`SELECT r.id,r.payload,i.reason FROM source_records r JOIN source_accounts a ON a.id=r.source_id LEFT JOIN source_interpretations i ON i.record_id=r.id WHERE a.service='yargfm' AND NOT EXISTS(SELECT 1 FROM source_play_records f JOIN source_play_records g ON g.play_key=f.play_key JOIN source_records gr ON gr.id=g.record_id WHERE f.record_id=r.id AND gr.parser=?)`, YARGDB)
		require.NoError(t, err)
		for rows.Next() {
			var id int64
			var payload, reason string
			require.NoError(t, rows.Scan(&id, &payload, &reason))
			v := normalizeRecord([]byte(payload), NormalizedJSONL, "")
			t.Logf("Unmatched YARGfm record %d: %s / %s / %s: %s", id, v.Artist, v.Title, v.Album, reason)
		}
		rows.Close()
	}
	require.Equal(t, 29, fmMatched)
	require.Equal(t, 541, games)
	var lastfmMatched int
	require.NoError(t, s.DB.QueryRow(`SELECT COUNT(*) FROM source_play_records f JOIN source_records r ON r.id=f.record_id WHERE r.parser=? AND EXISTS(SELECT 1 FROM source_play_records g JOIN source_records gr ON gr.id=g.record_id WHERE g.play_key=f.play_key AND gr.parser=?)`, LastfmCSV, YARGDB).Scan(&lastfmMatched))
	if lastfmMatched != 26 {
		for _, r := range records {
			if r.Parser != LastfmCSV {
				continue
			}
			var linked int
			require.NoError(t, s.DB.QueryRow(`SELECT COUNT(*) FROM source_play_records f JOIN source_play_records g ON g.play_key=f.play_key JOIN source_records gr ON gr.id=g.record_id WHERE f.record_id=? AND gr.parser=?`, r.ID, YARGDB).Scan(&linked))
			if linked == 0 {
				t.Logf("Unmatched Last.fm: %q / %q / %q", r.Values.Artist, r.Values.Title, r.Values.Album)
				for _, game := range records {
					if game.Parser == YARGDB && strings.EqualFold(game.Values.Title, r.Values.Title) && strings.EqualFold(game.Values.Artist, r.Values.Artist) {
						t.Logf("Native metadata: %q / %q / %q", game.Values.Artist, game.Values.Title, game.Values.Album)
					}
				}
			}
		}
	}
	require.Equal(t, 26, lastfmMatched)
	require.Equal(t, 1600, count(t, s, "source_plays"))
	t.Logf("Originals and repeats verified. Concert plays: %d; gameplay plays: %d; YARGfm records matched to native games: %d", concerts, games, fmMatched)
}

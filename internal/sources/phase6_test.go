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

func TestNavidromeCountCatalogAndGenres(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	data := sqliteFixture(t,
		`CREATE TABLE media_file(id TEXT PRIMARY KEY,title TEXT,artist TEXT,album TEXT,genre TEXT)`,
		`CREATE TABLE annotation(user_id TEXT,item_id TEXT,item_type TEXT,play_count INTEGER,play_date TEXT)`,
		`CREATE TABLE scrobbles(id INTEGER PRIMARY KEY,media_file_id TEXT,user_id TEXT,submission_time INTEGER)`,
		`INSERT INTO media_file VALUES('track','Song','Artist','Album','Folk, Rock'),('unplayed','Unplayed','Artist','Album','Folk')`,
		`INSERT INTO annotation VALUES('owner','track','media_file',3,'2020-01-02 03:04:05.000+00:00'),('owner','album','album',3,'2020-01-02 03:04:05.000+00:00')`,
		`INSERT INTO scrobbles VALUES(1,'track','owner',1577934245)`)
	_, err := s.Import(ctx, bytes.NewReader(data), "navidrome.db", 1)
	require.NoError(t, err)
	_, err = s.Rebuild(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, count(t, s, "listens"))
	require.Equal(t, 2, count(t, s, "source_music_states"))
	require.Equal(t, 2, count(t, s, "source_genres"))
	require.Zero(t, count(t, s, "source_aggregates"))
	libraries, err := s.Libraries(ctx)
	require.NoError(t, err)
	require.Len(t, libraries, 1)
	require.EqualValues(t, 3, libraries[0].Reported)
	saveTestRules(t, s, Rule{ID: "nav-count", Type: "inference", Subtype: "count", Enabled: true, Program: countProgram("maximum", RuleScope{Service: "navidrome"})})
	aggregate := readAggregate(t, s)
	require.EqualValues(t, 3, aggregate.Attributed)
	require.EqualValues(t, 1, aggregate.Observed)
	require.EqualValues(t, 2, aggregate.Undated)
	_, err = s.Rebuild(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 2, readAggregate(t, s).Undated)
	require.Equal(t, 1, count(t, s, "listens"))
}

func TestMultiCacheAndLogEvidence(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	play := map[string]any{"data": map[string]any{"track": "Song", "artists": []any{map[string]any{"name": "Artist"}}, "playDate": "2020-01-02T03:04:05Z"}, "meta": map[string]any{"source": "Spotify", "deviceId": "device", "extra": "kept"}}
	entry := map[string]any{"value": []any{map[string]any{"id": "delivery", "play": play, "retries": 3}}, "$types": map[string]any{"value": "arrayNonindexKeys", "value.0.play.data.playDate": "Dayjs"}}
	encoded, _ := json.Marshal(entry)
	pool := []any{[]any{"1", "2"}, map[string]any{"key": "3", "value": "4"}, map[string]any{"key": "5", "value": "4"}, "keyv:lastfm-Last.fm-dead", string(encoded), "keyv:koito-Koito-dead"}
	data, _ := json.Marshal(pool)
	first, err := s.Import(ctx, bytes.NewReader(data), "ms-scrobble.cache", 1)
	require.NoError(t, err)
	repeat, err := s.Import(ctx, bytes.NewReader(data), "ms-scrobble.cache", 1)
	require.NoError(t, err)
	require.Equal(t, first.Records, repeat.Reused)
	_, err = s.Rebuild(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, count(t, s, "listens"), "both endpoints share one source event")
	_, err = s.Import(ctx, bytes.NewBufferString("[2020-01-02 03:04:06.000 -0700] INFO : [App] [Play root] Discovered => Song\n"), "scrobble-2020-01-02.log", 1)
	require.NoError(t, err)
	_, err = s.Import(ctx, bytes.NewReader(nil), "scrobble-empty.log", 1)
	require.NoError(t, err)
	_, err = s.Rebuild(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, count(t, s, "listens"))
	entry["$types"] = map[string]any{"value": "FutureType"}
	encoded, _ = json.Marshal(entry)
	pool[4] = string(encoded)
	data, _ = json.Marshal(pool)
	_, err = s.Import(ctx, bytes.NewReader(data), "future.cache", 1)
	require.NoError(t, err)
	_, err = s.Rebuild(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, count(t, s, "listens"), "unknown serialized types stay preserved")
}

func TestLocalPhase6MultiHistory(t *testing.T) {
	input := os.Getenv("KOITO_TEST_PHASE6_MULTI_HISTORY")
	if input == "" {
		t.Skip("set KOITO_TEST_PHASE6_MULTI_HISTORY to the Multi-Scrobbler config directory")
	}
	s := testStore(t)
	ctx := context.Background()
	paths := []string{filepath.Join(input, "ms-scrobble.cache")}
	logs, err := filepath.Glob(filepath.Join(input, "logs", "*.log"))
	require.NoError(t, err)
	paths = append(paths, logs...)
	for _, path := range paths {
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		first, err := s.Import(ctx, bytes.NewReader(data), filepath.Base(path), 1)
		require.NoError(t, err)
		again, err := s.Import(ctx, bytes.NewReader(data), filepath.Base(path), 1)
		require.NoError(t, err)
		require.Equal(t, first.Records, again.Reused)
	}
	rows, err := s.DB.Query(`SELECT parser,payload FROM source_records WHERE parser=?`, MultiCache)
	require.NoError(t, err)
	reasons := map[string]int{}
	for rows.Next() {
		var parser, payload string
		require.NoError(t, rows.Scan(&parser, &payload))
		reasons[normalizeRecord([]byte(payload), parser, "").Reason]++
	}
	require.NoError(t, rows.Err())
	rows.Close()
	t.Logf("Cache interpretation: %v", reasons)
	result, err := s.Rebuild(ctx)
	require.NoError(t, err)
	t.Logf("%d files preserved %d records and contributed %d plays", len(paths), result.Records, result.Plays)
}

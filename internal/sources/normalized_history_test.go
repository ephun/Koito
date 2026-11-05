package sources

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNormalizedHistoryUsesOriginalsAndRetainsDerivedRows(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	mustImport(t, s, []byte("["+complete+"]"), "original.json", "")
	// Use an SQL string literal only for this fixed, anonymized fixture.
	data := sqliteFixture(t, `CREATE TABLE raw_play_events(id INTEGER PRIMARY KEY,source_type TEXT,played_at TEXT,title_raw TEXT,artist_raw TEXT,payload_json TEXT)`,
		`INSERT INTO raw_play_events VALUES(1,'spotify_json','2099-01-01T00:00:00Z','Changed','Changed','`+complete+`')`)
	_, err := s.Import(ctx, bytes.NewReader(data), "converted.sqlite", 1)
	require.NoError(t, err)
	converted := `{"source_type":"lastfm_csv","played_at":"2099-01-01T00:00:00Z","title_raw":"Changed","artist_raw":"Changed","payload_json":{"Artist":"Artist","Album":"Album","Title":"Another","Timestamp":"1/2/2020 04:01","extra":true}}`
	_, err = s.Import(ctx, bytes.NewBufferString(converted+"\n"), "converted.jsonl", 1)
	require.NoError(t, err)
	derived := `{"source_type":"itunes_json","played_at":"2004-01-01T00:00:00Z","title_raw":"Estimated","artist_raw":"Artist","payload_json":{"Name":"Estimated","Artist":"Artist","Persistent ID":"A","Play Count":"100","Estimated Play Time":"2004-01-01T00:00:00Z"}}`
	_, err = s.Import(ctx, bytes.NewBufferString(derived), "expanded.jsonl", 1)
	require.NoError(t, err)
	change := currentChange(t, s)
	change.Config.Rules[2].Settings.Timezone = "UTC"
	_, err = s.Save(ctx, change)
	require.NoError(t, err)
	result, err := s.Rebuild(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, result.Plays)
	require.Equal(t, 3, count(t, s, "source_play_records"))
	inspection, err := s.Inspect(ctx, 4)
	require.NoError(t, err)
	require.Equal(t, "Another", inspection.Record.Values.Title)
	require.Equal(t, int64(time.Minute), inspection.Record.Values.Precision)
	inspection, err = s.Inspect(ctx, 5)
	require.NoError(t, err)
	require.Equal(t, "derived", inspection.Record.Values.Origin)
	require.Equal(t, "legacy derived history", inspection.Record.Values.Reason)
	require.Zero(t, count(t, s, "source_aggregates"))
}

func TestKoitoDatabaseSnapshotIdentityAndDerivedHistory(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	base := []string{
		`CREATE TABLE tracks(id INTEGER PRIMARY KEY,release_id INTEGER)`,
		`CREATE TABLE track_aliases(track_id INTEGER,alias TEXT,is_primary INTEGER)`,
		`CREATE TABLE release_aliases(release_id INTEGER,alias TEXT,is_primary INTEGER)`,
		`CREATE TABLE artist_aliases(artist_id INTEGER,alias TEXT,is_primary INTEGER)`,
		`CREATE TABLE artist_tracks(artist_id INTEGER,track_id INTEGER,is_primary INTEGER)`,
		`CREATE TABLE listens(track_id INTEGER,listened_at INTEGER,user_id INTEGER,client TEXT,PRIMARY KEY(track_id,listened_at,user_id))`,
		`INSERT INTO tracks VALUES(1,1)`, `INSERT INTO track_aliases VALUES(1,'Song',1)`,
		`INSERT INTO release_aliases VALUES(1,'Album',1)`, `INSERT INTO artist_aliases VALUES(1,'Artist',1)`,
		`INSERT INTO artist_tracks VALUES(1,1,1)`,
		`INSERT INTO listens VALUES(1,1577934245,1,'lastfm'),(1,1577934445,1,'itunes')`,
	}
	for _, extra := range []string{`CREATE TABLE extra(value TEXT)`, `CREATE TABLE extra(value TEXT); INSERT INTO extra VALUES('changed snapshot')`} {
		data := sqliteFixture(t, append(append([]string{}, base...), extra)...)
		_, err := s.Import(ctx, bytes.NewReader(data), "koito.db", 1)
		require.NoError(t, err)
	}
	result, err := s.Rebuild(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, result.Plays)
	require.Equal(t, 2, count(t, s, "source_play_records"))
	var payload string
	require.NoError(t, s.DB.QueryRow(`SELECT payload FROM source_records WHERE kind='native_play' ORDER BY id LIMIT 1`).Scan(&payload))
	v := normalizeDatabase([]byte(payload), KoitoDB, "")
	require.Equal(t, "unknown", v.TimeMeaning)
	require.Equal(t, "converted", v.Origin)
	_, err = s.Rebuild(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, count(t, s, "listens"))
}

func TestSQLiteUnsupportedSchemaAndEmptyBackup(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	bad := sqliteFixture(t, `CREATE TABLE plays(id INTEGER PRIMARY KEY,play TEXT)`, `CREATE TABLE components(id INTEGER)`, `CREATE TABLE play_inputs(id INTEGER)`)
	result, err := s.Import(ctx, bytes.NewReader(bad), "unsupported.db", 1)
	require.ErrorContains(t, err, "Unsupported database schema")
	path, _ := s.FilePath(result.Hash)
	stored, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, bad, stored)
	_, err = s.Import(ctx, bytes.NewReader(nil), "ms.db.123.bak", 1)
	require.NoError(t, err)
	_, err = s.Rebuild(ctx)
	require.NoError(t, err)
	require.Zero(t, count(t, s, "listens"))
}

func TestLegacyLedgerDistinguishesOriginalAndDerivedPayloads(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	data := sqliteFixture(t, `CREATE TABLE source_account(id TEXT PRIMARY KEY,service TEXT)`,
		`CREATE TABLE observation(id TEXT PRIMARY KEY,source_service TEXT,payload TEXT,title_raw TEXT,artist_raw TEXT)`,
		`CREATE TABLE aggregate_evidence(id TEXT PRIMARY KEY,count INTEGER,payload TEXT)`,
		`INSERT INTO observation VALUES('original','spotify','`+complete+`','Changed','Changed'),('derived','itunes','{"title":"Song","listened_at":1577934245}','Song','Artist')`,
		`INSERT INTO aggregate_evidence VALUES('count',100,'{"Name":"Song","Play Count":100}')`)
	_, err := s.Import(ctx, bytes.NewReader(data), "ledger.sqlite", 1)
	require.NoError(t, err)
	result, err := s.Rebuild(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, result.Plays)
	var payload string
	require.NoError(t, s.DB.QueryRow(`SELECT payload FROM source_records WHERE kind='legacy_record' AND payload LIKE '%"key":"[\"derived\"]"%'`).Scan(&payload))
	var row databaseRecord
	require.NoError(t, json.Unmarshal([]byte(payload), &row))
	require.Equal(t, "derived", normalizeDatabase([]byte(payload), LedgerDB, "").Origin)
	require.Zero(t, count(t, s, "source_aggregates"))
}

func TestRetainedPayloadDeviceAndUnknownForwardingSource(t *testing.T) {
	payload := strings.Replace(complete, `"unknown":`, `"platform":"source platform","unknown":`, 1)
	row := map[string]any{"source_type": "spotify_json", "payload_json": payload}
	require.Equal(t, "source platform", normalizeHistoryRow(row, "UTC").Device)
	play := map[string]any{"data": map[string]any{"track": "Song", "artists": []any{"Artist"}, "playDate": "2020-01-02T03:04:05Z"}, "meta": map[string]any{"deviceId": "device"}}
	bytes, err := json.Marshal(databaseRecord{Kind: "play", Table: "plays", Fields: map[string]any{"play": play}, Context: map[string]any{"component": map[string]any{"mode": "source", "type": "endpointlz", "name": "Input"}}})
	require.NoError(t, err)
	v := normalizeDatabase(bytes, MultiDB, "")
	require.Empty(t, v.OriginalService, "an input protocol does not establish an original service")
	require.Empty(t, v.Endpoint)
	require.Equal(t, "multi-scrobbler", v.Forwarder)
	require.Equal(t, "device", v.Device)
}

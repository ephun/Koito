package sources

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func sqliteFixture(t *testing.T, statements ...string) []byte {
	t.Helper()
	path := filepath.Join(t.TempDir(), "input.db")
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	for _, query := range statements {
		_, err = db.Exec(query)
		require.NoError(t, err)
	}
	require.NoError(t, db.Close())
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return data
}

func TestSQLiteSourceAndForwarding(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	play := `{"data":{"track":"Song","artists":[{"name":"Artist"}],"album":"Album","playDate":"2020-01-02T03:04:05Z","playDateCompleted":"2020-01-02T03:07:05Z"},"meta":{"musicService":"Spotify","deviceId":"Player"}}`
	data := sqliteFixture(t,
		`CREATE TABLE components(id INTEGER PRIMARY KEY,mode TEXT,type TEXT,name TEXT)`,
		`INSERT INTO components VALUES(1,'source','spotify','Spotify'),(2,'client','lastfm','Last.fm'),(3,'client','koito','Koito')`,
		`CREATE TABLE plays(id INTEGER PRIMARY KEY,uid TEXT,componentId INTEGER,parentId INTEGER,play TEXT,state TEXT,extra TEXT)`,
		`CREATE TABLE play_inputs(id INTEGER PRIMARY KEY,playId INTEGER,data TEXT,play TEXT)`,
		`CREATE TABLE play_events(id INTEGER PRIMARY KEY,playId INTEGER,eventName TEXT,data TEXT)`)
	path := filepath.Join(t.TempDir(), "multi.db")
	require.NoError(t, os.WriteFile(path, data, 0600))
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	for _, row := range []struct {
		id        int
		uid       string
		component int
		parent    any
		state     string
	}{{1, "root", 1, nil, "discovered"}, {2, "lastfm-copy", 2, 1, "scrobbled"}, {3, "koito-copy", 3, 1, "scrobbled"}} {
		_, err = db.Exec(`INSERT INTO plays VALUES(?,?,?,?,?,?,?)`, row.id, row.uid, row.component, row.parent, play, row.state, "keep")
		require.NoError(t, err)
	}
	_, err = db.Exec(`INSERT INTO play_inputs VALUES(1,1,'{"unknown":true}',?)`, play)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO play_events VALUES(1,2,'scrobbleResult','{"success":true}'),(2,3,'queueStateChange','{"retries":2}')`)
	require.NoError(t, err)
	require.NoError(t, db.Close())
	data, err = os.ReadFile(path)
	require.NoError(t, err)
	first, err := s.Import(ctx, bytes.NewReader(data), "ms.db", 1)
	require.NoError(t, err)
	repeat, err := s.Import(ctx, bytes.NewReader(data), "renamed.db", 1)
	require.NoError(t, err)
	require.Zero(t, repeat.Added)
	require.Equal(t, first.Records, repeat.Reused)
	result, err := s.Rebuild(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, result.Plays)
	require.Equal(t, 6, count(t, s, "source_play_records"), "one source, two copies, one input, and two delivery events")
	var payload string
	require.NoError(t, s.DB.QueryRow(`SELECT payload FROM source_records WHERE kind='play'`).Scan(&payload))
	require.Contains(t, payload, `"extra":"keep"`)
	stored, _ := s.FilePath(first.Hash)
	original, err := os.ReadFile(stored)
	require.NoError(t, err)
	require.True(t, bytes.Equal(data, original))
}

func TestSQLiteWALBundle(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "navidrome.db")
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	defer db.Close()
	db.SetMaxOpenConns(1)
	for _, q := range []string{`PRAGMA journal_mode=WAL`, `PRAGMA wal_autocheckpoint=0`, `CREATE TABLE media_file(id TEXT PRIMARY KEY,title TEXT,artist TEXT,album TEXT,genre TEXT)`, `CREATE TABLE annotation(user_id TEXT,item_id TEXT,item_type TEXT,play_count INTEGER,play_date TEXT)`, `CREATE TABLE scrobbles(id INTEGER PRIMARY KEY,media_file_id TEXT,user_id TEXT,submission_time INTEGER)`, `PRAGMA wal_checkpoint(TRUNCATE)`, `INSERT INTO media_file VALUES('track','Song','Artist','Album','Rock')`, `INSERT INTO scrobbles VALUES(1,'track','owner',1577934245)`} {
		_, err = db.Exec(q)
		require.NoError(t, err)
	}
	main, err := os.ReadFile(path)
	require.NoError(t, err)
	wal, err := os.ReadFile(path + "-wal")
	require.NoError(t, err)
	require.Greater(t, len(wal), 32)
	var archive bytes.Buffer
	z := zip.NewWriter(&archive)
	// Deliberately put the sidecar first to verify order-independent pairing.
	for _, item := range []struct {
		name  string
		bytes []byte
	}{{"database/navidrome.db-wal", wal}, {"database/navidrome.db", main}} {
		w, e := z.Create(item.name)
		require.NoError(t, e)
		_, e = w.Write(item.bytes)
		require.NoError(t, e)
	}
	require.NoError(t, z.Close())
	_, err = s.Import(ctx, bytes.NewReader(archive.Bytes()), "snapshot.zip", 1)
	require.NoError(t, err)
	result, err := s.Rebuild(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, result.Plays)
	var raw string
	require.NoError(t, s.DB.QueryRow(`SELECT payload FROM source_records WHERE kind='play'`).Scan(&raw))
	var record databaseRecord
	require.NoError(t, json.Unmarshal([]byte(raw), &record))
	require.Equal(t, "submission", normalizeDatabase([]byte(raw), NavidromeDB, "").TimeMeaning)
	_, err = s.Import(ctx, bytes.NewReader(archive.Bytes()), "again.zip", 1)
	require.NoError(t, err)
	require.Equal(t, result.Records, count(t, s, "source_records"))
	_, err = db.Exec(`INSERT INTO scrobbles VALUES(2,'track','owner',1577934345)`)
	require.NoError(t, err)
	changed, err := os.ReadFile(path + "-wal")
	require.NoError(t, err)
	bundle := func(sidecar []byte) []byte {
		var buf bytes.Buffer
		writer := zip.NewWriter(&buf)
		for _, item := range []struct {
			name string
			data []byte
		}{{"navidrome.db", main}, {"navidrome.db-wal", sidecar}} {
			member, e := writer.Create(item.name)
			require.NoError(t, e)
			_, e = member.Write(item.data)
			require.NoError(t, e)
		}
		require.NoError(t, writer.Close())
		return buf.Bytes()
	}
	broken := bytes.Clone(changed)
	broken[len(broken)-1] ^= 1
	_, err = s.Import(ctx, bytes.NewReader(bundle(broken)), "damaged.zip", 1)
	require.ErrorContains(t, err, "checksum")
	require.Equal(t, 1, count(t, s, "listens"), "failed extraction preserves the published view")
	_, err = s.Import(ctx, bytes.NewReader(bundle(changed)), "new-snapshot.zip", 1)
	require.NoError(t, err)
	result, err = s.Rebuild(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, result.Plays, "changed WAL reuses the old event and adds the new event")
	_, err = s.Import(ctx, bytes.NewReader(bundle(changed)), "same-snapshot.zip", 1)
	require.NoError(t, err)
	result, err = s.Rebuild(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, result.Plays)
}

func TestLocalPhase6SQLite(t *testing.T) {
	input := os.Getenv("KOITO_TEST_PHASE6_SQLITE")
	if input == "" {
		t.Skip("set KOITO_TEST_PHASE6_SQLITE to a JSON list of database paths")
	}
	var paths []string
	require.NoError(t, json.Unmarshal([]byte(input), &paths))
	s := testStore(t)
	ctx := context.Background()
	for _, path := range paths {
		file, err := os.Open(path)
		require.NoError(t, err)
		first, err := s.Import(ctx, file, filepath.Base(path), 1)
		file.Close()
		require.NoError(t, err)
		file, err = os.Open(path)
		require.NoError(t, err)
		second, err := s.Import(ctx, file, filepath.Base(path), 1)
		file.Close()
		require.NoError(t, err)
		require.Zero(t, second.Added)
		require.Equal(t, first.Records, second.Reused)
		t.Logf("%s: %d preserved records, repeated import reused every record", filepath.Base(path), first.Records)
	}
	result, err := s.Rebuild(ctx)
	require.NoError(t, err)
	t.Logf("Rebuilt %d plays from %d records", result.Plays, result.Records)
}

package sources

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	migrations "github.com/gabehf/koito/db/migrations_sqlite"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "test.db")+"?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	goose.SetBaseFS(migrations.Files)
	if err = goose.SetDialect("sqlite3"); err != nil {
		t.Fatal(err)
	}
	if err = goose.Up(db, "."); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO users(id,username,role,password) VALUES(1,'test','admin',x'00')`); err != nil {
		t.Fatal(err)
	}
	return &Store{DB: db, Root: t.TempDir()}
}

const complete = `{"ts":"2020-01-02T03:04:05Z","ms_played":180001,"master_metadata_track_name":"Song","master_metadata_album_artist_name":"Artist","master_metadata_album_album_name":"Album","spotify_track_uri":"spotify:track:one","reason_end":"trackdone","unknown":{"keep":true}}`
const skipped = `{"ts":"2020-01-02T04:04:05Z","ms_played":120,"master_metadata_track_name":"Song","master_metadata_album_artist_name":"Artist","reason_end":"fwdbtn"}`
const podcast = `{"ts":"2020-01-02T05:04:05Z","ms_played":123,"episode_name":"Episode","reason_end":"trackdone"}`

func count(t *testing.T, s *Store, table string) int {
	t.Helper()
	var n int
	if err := s.DB.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
func mustImport(t *testing.T, s *Store, data []byte, name, account string) ImportResult {
	t.Helper()
	v, err := s.ImportSpotify(context.Background(), bytes.NewReader(data), name, account, 1)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestPreservationRepeatAndRebuild(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	data := []byte("[" + complete + "," + complete + "," + skipped + "," + podcast + "]")
	first := mustImport(t, s, data, "history.json", "me")
	for i := 0; i < 100; i++ {
		r := mustImport(t, s, data, "renamed.json", "me")
		if r.Added != 0 || r.Reused != 4 {
			t.Fatalf("repeat multiplied records: %+v", r)
		}
	}
	if count(t, s, "source_records") != 4 {
		t.Fatal("lost or multiplied record occurrences")
	}
	path, _ := s.FilePath(first.Hash)
	stored, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(data, stored) {
		t.Fatal("original bytes changed", err)
	}
	records, err := s.Records(ctx, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(records[0].Payload, `"unknown":{"keep":true}`) {
		t.Fatal("unknown field lost")
	}
	for i := 0; i < 3; i++ {
		p, err := s.Rebuild(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if p.Records != 4 || p.Plays != 1 || p.Collisions != 0 {
			t.Fatalf("unexpected projection %+v", p)
		}
		if count(t, s, "listens") != 1 {
			t.Fatal("rebuild multiplied plays")
		}
	}
	var duration int
	if err = s.DB.QueryRow("SELECT duration FROM tracks").Scan(&duration); err != nil {
		t.Fatal(err)
	}
	if duration != 0 {
		t.Fatal("played duration was misrepresented as track duration")
	}
	if _, err = s.DB.Exec("UPDATE source_records SET kind='changed'"); err == nil {
		t.Fatal("raw evidence can be changed")
	}
	if _, err = s.DB.Exec("DELETE FROM source_records"); err == nil {
		t.Fatal("raw evidence can be deleted")
	}
}

func TestZipStandaloneAndOverlappingExports(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	data := []byte("[" + complete + "]")
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	member, err := z.Create("Spotify Extended Streaming History/Streaming_History_Audio_2020.json")
	if err != nil {
		t.Fatal(err)
	}
	member.Write(data)
	if err = z.Close(); err != nil {
		t.Fatal(err)
	}
	mustImport(t, s, buf.Bytes(), "archive.zip", "me")
	r := mustImport(t, s, data, "history.json", "me")
	if r.Added != 0 || r.Reused != 1 {
		t.Fatalf("standalone duplicated member %+v", r)
	}
	mustImport(t, s, []byte("["+complete+","+skipped+"]"), "overlap.json", "me")
	if count(t, s, "source_records") != 3 {
		t.Fatal("different files must preserve their own occurrences")
	}
	p, err := s.Rebuild(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if p.Plays != 1 {
		t.Fatal("overlapping exports multiplied plays", p)
	}
	mustImport(t, s, data, "history.json", "other-account")
	p, err = s.Rebuild(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if p.Plays != 2 || p.Collisions != 1 {
		t.Fatal("separate accounts must not match; legacy key collision must be explicit", p)
	}
}

func TestFailedImportKeepsFileAndCanRetry(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	bad := []byte("[" + complete + ",42]")
	result, err := s.ImportSpotify(ctx, bytes.NewReader(bad), "broken.json", "me", 1)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if count(t, s, "source_records") != 0 {
		t.Fatal("partial records committed")
	}
	receipts, err := s.Receipts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(receipts) != 1 || receipts[0].Status != "failed" {
		t.Fatal("failure receipt missing", receipts)
	}
	p, _ := s.FilePath(result.Hash)
	stored, err := os.ReadFile(p)
	if err != nil || !bytes.Equal(stored, bad) {
		t.Fatal("failed original not preserved")
	}
	mustImport(t, s, []byte("["+complete+"]"), "fixed.json", "me")
	if count(t, s, "source_records") != 1 {
		t.Fatal("corrected upload failed")
	}
}

func TestLegacyListenAndAliasesSurviveRebuild(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	mustImport(t, s, []byte("["+complete+"]"), "history.json", "me")
	if _, err := s.Rebuild(ctx); err != nil {
		t.Fatal(err)
	}
	// Represent a pre-existing legacy row at the same key, outside projection ownership.
	if _, err := s.DB.Exec(`UPDATE listens SET client='legacy'; DELETE FROM source_projection; INSERT INTO track_aliases(track_id,alias,source,is_primary) SELECT id,'Custom title','user',0 FROM tracks`); err != nil {
		t.Fatal(err)
	}
	p, err := s.Rebuild(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if p.Collisions != 1 {
		t.Fatal("legacy overlap not reported")
	}
	if _, err = s.Rebuild(ctx); err != nil {
		t.Fatal(err)
	}
	var client string
	if err = s.DB.QueryRow("SELECT client FROM listens").Scan(&client); err != nil || client != "legacy" {
		t.Fatal("legacy listen was changed", err)
	}
	var n int
	if err = s.DB.QueryRow("SELECT COUNT(*) FROM track_aliases WHERE alias='Custom title'").Scan(&n); err != nil || n != 1 {
		t.Fatal("legacy alias lost", err)
	}
}

// Run explicitly against a local extended history JSON; private data never
// becomes a fixture or repository artifact.
func TestLocalSpotifyArchive(t *testing.T) {
	path := os.Getenv("KOITO_TEST_SPOTIFY_FILE")
	if path == "" {
		t.Skip("set KOITO_TEST_SPOTIFY_FILE for local acceptance")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := testStore(t)
	ctx := context.Background()
	first := mustImport(t, s, data, "history.json", "local-acceptance")
	second := mustImport(t, s, data, "renamed.json", "local-acceptance")
	if first.Added == 0 || second.Added != 0 || second.Reused != first.Added {
		t.Fatal("real-file repeat mismatch", first, second)
	}
	before, err := s.Rebuild(ctx)
	if err != nil {
		t.Fatal(err)
	}
	after, err := s.Rebuild(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("real-file rebuild mismatch", before, after)
	}
	t.Logf("records=%d plays=%d collisions=%d", before.Records, before.Plays, before.Collisions)
}

func TestAdditiveMigrationPreservesExistingCatalog(t *testing.T) {
	conn, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "legacy.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	goose.SetBaseFS(migrations.Files)
	if err = goose.SetDialect("sqlite3"); err != nil {
		t.Fatal(err)
	}
	if err = goose.UpTo(conn, ".", 2); err != nil {
		t.Fatal(err)
	}
	_, err = conn.Exec(`INSERT INTO users VALUES(1,'legacy','admin',x'00');
 INSERT INTO artists(id) VALUES(1);
 INSERT INTO artist_aliases VALUES(1,'My artist','user',1);
 INSERT INTO releases(id) VALUES(1);
 INSERT INTO release_aliases VALUES(1,'My album','user',1);
 INSERT INTO artist_releases VALUES(1,1,1);
 INSERT INTO tracks(id,release_id,duration) VALUES(1,1,123);
 INSERT INTO track_aliases VALUES(1,'My title','user',1);
 INSERT INTO artist_tracks VALUES(1,1,1);
 INSERT INTO listens VALUES(1,1234567890,1,'legacy');`)
	if err != nil {
		t.Fatal(err)
	}
	if err = goose.Up(conn, "."); err != nil {
		t.Fatal(err)
	}
	var title, client string
	var duration int
	err = conn.QueryRow(`SELECT t.title,t.duration,l.client FROM tracks_with_title t JOIN listens l ON l.track_id=t.id`).Scan(&title, &duration, &client)
	if err != nil || title != "My title" || duration != 123 || client != "legacy" {
		t.Fatal("migration changed legacy data", title, duration, client, err)
	}
}

func TestZipValidationIsAtomic(t *testing.T) {
	s := testStore(t)
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	for name, data := range map[string]string{"Streaming_History_Audio_good.json": "[" + complete + "]", "Streaming_History_Audio_bad.json": "not JSON"} {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(data))
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	_, err := s.ImportSpotify(context.Background(), bytes.NewReader(buf.Bytes()), "broken.zip", "me", 1)
	if err == nil {
		t.Fatal("malformed ZIP member was accepted")
	}
	if count(t, s, "source_records") != 0 || count(t, s, "source_extractions") != 0 {
		t.Fatal("failed ZIP partially committed records")
	}
}

func TestFailedProjectionPreservesPreviousView(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	mustImport(t, s, []byte("["+complete+"]"), "history.json", "me")
	before, err := s.Rebuild(ctx)
	if err != nil {
		t.Fatal(err)
	}
	mustImport(t, s, []byte("["+skipped+"]"), "more.json", "me")
	_, err = s.DB.Exec(`CREATE TRIGGER reject_projection BEFORE INSERT ON listens BEGIN SELECT RAISE(ABORT,'simulated failure'); END`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Rebuild(ctx); err == nil {
		t.Fatal("expected projection failure")
	}
	if count(t, s, "listens") != 1 || count(t, s, "source_interpretations") != 1 || count(t, s, "source_records") != 2 {
		t.Fatal("failure damaged view or evidence")
	}
	if _, err = s.DB.Exec("DROP TRIGGER reject_projection"); err != nil {
		t.Fatal(err)
	}
	after, err := s.Rebuild(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if after.Plays != before.Plays || after.Records != 2 {
		t.Fatal("rebuild could not recover", after)
	}
}

func TestImportDetectsContentAndRejectsUnsupportedFiles(t *testing.T) {
	s := testStore(t)
	data := []byte("[" + complete + "]")
	mustImport(t, s, data, "history.data", "me")
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	member, err := z.Create("Streaming_History_Audio_2020.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = member.Write(data); err != nil {
		t.Fatal(err)
	}
	if err = z.Close(); err != nil {
		t.Fatal(err)
	}
	result := mustImport(t, s, buf.Bytes(), "archive.data", "me")
	if result.Added != 0 || result.Reused != 1 {
		t.Fatal("renamed ZIP was not detected", result)
	}
	for _, input := range []string{"%PDF-1.7 unsupported", `[{"artist":"Someone","track":"Something"}]`} {
		_, err = s.ImportSpotify(context.Background(), strings.NewReader(input), "upload.json", "me", 1)
		if err == nil || err.Error() != "Unsupported file." {
			t.Fatal("unsupported upload did not get a direct error", err)
		}
	}
	if count(t, s, "source_records") != 1 {
		t.Fatal("unsupported upload created records")
	}
}

package sources

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMultiScrobblerMissingAndCyclicParents(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	play := `{"data":{"track":"Song","artists":[{"name":"Artist"}],"playDate":"2020-01-02T03:04:05Z"},"meta":{"source":"Spotify"}}`
	data := sqliteFixture(t, `CREATE TABLE components(id INTEGER PRIMARY KEY,mode TEXT,type TEXT,name TEXT)`,
		`CREATE TABLE plays(id INTEGER PRIMARY KEY,uid TEXT,componentId INTEGER,parentId INTEGER,play TEXT,state TEXT)`,
		`CREATE TABLE play_inputs(id INTEGER PRIMARY KEY,playId INTEGER,data TEXT,play TEXT)`,
		`INSERT INTO components VALUES(1,'source','spotify','Spotify'),(2,'client','lastfm','Last.fm')`,
		`INSERT INTO plays VALUES(1,'source',1,NULL,'`+play+`','discovered'),(2,'orphan',2,99,'`+play+`','failed'),(3,'cycle-a',2,4,'`+play+`','failed'),(4,'cycle-b',2,3,'`+play+`','failed')`)
	_, err := s.Import(ctx, bytes.NewReader(data), "ms.db", 1)
	require.NoError(t, err)
	result, err := s.Rebuild(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, result.Plays)
	require.Equal(t, 1, count(t, s, "source_play_records"))
	rows, err := s.DB.Query(`SELECT payload FROM source_records WHERE kind='delivery' ORDER BY ordinal`)
	require.NoError(t, err)
	defer rows.Close()
	issues := map[string]int{}
	for rows.Next() {
		var payload string
		require.NoError(t, rows.Scan(&payload))
		var row databaseRecord
		require.NoError(t, json.Unmarshal([]byte(payload), &row))
		issues[valueText(object(row.Context["parent"])["issue"])]++
	}
	require.NoError(t, rows.Err())
	require.Equal(t, 1, issues["missing parent record"])
	require.Equal(t, 2, issues["cyclic parent references"])
}

func TestNavidromeLibrariesRemainSeparateWithinOneFile(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	data := sqliteFixture(t, `CREATE TABLE media_file(id TEXT PRIMARY KEY,title TEXT,artist TEXT,album TEXT)`,
		`CREATE TABLE annotation(user_id TEXT,item_id TEXT,item_type TEXT,play_count INTEGER,play_date TEXT)`,
		`INSERT INTO media_file VALUES('track','Song','Artist','Album')`,
		`INSERT INTO annotation VALUES('one','track','media_file',3,NULL),('two','track','media_file',8,NULL)`)
	_, err := s.Import(ctx, bytes.NewReader(data), "navidrome.db", 1)
	require.NoError(t, err)
	libraries, err := s.Libraries(ctx)
	require.NoError(t, err)
	require.Len(t, libraries, 2)
	require.Equal(t, "navidrome:one", libraries[0].LibraryID)
	require.EqualValues(t, 3, libraries[0].Reported)
	require.Equal(t, "navidrome:two", libraries[1].LibraryID)
	require.EqualValues(t, 8, libraries[1].Reported)
}

func TestNavidromeStructuredGenreTags(t *testing.T) {
	row := databaseRecord{Kind: "catalog_track", Table: "media_file", Fields: map[string]any{"tags": `{"genre":[{"id":"one","value":"Folk"},{"id":"two","value":"Rock"}],"unknown":"kept"}`}}
	raw, err := json.Marshal(row)
	require.NoError(t, err)
	genres, err := SourceGenres(NavidromeDB, raw)
	require.NoError(t, err)
	require.Equal(t, []SourceGenre{{Value: "Folk", Field: "raw:/fields/tags"}, {Value: "Rock", Field: "raw:/fields/tags"}}, genres)
}

func TestNavidromeCounterRespectsMatchingAndRetainsReportedCount(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	data := sqliteFixture(t, `CREATE TABLE media_file(id TEXT PRIMARY KEY,title TEXT,artist TEXT,album TEXT)`,
		`CREATE TABLE annotation(user_id TEXT,item_id TEXT,item_type TEXT,play_count INTEGER,play_date TEXT)`,
		`CREATE TABLE scrobbles(id INTEGER PRIMARY KEY,media_file_id TEXT,user_id TEXT,submission_time INTEGER)`,
		`INSERT INTO media_file VALUES('track','Song','Artist','Album')`,
		`INSERT INTO annotation VALUES('one','track','media_file',3,'2020-01-02 03:04:05.000+00:00')`,
		`INSERT INTO scrobbles VALUES(1,'track','one',1577934245),(2,'track','one',1577934245)`)
	_, err := s.Import(ctx, bytes.NewReader(data), "navidrome.db", 1)
	require.NoError(t, err)
	saveTestRules(t, s, Rule{ID: "count-nav", Type: "inference", Subtype: "count", Enabled: true, Program: countProgram("maximum", RuleScope{Service: "navidrome"})})
	aggregate := readAggregate(t, s)
	require.EqualValues(t, 3, *aggregate.Reported)
	require.EqualValues(t, 2, aggregate.Available)
	require.EqualValues(t, 1, aggregate.Observed)
	require.EqualValues(t, 1, aggregate.Undated)
	change := currentChange(t, s)
	for i := range change.Config.Rules {
		if change.Config.Rules[i].Type == "matching" && change.Config.Rules[i].Subtype == "play" {
			change.Config.Rules[i].Enabled = false
		}
	}
	_, err = s.Save(ctx, change)
	require.NoError(t, err)
	aggregate = readAggregate(t, s)
	require.EqualValues(t, 3, *aggregate.Reported)
	require.EqualValues(t, 3, aggregate.Available)
	require.EqualValues(t, 2, aggregate.Observed)
	require.EqualValues(t, 1, aggregate.Undated)
}

func TestSQLiteBundleRejectsMissingAndAmbiguousSidecars(t *testing.T) {
	for _, test := range []struct {
		name  string
		files []struct {
			name string
			body []byte
		}
		message string
	}{
		{"missing", []struct {
			name string
			body []byte
		}{{"db-wal", []byte("wal")}}, "no matching database"},
		{"conflict", []struct {
			name string
			body []byte
		}{{"db", []byte("main")}, {"db-wal", []byte("one")}, {"db-wal", []byte("two")}}, "conflicting files"},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := testStore(t)
			var buf bytes.Buffer
			writer := zip.NewWriter(&buf)
			for _, file := range test.files {
				member, err := writer.Create(file.name)
				require.NoError(t, err)
				_, err = member.Write(file.body)
				require.NoError(t, err)
			}
			require.NoError(t, writer.Close())
			_, err := s.Import(context.Background(), bytes.NewReader(buf.Bytes()), "snapshot.zip", 1)
			require.ErrorContains(t, err, test.message)
			require.Zero(t, count(t, s, "source_records"))
			require.Equal(t, len(test.files), count(t, s, "source_members"))
		})
	}
}

func TestSQLiteUnknownTableNameIsNotAssumedToBeSearchStorage(t *testing.T) {
	s := testStore(t)
	data := sqliteFixture(t, `CREATE TABLE raw_play_events(source_type TEXT,played_at TEXT,title_raw TEXT,artist_raw TEXT,payload_json TEXT)`, `CREATE TABLE custom_fts_notes(extra TEXT)`, `INSERT INTO custom_fts_notes VALUES('kept')`)
	_, err := s.Import(context.Background(), bytes.NewReader(data), "history.sqlite", 1)
	require.NoError(t, err)
	var value string
	require.NoError(t, s.DB.QueryRow(`SELECT json_extract(payload,'$.fields.extra') FROM source_records WHERE json_extract(payload,'$.table')='custom_fts_notes'`).Scan(&value))
	require.Equal(t, "kept", value)
}

func TestRecordStreamRejectsMissingAndExtraRecords(t *testing.T) {
	for _, test := range []struct {
		body  string
		count int
	}{{"{}\n", 2}, {"{}\n{}\n", 1}} {
		path := filepath.Join(t.TempDir(), "records.jsonl")
		require.NoError(t, os.WriteFile(path, []byte(test.body), 0600))
		part := parsedFile{recordFile: path, recordCount: test.count}
		require.Error(t, part.eachRecord(func(int, json.RawMessage) error { return nil }))
	}
}

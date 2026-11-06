package sources

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func yargFixture(t *testing.T, extraScore bool) []byte {
	statements := []string{
		`CREATE TABLE GameRecords(Id INTEGER PRIMARY KEY,Date BIGINT,SongChecksum BLOB,GameVersion TEXT,SongName TEXT,SongArtist TEXT,SongSpeed REAL,PlayedWithReplay INTEGER,HasBots INTEGER,ReplayChecksum BLOB,extra TEXT)`,
		`CREATE TABLE Players(Id TEXT PRIMARY KEY,Name TEXT)`,
		`CREATE TABLE PlayerScores(Id INTEGER PRIMARY KEY,GameRecordId INTEGER,PlayerId TEXT,Score INTEGER,NotesHit INTEGER,NotesMissed INTEGER,IsReplay INTEGER)`,
		`INSERT INTO GameRecords VALUES(1,638577685386176539,x'010203','v0.12.4','Song','Artist',1,0,0,x'040506','keep')`,
		`INSERT INTO Players VALUES('one','Player One'),('two','Player Two')`,
		`INSERT INTO PlayerScores VALUES(1,1,'one',100,10,1,0),(2,1,'two',200,20,2,0)`,
	}
	if extraScore {
		statements = append(statements, `INSERT INTO PlayerScores VALUES(3,1,'one',300,30,3,0)`)
	}
	return sqliteFixture(t, statements...)
}

func addYARGTime(t *testing.T, change *FilterChange) {
	change.Config.Rules = append(change.Config.Rules, Rule{ID: "game-zone", Type: "inference", Subtype: "time", Enabled: true, Program: namingFixture(t, `{"steps":[{"if":{"op":"eq","args":[{"op":"field","field":"source.service"},{"op":"literal","value":"yarg"}]},"then":[{"set":"time.zone","value":{"op":"literal","value":"America/Denver"}}]}]}`)})
}

func addYARGListener(t *testing.T, change *FilterChange, name string) {
	encoded, err := json.Marshal(name)
	require.NoError(t, err)
	program := `{"steps":[{"if":{"op":"and","args":[{"op":"eq","args":[{"op":"field","field":"source.format"},{"op":"literal","value":"yarg-sqlite-v1"}]},{"op":"in","args":[{"op":"literal","value":` + string(encoded) + `},{"op":"field","field":"game.player_names"}]}]},"then":[{"set":"listener","value":{"op":"literal","value":"self"}}]}]}`
	change.Config.Rules = append(change.Config.Rules, Rule{ID: "game-player", Type: "attribution", Subtype: "direct", Enabled: true, Program: namingFixture(t, program)})
}

func TestYARGMultiplayerPreservationAttributionAndChangedSnapshots(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	data := yargFixture(t, false)
	first, err := s.Import(ctx, bytes.NewReader(data), "renamed.data", 1)
	require.NoError(t, err)
	require.Equal(t, 6, first.Added)
	again, err := s.Import(ctx, bytes.NewReader(data), "another.db", 1)
	require.NoError(t, err)
	require.Zero(t, again.Added)
	require.Equal(t, 6, again.Reused)
	path, err := s.FilePath(first.Hash)
	require.NoError(t, err)
	original, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, data, original)
	var raw string
	require.NoError(t, s.DB.QueryRow(`SELECT payload FROM source_records WHERE kind='game'`).Scan(&raw))
	require.Contains(t, raw, `638577685386176539`)
	require.Contains(t, raw, `"extra":"keep"`)
	v := normalizeRecord(json.RawMessage(raw), YARGDB, "America/Denver")
	require.Equal(t, "2024-07-28T13:02:18.6176539", v.TimeText)
	require.Equal(t, "2024-07-28T19:02:18.6176539Z", v.Time.Format(time.RFC3339Nano))
	require.EqualValues(t, 100, v.Precision)
	require.Equal(t, "result", v.TimeMeaning)
	require.Equal(t, "010203", v.NativeTrackID)
	require.Equal(t, []string{"one", "two"}, v.Performance.PlayerIDs)
	result, err := s.Rebuild(ctx)
	require.NoError(t, err)
	require.Zero(t, result.Plays, "timezone is unknown")
	change := currentChange(t, s)
	addYARGTime(t, &change)
	_, err = s.Save(ctx, change)
	require.NoError(t, err)
	require.Zero(t, count(t, s, "listens"), "player attribution is unknown")
	change = currentChange(t, s)
	addYARGListener(t, &change, "Player One")
	_, err = s.Save(ctx, change)
	require.NoError(t, err)
	require.Equal(t, 1, count(t, s, "listens"))
	changed, err := s.Import(ctx, bytes.NewReader(yargFixture(t, true)), "updated.db", 1)
	require.NoError(t, err)
	require.Equal(t, 7, changed.Added)
	result, err = s.Rebuild(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, result.Plays, "another score or snapshot is not another game")
	require.Equal(t, 13, count(t, s, "source_records"))
	var scores int
	require.NoError(t, s.DB.QueryRow(`SELECT json_array_length(json_extract(context_json,'$.scores')) FROM source_play_context`).Scan(&scores))
	require.Equal(t, 3, scores, "a later reading of the same native game supplies its updated score context")
}

func TestYARGTicksAndMissingFields(t *testing.T) {
	require.False(t, comparableTimeBasis("result", "start"))
	require.False(t, comparableTimeBasis("result", "end"))
	require.True(t, comparableTimeBasis("result", "result"))
	for _, value := range []any{nil, "not a date", -1, "3155378976000000000"} {
		_, valid := yargWallTime(value)
		require.False(t, valid)
	}
	v := normalizeYARG(json.RawMessage(`{"kind":"game","fields":{"Date":638577685386176539,"SongName":"Song","SongArtist":"Artist","SongSpeed":1},"context":{}}`), "UTC")
	require.Nil(t, v.Performance.HasBots, "an older schema does not report false")
	require.Nil(t, v.Performance.WithReplay)
	require.Equal(t, "not reported complete", v.Reason, "missing score references remain unresolved")
	for _, speed := range []string{"NaN", "+Inf", "-Inf", "unreported"} {
		data, err := json.Marshal(map[string]any{"kind": "game", "fields": map[string]any{"Date": int64(638577685386176539), "SongName": "Song", "SongArtist": "Artist", "SongSpeed": speed}, "context": map[string]any{}})
		require.NoError(t, err)
		value := normalizeYARG(data, "UTC")
		require.Equal(t, "invalid game speed", value.Reason)
		require.Nil(t, value.Performance.Speed)
		_, err = json.Marshal(value.Performance)
		require.NoError(t, err, "invalid source text must not poison performance publication")
	}
}

func TestLocalPhase7YARG(t *testing.T) {
	path := os.Getenv("KOITO_TEST_PHASE7_YARG")
	if path == "" {
		t.Skip("set KOITO_TEST_PHASE7_YARG to the release score database")
	}
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	s := testStore(t)
	ctx := context.Background()
	first, err := s.Import(ctx, bytes.NewReader(data), "scores.db", 1)
	require.NoError(t, err)
	require.Equal(t, 1587, first.Records)
	again, err := s.Import(ctx, bytes.NewReader(data), "renamed.db", 1)
	require.NoError(t, err)
	require.Zero(t, again.Added)
	require.Equal(t, 1587, again.Reused)
	change := currentChange(t, s)
	addYARGTime(t, &change)
	addYARGListener(t, &change, "ephun")
	_, err = s.Save(ctx, change)
	require.NoError(t, err)
	require.Equal(t, 541, count(t, s, "listens"))
	require.Equal(t, 1587, count(t, s, "source_records"))
	t.Log("568 games, 996 scores, and 22 players preserved. Repeat import reused all 1,587 records. Explicit ephun attribution contributes 541 games.")
}

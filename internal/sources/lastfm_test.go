package sources

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func TestMixedArchiveReusesLastfmAndSpotifyMembers(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	csv, err := os.ReadFile("testdata/lastfm-headerless.csv")
	if err != nil {
		t.Fatal(err)
	}
	spotify := []byte("[" + complete + "]")
	archive := func(bad bool) []byte {
		var data bytes.Buffer
		writer := zip.NewWriter(&data)
		members := map[string][]byte{"renamed.history": csv, "another.history": spotify, "notes.pdf": []byte("%PDF-1.7 supporting file")}
		if bad {
			members["broken.csv"] = []byte("Artist,Album,Title,Timestamp\n\"unterminated")
		}
		for name, payload := range members {
			member, e := writer.Create(name)
			if e != nil {
				t.Fatal(e)
			}
			if _, e = member.Write(payload); e != nil {
				t.Fatal(e)
			}
		}
		if e := writer.Close(); e != nil {
			t.Fatal(e)
		}
		return data.Bytes()
	}
	first, err := s.Import(ctx, bytes.NewReader(archive(false)), "mixed.data", 1)
	if err != nil || first.Added != 3 {
		t.Fatal(first, err)
	}
	for _, data := range [][]byte{csv, spotify} {
		result, e := s.Import(ctx, bytes.NewReader(data), "member.data", 1)
		if e != nil || result.Added != 0 {
			t.Fatal("standalone member multiplied records", result, e)
		}
	}
	if count(t, s, "source_records") != 3 || count(t, s, "source_members") != 3 {
		t.Fatal("mixed archive evidence lost")
	}
	if _, err = s.Import(ctx, bytes.NewReader(archive(true)), "broken.zip", 1); err == nil {
		t.Fatal("broken member accepted")
	}
	if count(t, s, "source_records") != 3 || count(t, s, "source_extractions") != 2 {
		t.Fatal("broken archive changed records")
	}
	c := currentChange(t, s)
	c.Config.Rules[2].Settings.Timezone = "UTC"
	p := applyChange(t, s, c)
	if p.After != 3 {
		t.Fatal("mixed archive plays lost", p)
	}
}

func TestConvertedFieldsDoNotFillOriginalBlanks(t *testing.T) {
	raw := json.RawMessage(`{"source_type":"lastfm_csv","title_raw":"Song","artist_raw":"Artist","album_raw":"Guessed album","payload_json":{"Title":"Song","Artist":"Artist","Album":"","Timestamp":"1/2/2020 03:01"}}`)
	v := normalizeRecord(raw, LastfmJSONL, "UTC")
	if v.Album != "" || v.Reason != "counted" {
		t.Fatal("converted metadata replaced an original blank", v)
	}
}

func TestLastfmFormatsPreserveFilesAndRecords(t *testing.T) {
	for _, fixture := range []struct {
		name   string
		count  int
		parser string
	}{
		{"lastfm-headerless.csv", 2, LastfmCSV}, {"lastfm-named.csv", 2, LastfmCSV},
		{"lastfm-pages.json", 3, LastfmJSON}, {"lastfm-normalized.jsonl", 2, LastfmJSONL},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			s := testStore(t)
			data, err := os.ReadFile("testdata/" + fixture.name)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(fixture.name, "headerless") {
				data = append([]byte{0xef, 0xbb, 0xbf}, data...)
			}
			first, err := s.Import(context.Background(), bytes.NewReader(data), "renamed.data", 1)
			if err != nil {
				t.Fatal(err)
			}
			if first.Added != fixture.count {
				t.Fatalf("lost records: %+v", first)
			}
			second, err := s.Import(context.Background(), bytes.NewReader(data), "again", 1)
			if err != nil || second.Added != 0 || second.Reused != fixture.count {
				t.Fatal("repeat import changed records", second, err)
			}
			p, _ := s.FilePath(first.Hash)
			original, err := os.ReadFile(p)
			if err != nil || !bytes.Equal(original, data) {
				t.Fatal("original bytes changed", err)
			}
			var parser, service, payload string
			err = s.DB.QueryRow(`SELECT r.parser,a.service,r.payload FROM source_records r JOIN source_accounts a ON a.id=r.source_id ORDER BY r.id LIMIT 1`).Scan(&parser, &service, &payload)
			if err != nil || parser != fixture.parser || service != "lastfm" {
				t.Fatal("incorrect detection", parser, service, err)
			}
			values := normalizeRecord([]byte(payload), parser, "")
			if values.Title != "Morning Light" || values.Album != "Example Album" {
				t.Fatal("metadata changed", values)
			}
			if fixture.parser == LastfmJSON {
				if values.Time.Unix() != 1577934240 || values.TimeMeaning != "start" {
					t.Fatal("native epoch timestamp changed", values)
				}
				if !strings.Contains(payload, `"extra":{"keep":true}`) {
					t.Fatal("unknown native fields lost")
				}
			} else {
				if !values.Time.IsZero() || values.Reason != "timezone needed" {
					t.Fatal("undeclared zone was guessed", values)
				}
				values = normalizeRecord([]byte(payload), parser, "UTC")
				if values.Time.Unix() != 1577934240 || values.Precision != int64(time.Minute) {
					t.Fatal("wrong wall time or precision", values)
				}
			}
		})
	}
}

func TestLastfmNormalizationKeepsSparseAndConvertedData(t *testing.T) {
	row, _ := json.Marshal(csvRecord{Columns: []string{"Artist", "Album", "Title", "Timestamp", "Extra"}, Values: []string{"Example Artist", "", "Evening Song", "1/2/2020 03:08", "retain"}})
	v := normalizeRecord(row, LastfmCSV, "UTC")
	if v.Album != "" || v.TimeText != "1/2/2020 03:08" {
		t.Fatal("sparse fields changed", v)
	}
	converted := []byte(`{"source_type":"lastfm_csv","played_at":"2020-01-02T03:04:00Z","title_raw":"Morning Light","artist_raw":"Example Artist","payload_json":{"Title":"Morning Light","Artist":"Example Artist","Timestamp":"1/2/2020 03:04"}}`)
	v = normalizeRecord(converted, LastfmJSONL, "")
	if v.Origin != "converted" || v.Reason != "timezone needed" || !v.Time.IsZero() {
		t.Fatal("converter's appended Z was trusted", v)
	}
	playing := normalizeRecord([]byte(`{"name":"Song","artist":{"#text":"Artist"},"@attr":{"nowplaying":true}}`), LastfmJSON, "")
	if !playing.NowPlaying || playing.Reason != "now playing" {
		t.Fatal("now-playing treated as completed", playing)
	}
}

func TestLastfmWallTimesDoNotGuessDST(t *testing.T) {
	for _, test := range []struct{ text, zone, reason string }{
		{"11/1/2020 01:30", "America/Denver", "ambiguous local time"},
		{"3/8/2020 02:30", "America/Denver", "nonexistent local time"},
		{"1/2/2020 03:04", "", "timezone needed"},
		{"1/2/2020 03:04", "not/a/zone", "invalid timezone"},
	} {
		t.Run(test.reason, func(t *testing.T) {
			instant, _, reason := interpretTime(test.text, test.zone)
			if !instant.IsZero() || reason != test.reason {
				t.Fatal("uncertainty lost", instant, reason)
			}
		})
	}
	instant, precision, reason := interpretTime("1/2/2020 03:04", "America/Denver")
	if reason != "counted" || instant.Format(time.RFC3339) != "2020-01-02T10:04:00Z" || precision != int64(time.Minute) {
		t.Fatal("wrong declared zone conversion", instant, precision, reason)
	}
}

func TestMalformedLastfmIsAtomic(t *testing.T) {
	s := testStore(t)
	data := []byte("Artist,Album,Title,Timestamp\nArtist,Album,Song,1/2/2020 03:04\n\"unclosed")
	result, err := s.Import(context.Background(), bytes.NewReader(data), "broken.csv", 1)
	if err == nil || count(t, s, "source_records") != 0 {
		t.Fatal("malformed CSV partly imported", err)
	}
	path, _ := s.FilePath(result.Hash)
	stored, e := os.ReadFile(path)
	if e != nil || !bytes.Equal(data, stored) {
		t.Fatal("failed original was lost")
	}
	receipts, e := s.Receipts(context.Background())
	if e != nil || len(receipts) != 1 || receipts[0].Status != "failed" {
		t.Fatal("failure not recorded", receipts, e)
	}
}

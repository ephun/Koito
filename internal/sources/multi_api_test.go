package sources

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestMultiAPIHistoryReusesNativeEvents(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	data := []byte(`{"data":[{"uid":"source-one","componentId":3,"parentId":null,"play":{"data":{"track":"Song","artists":["Artist"],"album":"Album","playDate":"2026-11-05T03:00:00Z"},"meta":{"musicService":"spotify"}},"unknown":{"keep":true}}],"meta":{"limit":100,"offset":0,"total":1}}`)
	for i := 0; i < 2; i++ {
		result, err := s.Import(ctx, bytes.NewReader(data), "router-page.json", 1)
		if err != nil {
			t.Fatal(err)
		}
		if i == 1 && result.Added != 0 {
			t.Fatal("Reimport added duplicate records")
		}
	}
	var original string
	if err := s.DB.QueryRow(`SELECT payload FROM source_records LIMIT 1`).Scan(&original); err != nil {
		t.Fatal(err)
	}
	value := normalizeMultiAPI(json.RawMessage(original))
	if value.EventID != "multi:source-one" || value.Title != "Song" || value.OriginalService != "spotify" || value.Reason != "counted" {
		t.Fatalf("Lost native event identity: %+v", value)
	}
	if _, err := s.Rebuild(ctx); err != nil {
		t.Fatal(err)
	}
	if count(t, s, "listens") != 1 {
		t.Fatal("Router source history did not contribute a play")
	}
	client := []byte(`{"data":[{"uid":"delivery-one","componentId":1,"parentId":7,"parent":{"uid":"source-one"},"play":{"data":{"track":"Song","artists":["Artist"],"album":"Album","playDate":"2026-11-05T03:00:00Z"},"meta":{"musicService":"spotify"}}}],"meta":{"limit":100,"offset":0,"total":1}}`)
	if _, err := s.Import(ctx, bytes.NewReader(client), "client-page.json", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Rebuild(ctx); err != nil {
		t.Fatal(err)
	}
	if count(t, s, "listens") != 1 || count(t, s, "source_play_records") != 2 {
		t.Fatal("Destination copy counted independently or lost supporting evidence")
	}
}

func TestMultiAPIDuplicateStateRetainsEvidence(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	data := []byte(`{"data":[{"uid":"duplicate-source","componentId":3,"parentId":null,"state":"duped","play":{"data":{"track":"Song","artists":["Artist"],"playDate":"2026-11-05T03:00:00Z"}}},{"uid":"duplicate-delivery","componentId":1,"parentId":7,"parent":{"uid":"original-source"},"state":"duped","play":{"data":{"track":"Song","artists":["Artist"],"playDate":"2026-11-05T03:00:00Z"}}}],"meta":{"limit":100,"offset":0,"total":2}}`)
	if _, err := s.Import(ctx, bytes.NewReader(data), "duplicates.json", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Rebuild(ctx); err != nil {
		t.Fatal(err)
	}
	if count(t, s, "source_records") != 2 || count(t, s, "listens") != 0 {
		t.Fatal("Source duplicates or delivery evidence contributed an independent play")
	}
	delivery := normalizeMultiAPI(json.RawMessage(`{"uid":"duplicate-delivery","parentId":7,"parent":{"uid":"original-source"},"state":"duped","play":{"data":{"track":"Song","artists":["Artist"],"playDate":"2026-11-05T03:00:00Z"}}}`))
	if delivery.Reason != "forwarding evidence" || delivery.SupportID != "multi:original-source" {
		t.Fatal("Duplicate delivery lost its supporting relationship")
	}
}

// Opt-in local check uses original saved router responses, including all nested
// metadata and destination attempts. It never changes the source snapshot.
func TestLocalPhase8Router(t *testing.T) {
	directory := os.Getenv("KOITO_PHASE8_ROUTER")
	if directory == "" {
		t.Skip("Set KOITO_PHASE8_ROUTER to a preserved API snapshot")
	}
	s := testStore(t)
	ctx := context.Background()
	files, err := filepath.Glob(filepath.Join(directory, "component-*-offset-*.json"))
	if err != nil || len(files) == 0 {
		t.Fatal("Missing saved pages", err)
	}
	rows := 0
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var page struct {
			Data []json.RawMessage `json:"data"`
		}
		if err = json.Unmarshal(data, &page); err != nil {
			t.Fatal(err)
		}
		if len(page.Data) == 0 {
			continue
		}
		rows += len(page.Data)
		for attempt := 0; attempt < 2; attempt++ {
			result, err := s.Import(ctx, bytes.NewReader(data), filepath.Base(file), 1)
			if err != nil {
				t.Fatalf("%s: %v", filepath.Base(file), err)
			}
			if attempt == 1 && result.Added != 0 {
				t.Fatal("Repeat imported extra records")
			}
		}
	}
	if count(t, s, "source_records") != rows {
		t.Fatal("Did not preserve all original rows")
	}
	result, err := s.Rebuild(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.Plays == 0 {
		t.Fatal("No source events recovered")
	}
	first := count(t, s, "listens")
	if _, err = s.Rebuild(ctx); err != nil {
		t.Fatal(err)
	}
	if count(t, s, "listens") != first {
		t.Fatal("Repeated reconstruction changed the play count")
	}
	t.Logf("Preserved %d original API rows across %d pages. Rebuilt %d source plays and %d displayed listens.", rows, len(files), result.Plays, first)
}

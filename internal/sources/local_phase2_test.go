package sources

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Opt-in acceptance using private original files. Only counts and byte hashes
// are reported; fixtures and all application state remain outside Git.
func TestLocalPhase2Sources(t *testing.T) {
	raw := os.Getenv("KOITO_TEST_PHASE2_FILES")
	if raw == "" {
		t.Skip("set KOITO_TEST_PHASE2_FILES to a JSON array of local source paths")
	}
	var files []string
	if err := json.Unmarshal([]byte(raw), &files); err != nil {
		t.Fatal(err)
	}
	s := testStore(t)
	ctx := context.Background()
	start := time.Now()
	total := 0
	for i, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		first, err := s.Import(ctx, bytes.NewReader(data), filepath.Base(path), 1)
		if err != nil {
			t.Fatalf("source %d: %v", i, err)
		}
		second, err := s.Import(ctx, bytes.NewReader(data), "renamed.data", 1)
		if err != nil || second.Added != 0 || second.Reused != first.Records {
			t.Fatalf("source %d repeated: %+v %v", i, second, err)
		}
		original, _ := s.FilePath(first.Hash)
		stored, err := os.ReadFile(original)
		if err != nil || !bytes.Equal(stored, data) {
			t.Fatal("original bytes changed", i, err)
		}
		total += first.Added
		t.Logf("source %d: %d preserved records", i, first.Added)
	}
	fingerprint := func(query string) string {
		rows, err := s.DB.QueryContext(ctx, query)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		h := sha256.New()
		for rows.Next() {
			var row string
			if err = rows.Scan(&row); err != nil {
				t.Fatal(err)
			}
			fmt.Fprintf(h, "%d:%s", len(row), row)
		}
		if err = rows.Err(); err != nil {
			t.Fatal(err)
		}
		return hex.EncodeToString(h.Sum(nil))
	}
	evidenceQuery := `SELECT json_array(id,source_id,file_hash,ordinal,kind,payload,parser) FROM source_records ORDER BY id`
	before := fingerprint(evidenceQuery)
	initial, err := s.Rebuild(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("initial rebuild: plays=%d elapsed=%s", initial.Plays, time.Since(start).Round(time.Millisecond))
	c := currentChange(t, s)
	// Explicit test setting, not an assumption about the user's CSV timezone.
	c.Config.Rules[2].Settings.Timezone = "UTC"
	preview := applyChange(t, s, c)
	t.Logf("timezone apply: plays=%d elapsed=%s", preview.After, time.Since(start).Round(time.Millisecond))
	var cross int
	if err = s.DB.QueryRow(`SELECT COUNT(*) FROM (SELECT i.play_key FROM source_interpretations i JOIN source_records r ON r.id=i.record_id JOIN source_accounts a ON a.id=r.source_id WHERE i.policy=? AND i.play_key<>'' GROUP BY i.play_key HAVING COUNT(DISTINCT a.service)>1)`, Policy).Scan(&cross); err != nil || cross == 0 {
		t.Fatal("no real cross-service matches", cross, err)
	}
	linksQuery := `SELECT json_array(record_id,play_key,reason) FROM source_interpretations ORDER BY record_id`
	links := fingerprint(linksQuery)
	again, err := s.Rebuild(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("repeat rebuild: plays=%d elapsed=%s", again.Plays, time.Since(start).Round(time.Millisecond))
	if again.Plays != preview.After || fingerprint(linksQuery) != links {
		t.Fatal("real rebuild changed supporting-record links")
	}
	if fingerprint(evidenceQuery) != before || count(t, s, "source_records") != total {
		t.Fatal("rules or rebuild changed real evidence")
	}
	state, err := s.Filters(ctx)
	if err != nil {
		t.Fatal(err)
	}
	c = currentChange(t, s)
	c.Config.Rules[1].Settings.ToleranceSeconds = 2
	relaxed, err := s.Preview(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	if fingerprint(linksQuery) != links {
		t.Fatal("real preview changed view")
	}
	c = currentChange(t, s)
	c.RestoreVersion = 1
	applyChange(t, s, c)
	if count(t, s, "source_plays") != initial.Plays {
		t.Fatal("real rule restore changed previous result")
	}
	c = currentChange(t, s)
	c.RestoreVersion = state.Current.ID
	applyChange(t, s, c)
	if fingerprint(linksQuery) != links || fingerprint(evidenceQuery) != before {
		t.Fatal("real rule restore lost links/evidence")
	}
	t.Logf("records=%d plays=%d cross-service=%d ambiguous_records=%d tolerance_2s_plays=%d tolerance_2s_ambiguous=%d raw_sha256=%s elapsed=%s", total, preview.After, cross, preview.Ambiguous, relaxed.After, relaxed.Ambiguous, before, time.Since(start).Round(time.Millisecond))
}

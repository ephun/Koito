package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func importLastfm(t *testing.T, s *Store, raw string) {
	t.Helper()
	if _, err := s.Import(context.Background(), strings.NewReader(raw), "history.data", 1); err != nil {
		t.Fatal(err)
	}
}
func lf(title, album string, unix int64) string {
	return fmt.Sprintf(`{"name":%q,"artist":{"#text":"Artist"},"album":{"#text":%q},"date":{"uts":%q},"extra":{"kept":true}}`, title, album, fmt.Sprint(unix))
}
func currentChange(t *testing.T, s *Store) FilterChange {
	t.Helper()
	state, err := s.Filters(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return FilterChange{BaseVersion: state.Current.ID, Config: state.Current.Config}
}
func applyChange(t *testing.T, s *Store, c FilterChange) FilterPreview {
	t.Helper()
	p, err := s.Preview(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	c.Token = p.Token
	if _, err = s.Apply(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestMatchPreviewApplyManualSplitAndRestore(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	mustImport(t, s, []byte("["+complete+"]"), "spotify.json", "")
	// Spotify stopped at 03:04:05, after 180.001 seconds. Last.fm reports a
	// start of 03:01:05, whose one-second interval overlaps the estimate.
	importLastfm(t, s, "["+lf("Song", "Album", 1577934065)+"]")
	if _, err := s.Rebuild(ctx); err != nil {
		t.Fatal(err)
	}
	if count(t, s, "listens") != 1 || count(t, s, "source_records") != 2 {
		t.Fatal("known pair not one play")
	}
	inspection, err := s.Inspect(ctx, 1)
	if err != nil || len(inspection.Contributors) != 2 {
		t.Fatal(inspection, err)
	}
	if inspection.Contributors[0].Values.TimeMeaning != "end" || inspection.Contributors[1].Values.TimeMeaning != "start" {
		t.Fatal("timestamp semantics lost")
	}
	var original string
	if err = s.DB.QueryRow(`SELECT payload FROM source_records WHERE id=1`).Scan(&original); err != nil {
		t.Fatal(err)
	}
	c := currentChange(t, s)
	c.Decision = &DecisionChange{Left: 1, Right: 2, Action: "split"}
	p, err := s.Preview(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	if p.Before != 1 || p.After != 2 || p.Changed != 2 || len(p.Examples) != 2 {
		t.Fatal("split preview", p)
	}
	if count(t, s, "listens") != 1 || count(t, s, "match_decisions") != 0 || count(t, s, "filter_versions") != 1 {
		t.Fatal("preview mutated state")
	}
	c.Token = p.Token
	if _, err = s.Apply(ctx, c); err != nil {
		t.Fatal(err)
	}
	if count(t, s, "listens") != 2 {
		t.Fatal("split collapsed same-second plays")
	}
	if _, err = s.Rebuild(ctx); err != nil {
		t.Fatal(err)
	}
	if count(t, s, "listens") != 2 {
		t.Fatal("manual split lost on rebuild")
	}
	c = currentChange(t, s)
	c.RestoreVersion = 1
	applyChange(t, s, c)
	if count(t, s, "listens") != 2 {
		t.Fatal("restoring rules erased manual split")
	}
	c = currentChange(t, s)
	c.Decision = &DecisionChange{UndoID: 1}
	applyChange(t, s, c)
	if count(t, s, "listens") != 1 {
		t.Fatal("undo did not restore automatic match")
	}
	var after string
	if err = s.DB.QueryRow(`SELECT payload FROM source_records WHERE id=1`).Scan(&after); err != nil || original != after {
		t.Fatal("records changed", err)
	}
	if count(t, s, "match_decisions") != 2 || count(t, s, "filter_versions") != 4 {
		t.Fatal("change history lost")
	}
}

func TestRulesRetainUnmatchedAndPreferredRelease(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	mustImport(t, s, []byte("["+complete+"]"), "spotify", "")
	// Reliable common URI permits differing reported albums; both remain in
	// inspection. Preference chooses display fields, never drops unmatched plays.
	first := strings.TrimSuffix(lf("Song", "Compilation", 1577934065), "}") + `,"spotify_track_uri":"spotify:track:one"}`
	importLastfm(t, s, "["+first+","+lf("Last.fm only", "Other album", 1577935000)+"]")
	if _, err := s.Rebuild(ctx); err != nil {
		t.Fatal(err)
	}
	if count(t, s, "listens") != 2 {
		t.Fatal("unmatched Last.fm play lost")
	}
	c := currentChange(t, s)
	c.Config.Rules[1].Settings.PreferredSource = "lastfm"
	p := applyChange(t, s, c)
	if p.Before != 2 || p.After != 2 || p.Changed != 1 {
		t.Fatal("field-only preview missing", p)
	}
	plays, err := s.Plays(ctx, 0, "", false)
	if err != nil || len(plays) != 2 || plays[0].Album != "Compilation" {
		t.Fatal(plays, err)
	}
	inspected, err := s.Inspect(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if inspected.Contributors[0].Values.Album != "Album" || inspected.Contributors[1].Values.Album != "Compilation" {
		t.Fatal("source albums overwritten")
	}
	c = currentChange(t, s)
	c.Config.Rules[1].Enabled = false
	applyChange(t, s, c)
	if count(t, s, "listens") != 3 {
		t.Fatal("disabled matching hid source plays")
	}
	c = currentChange(t, s)
	c.Decision = &DecisionChange{Left: 1, Right: 2, Action: "match"}
	applyChange(t, s, c)
	if count(t, s, "listens") != 2 {
		t.Fatal("manual matching depends on automatic matching")
	}
	if _, err = s.Rebuild(ctx); err != nil {
		t.Fatal(err)
	}
	if count(t, s, "listens") != 2 {
		t.Fatal("manual match lost")
	}
}

func TestAmbiguityDoesNotChainRepeats(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	importLastfm(t, s, "["+lf("Song", "Album", 1577934060)+","+lf("Song", "Album", 1577934080)+"]")
	importLastfm(t, s, "Artist,Album,Song,\"02 Jan 2020, 03:01\"\n")
	c := currentChange(t, s)
	c.Config.Rules[2].Settings.Timezone = "UTC"
	p := applyChange(t, s, c)
	if p.After != 3 || p.Ambiguous != 3 {
		t.Fatal("ambiguous minute matched a repeat", p)
	}
	inspected, err := s.Inspect(ctx, 3)
	if err != nil || len(inspected.Candidates) != 2 {
		t.Fatal(inspected, err)
	}
	plays, err := s.Plays(ctx, 0, "Song", true)
	if err != nil || len(plays) != 3 {
		t.Fatal("ambiguous search", plays, err)
	}
	c = currentChange(t, s)
	c.Decision = &DecisionChange{Left: 1, Right: 3, Action: "match"}
	applyChange(t, s, c)
	if count(t, s, "listens") != 2 {
		t.Fatal("explicit match failed")
	}
	// The known second event must not join through the minute-precision record.
	inspected, err = s.Inspect(ctx, 2)
	if err != nil || len(inspected.Contributors) != 1 {
		t.Fatal("transitive repeat merge", inspected, err)
	}
}

func TestConflictsSparseAlbumAndTimeSettings(t *testing.T) {
	for _, test := range []struct {
		name, album string
		delta       int64
		tolerance   int
		timing      string
		want        int
	}{
		{"conflicting albums", "Other", 0, 0, "estimated_start", 2},
		{"sparse album unique", "", 0, 0, "estimated_start", 1},
		{"offset outside tolerance", "Album", 3, 0, "estimated_start", 2},
		{"offset allowed", "Album", 3, 3, "estimated_start", 1},
		{"end is not start", "Album", 0, 3600, "end", 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := testStore(t)
			mustImport(t, s, []byte("["+complete+"]"), "spotify", "")
			importLastfm(t, s, "["+lf("Song", test.album, 1577934065+test.delta)+"]")
			c := currentChange(t, s)
			c.Config.Rules[1].Settings.ToleranceSeconds = test.tolerance
			c.Config.Rules[1].Settings.SpotifyTime = test.timing
			p := applyChange(t, s, c)
			if p.After != test.want {
				t.Fatal(p)
			}
		})
	}
}

func TestTimezoneScopeAndUndo(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	importLastfm(t, s, "Artist,Album,Song,\"02 Jan 2020, 03:01\"\n")
	importLastfm(t, s, "Artist,Album,Other,\"02 Jan 2020, 04:01\"\n")
	if _, err := s.Rebuild(ctx); err != nil {
		t.Fatal(err)
	}
	if count(t, s, "listens") != 0 {
		t.Fatal("unknown timezone became UTC")
	}
	state, err := s.Filters(ctx)
	if err != nil || len(state.TimeFiles) != 2 {
		t.Fatal(state, err)
	}
	c := currentChange(t, s)
	c.Config.Rules = append(c.Config.Rules, Rule{ID: "one-file", Type: "inference", Subtype: "time", Enabled: true, Scope: RuleScope{FileHash: state.TimeFiles[0].Hash}, Settings: RuleSettings{Timezone: "UTC"}})
	p := applyChange(t, s, c)
	if p.After != 1 || p.Unresolved != 1 {
		t.Fatal("file scope leaked", p)
	}
	c = currentChange(t, s)
	c.RestoreVersion = 1
	applyChange(t, s, c)
	if count(t, s, "listens") != 0 || count(t, s, "source_records") != 2 {
		t.Fatal("time undo lost evidence")
	}
}

func TestStalePreviewAndAtomicFailure(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	mustImport(t, s, []byte("["+complete+"]"), "spotify", "")
	if _, err := s.Rebuild(ctx); err != nil {
		t.Fatal(err)
	}
	c := currentChange(t, s)
	p, err := s.Preview(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	c.Token = p.Token
	importLastfm(t, s, "["+lf("Song", "Album", 1577934065)+"]")
	if _, err = s.Apply(ctx, c); err != ErrStalePreview {
		t.Fatal("stale evidence accepted", err)
	}
	c = currentChange(t, s)
	p, err = s.Preview(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	c.Token = p.Token
	if _, err = s.DB.Exec(`CREATE TRIGGER fail_apply BEFORE INSERT ON listens BEGIN SELECT RAISE(ABORT,'test failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Apply(ctx, c); err == nil {
		t.Fatal("expected publication failure")
	}
	if count(t, s, "listens") != 1 || count(t, s, "filter_versions") != 1 || count(t, s, "source_records") != 2 || count(t, s, "source_interpretations") != 1 {
		t.Fatal("failed apply changed previous view")
	}
	if _, err = s.DB.Exec(`DROP TRIGGER fail_apply`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Apply(ctx, c); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Apply(ctx, c); err != ErrStalePreview {
		t.Fatal("stale version accepted", err)
	}
	for _, q := range []string{`UPDATE filter_versions SET label='x'`, `DELETE FROM filter_versions`} {
		if _, err = s.DB.Exec(q); err == nil {
			t.Fatal("saved version mutable")
		}
	}
}

func TestPreviewDeterministicAndNoChanges(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	mustImport(t, s, []byte("["+complete+"]"), "spotify", "")
	if _, err := s.Rebuild(ctx); err != nil {
		t.Fatal(err)
	}
	c := currentChange(t, s)
	a, err := s.Preview(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Preview(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) || a.Changed != 0 {
		t.Fatal(a, b)
	}
	raw, _ := json.Marshal(c.Config)
	if !strings.Contains(string(raw), "matching") {
		t.Fatal("settings missing")
	}
}

func TestUnicodeCaseMatchingAndConflictingIDs(t *testing.T) {
	ctx := context.Background()
	t.Run("Unicode case", func(t *testing.T) {
		s := testStore(t)
		importLastfm(t, s, "["+strings.Replace(lf("Song", "Album", 1577934065), "Artist", "Σ", 1)+"]")
		importLastfm(t, s, "["+strings.Replace(lf("Song", "Album", 1577934065), "Artist", "ς", 1)+"]")
		if _, err := s.Rebuild(ctx); err != nil {
			t.Fatal(err)
		}
		if count(t, s, "listens") != 2 {
			t.Fatal("case-sensitive music was merged")
		}
		c := currentChange(t, s)
		c.Config.Rules[0].Settings.CaseSensitive = false
		applyChange(t, s, c)
		if count(t, s, "listens") != 1 {
			t.Fatal("Unicode case equivalents were missed")
		}
		c = currentChange(t, s)
		c.Config.Rules[0].Enabled = false
		applyChange(t, s, c)
		if count(t, s, "listens") != 2 {
			t.Fatal("disabled music rule still folded names")
		}
	})
	t.Run("Conflicting recording IDs", func(t *testing.T) {
		s := testStore(t)
		first := strings.TrimSuffix(lf("Song", "Album", 1577934065), "}") + `,"spotify_track_uri":"spotify:track:one","recording_mbid":"00000000-0000-0000-0000-000000000001"}`
		second := strings.Replace(first, "00000000-0000-0000-0000-000000000001", "00000000-0000-0000-0000-000000000002", 1)
		importLastfm(t, s, "["+first+","+second+"]")
		if _, err := s.Rebuild(ctx); err != nil {
			t.Fatal(err)
		}
		if count(t, s, "listens") != 2 {
			t.Fatal("conflicting recording IDs were merged")
		}
	})
}

func TestTimingToleranceDoesNotTransitivelyJoinEvents(t *testing.T) {
	s := testStore(t)
	for _, unix := range []int64{1577934000, 1577934010, 1577934020} {
		importLastfm(t, s, "["+lf("Song", "Album", unix)+"]")
	}
	c := currentChange(t, s)
	c.Config.Rules[1].Settings.ToleranceSeconds = 15
	p := applyChange(t, s, c)
	if p.After != 3 || p.Ambiguous != 3 {
		t.Fatal("timing chain merged genuine repeats", p)
	}
}

func TestCoarseSameMinuteMultiplicityRemainsAmbiguous(t *testing.T) {
	s := testStore(t)
	row := "Artist,Album,Short Song,\"02 Jan 2020, 03:01\"\n"
	importLastfm(t, s, row+row)
	c := currentChange(t, s)
	c.Config.Rules[2].Settings.Timezone = "UTC"
	p := applyChange(t, s, c)
	if p.After != 2 || p.Ambiguous != 2 {
		t.Fatal("minute precision erased possible genuine repeat", p)
	}
	importLastfm(t, s, "Artist,Album,Title,Timestamp\nArtist,Album,Short Song,1/2/2020 03:01\n")
	c = currentChange(t, s)
	p = applyChange(t, s, c)
	if p.After != 3 || p.Ambiguous != 3 {
		t.Fatal("overlap was assigned arbitrarily to repeated rows", p)
	}
	c = currentChange(t, s)
	c.Decision = &DecisionChange{Left: 1, Right: 2, Action: "match"}
	applyChange(t, s, c)
	if count(t, s, "listens") != 1 {
		t.Fatal("explicit duplicate correction did not allow unique overlap")
	}
}

package sources

import (
	"context"
	"testing"
)

func TestSaveRulesPublishesOnceAndKeepsInactiveSettings(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	mustImport(t, s, []byte("["+complete+"]"), "spotify", "")
	importLastfm(t, s, "["+lf("Song", "Album", 1577934065)+"]")
	change := currentChange(t, s)
	change.Config.Rules[0].Enabled = false
	change.Config.Rules[0].Settings.CaseSensitive = false
	change.Config.Rules[0].Settings.MatchMissingAlbum = true
	version, err := s.Save(ctx, change)
	if err != nil {
		t.Fatal(err)
	}
	if count(t, s, "listens") != 1 || count(t, s, "source_projection_runs") != 1 {
		t.Fatal("save did not publish one complete view")
	}
	music := version.Config.Rules[0]
	if music.Enabled || music.Settings.CaseSensitive || !music.Settings.MatchMissingAlbum {
		t.Fatal("inactive settings were lost", music)
	}
	if _, err = s.Save(ctx, change); err != ErrStalePreview {
		t.Fatal("stale save accepted", err)
	}
	change = currentChange(t, s)
	change.Decision = &DecisionChange{Left: 1, Right: 2, Action: "split"}
	if _, err = s.Save(ctx, change); err != nil {
		t.Fatal(err)
	}
	if count(t, s, "listens") != 2 || count(t, s, "match_decisions") != 1 {
		t.Fatal("direct manual split failed")
	}
	if _, err = s.Rebuild(ctx); err != nil {
		t.Fatal(err)
	}
	if count(t, s, "listens") != 2 {
		t.Fatal("save lost manual decisions")
	}
}

func TestSaveFailurePreservesRulesAndView(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	mustImport(t, s, []byte("["+complete+"]"), "spotify", "")
	if _, err := s.Rebuild(ctx); err != nil {
		t.Fatal(err)
	}
	change := currentChange(t, s)
	change.Config.Rules[1].Enabled = false
	if _, err := s.DB.Exec(`CREATE TRIGGER reject_save BEFORE INSERT ON listens BEGIN SELECT RAISE(ABORT,'save failed'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Save(ctx, change); err == nil {
		t.Fatal("expected save failure")
	}
	state, err := s.Filters(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if state.Current.ID != 2 || count(t, s, "filter_versions") != 2 || count(t, s, "listens") != 1 || count(t, s, "source_records") != 1 {
		t.Fatal("failed save changed rules or view")
	}
}

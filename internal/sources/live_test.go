package sources

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const liveExample = ` {
 "listen_type":"single", "unknown_envelope":{"original":true},
 "payload":[{"listened_at":1793937600,"track_metadata":{"artist_name":"Artist","track_name":"Song","release_name":"Album","additional_info":{"music_service_name":"Spotify","submission_client":"Multi-Scrobbler","device_name":"Desktop","duration_ms":180000,"unknown_nested":{"keep":[1,true,"value"]}}}}]
} `

func captureExample(t *testing.T, s *Store, text, key string) LiveEnvelope {
	t.Helper()
	entry, err := s.CaptureLive(context.Background(), []byte(text), 1, key, "test")
	if err != nil {
		t.Fatal(err)
	}
	return entry
}

func TestLiveCaptureIndependentOfDatabaseWriter(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`UPDATE users SET username='writer-held' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	// With one SQL connection, even a database read here would deadlock.
	entry := captureExample(t, s, liveExample, "")
	original, err := s.ReadLiveRequest(entry)
	if err != nil || !bytes.Equal(original, []byte(liveExample)) {
		t.Fatalf("Original bytes changed: %v", err)
	}
	entries, err := s.PendingLive(ctx)
	if err != nil || len(entries) != 1 {
		t.Fatalf("Acknowledged request not recoverable: %v, %d", err, len(entries))
	}
}

func TestLiveRecoveryAndRetryMatching(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	first := captureExample(t, s, liveExample, "")
	captureExample(t, s, liveExample, "")
	if err := s.StoreLive(ctx, first); err != nil {
		t.Fatal(err)
	}
	// Simulate a stop after SQL import but before publication or journal removal.
	restarted := &Store{DB: s.DB, Root: s.Root}
	if err := restarted.ProcessLive(ctx); err != nil {
		t.Fatal(err)
	}
	if count(t, s, "source_records") != 2 || count(t, s, "live_submissions") != 2 || count(t, s, "listens") != 1 {
		t.Fatal("Lost delivery evidence or multiplied a matched retry")
	}
	if err := restarted.ProcessLive(ctx); err != nil {
		t.Fatal(err)
	}
	if count(t, s, "source_records") != 2 || count(t, s, "listens") != 1 {
		t.Fatal("Recovery replay multiplied data")
	}
	var payload string
	if err := s.DB.QueryRow(`SELECT payload FROM source_records LIMIT 1`).Scan(&payload); err != nil || !strings.Contains(payload, `"unknown_nested":{"keep":[1,true,"value"]}`) {
		t.Fatal("Unknown fields lost", err)
	}
	pending, err := s.PendingLive(ctx)
	if err != nil || len(pending) != 0 {
		t.Fatal("Published journal was not completed", err)
	}
	var projected int
	if err = s.DB.QueryRow(`SELECT COUNT(*) FROM live_submissions WHERE status='projected' AND version_id IS NOT NULL`).Scan(&projected); err != nil || projected != 2 {
		t.Fatal("Missing publication provenance", err)
	}
}

func TestLiveExplicitRetryKeyReusesRecords(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	captureExample(t, s, liveExample, "delivery-one")
	captureExample(t, s, liveExample, "delivery-one")
	if err := s.ProcessLive(ctx); err != nil {
		t.Fatal(err)
	}
	if count(t, s, "source_records") != 1 || count(t, s, "live_submissions") != 2 || count(t, s, "live_submission_records") != 2 {
		t.Fatal("Explicit retry lost attempts or created duplicate records")
	}
	// Reusing a transport key with different bytes must preserve the new input.
	captureExample(t, s, strings.ReplaceAll(liveExample, "1793937600", "1793937900"), "delivery-one")
	if err := s.ProcessLive(ctx); err != nil {
		t.Fatal(err)
	}
	if count(t, s, "source_records") != 2 || count(t, s, "listens") != 2 {
		t.Fatal("A reused key silently discarded different data")
	}
}

func TestLiveNowPlayingWithTimestampDoesNotBecomePlay(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	captureExample(t, s, strings.Replace(liveExample, `"single"`, `"playing_now"`, 1), "")
	if err := s.ProcessLive(ctx); err != nil {
		t.Fatal(err)
	}
	if count(t, s, "source_records") != 1 || count(t, s, "listens") != 0 {
		t.Fatal("Now playing became a completed listen")
	}
	if _, err := s.Rebuild(ctx); err != nil {
		t.Fatal(err)
	}
	if count(t, s, "listens") != 0 {
		t.Fatal("Rebuild converted now playing into a listen")
	}
}

func TestLiveNowPlayingExpiryStopAndRestart(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	entry := captureExample(t, s, strings.Replace(liveExample, `"single"`, `"playing_now"`, 1), "")
	track, handled, err := s.LiveNowPlaying(ctx, 1, entry.ReceivedAt.Add(time.Second))
	if err != nil || !handled || track == nil || track.Title != "Song" {
		t.Fatalf("Pending playback unavailable: %v %v %+v", err, handled, track)
	}
	track, handled, err = s.LiveNowPlaying(ctx, 1, entry.ReceivedAt.Add(4*time.Minute))
	if err != nil || !handled || track != nil {
		t.Fatal("Expired playback remained active", err)
	}
	if err = s.ProcessLive(ctx); err != nil {
		t.Fatal(err)
	}
	restarted := &Store{DB: s.DB, Root: s.Root}
	track, handled, err = restarted.LiveNowPlaying(ctx, 1, entry.ReceivedAt.Add(time.Second))
	if err != nil || !handled || track == nil {
		t.Fatal("Restart lost valid playback", err)
	}
	stop := captureExample(t, s, `{"listen_type":"playing_now","payload":[]}`, "")
	track, handled, err = restarted.LiveNowPlaying(ctx, 1, stop.ReceivedAt.Add(time.Second))
	if err != nil || !handled || track != nil {
		t.Fatal("Stop did not clear playback", err)
	}
	if err = s.ProcessLive(ctx); err != nil {
		t.Fatal(err)
	}
	track, handled, err = restarted.LiveNowPlaying(ctx, 1, stop.ReceivedAt.Add(time.Second))
	if err != nil || !handled || track != nil {
		t.Fatal("Restart replayed stopped playback", err)
	}
	if count(t, s, "listens") != 0 {
		t.Fatal("Playback state created a completed play")
	}
}

func TestLiveNowPlayingOrderingAndCompletion(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	playing := strings.Replace(liveExample, `"single"`, `"playing_now"`, 1)
	newer := strings.ReplaceAll(strings.ReplaceAll(playing, "1793937600", "1793937900"), `"Song"`, `"New Song"`)
	newEntry := captureExample(t, s, newer, "")
	late := captureExample(t, s, playing, "")
	track, _, err := s.LiveNowPlaying(ctx, 1, late.ReceivedAt.Add(time.Second))
	if err != nil || track == nil || track.Title != "New Song" {
		t.Fatal("Late old update replaced current playback", err)
	}
	captureExample(t, s, liveExample, "")
	track, _, err = s.LiveNowPlaying(ctx, 1, late.ReceivedAt.Add(time.Second))
	if err != nil || track == nil || track.Title != "New Song" {
		t.Fatal("Older completion stopped the current song", err)
	}
	completed := strings.ReplaceAll(strings.ReplaceAll(liveExample, "1793937600", "1793937900"), `"Song"`, `"New Song"`)
	captureExample(t, s, completed, "")
	track, _, err = s.LiveNowPlaying(ctx, 1, newEntry.ReceivedAt.Add(time.Second))
	if err != nil || track != nil {
		t.Fatal("Matching completion did not clear playback", err)
	}
}

func TestLiveFailedPublicationKeepsAcceptedRequest(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	entry := captureExample(t, s, liveExample, "")
	if _, err := s.DB.Exec(`CREATE TRIGGER fail_live_projection BEFORE INSERT ON source_projection BEGIN SELECT RAISE(ABORT,'publication test failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := s.ProcessLive(ctx); err == nil {
		t.Fatal("Expected publication failure")
	}
	if count(t, s, "source_records") != 1 || count(t, s, "listens") != 0 {
		t.Fatal("Publication failure lost accepted evidence or partially published")
	}
	if _, err := s.ReadLiveRequest(entry); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`DROP TRIGGER fail_live_projection`); err != nil {
		t.Fatal(err)
	}
	if err := s.ProcessLive(ctx); err != nil {
		t.Fatal(err)
	}
	if count(t, s, "source_records") != 1 || count(t, s, "listens") != 1 {
		t.Fatal("Recovery lost or multiplied accepted data")
	}
}

func TestLiveCaptureStorageFailure(t *testing.T) {
	s := testStore(t)
	blocked := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocked, []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	s.Root = blocked
	if _, err := s.CaptureLive(context.Background(), []byte(liveExample), 1, "", ""); err == nil {
		t.Fatal("Acknowledged a request without durable storage")
	}
}

func TestLiveProtocolValidation(t *testing.T) {
	for _, data := range []string{`null`, `{}`, `{"listen_type":"invalid","payload":[]}`, strings.ReplaceAll(liveExample, "1793937600", "0"), liveExample + `{}`} {
		if _, err := DecodeLive([]byte(data)); err == nil {
			t.Errorf("Accepted invalid submission: %s", data)
		}
	}
	if _, err := DecodeLive([]byte(`{"listen_type":"playing_now","payload":[]}`)); err != nil {
		t.Fatal("Stop update rejected", err)
	}
}

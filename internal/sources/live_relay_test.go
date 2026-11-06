package sources

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLiveRelayRetriesOriginalBytesAfterPublication(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	attempts := 0
	identifiers := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		body, err := io.ReadAll(r.Body)
		if err != nil || !bytes.Equal(body, []byte(liveExample)) {
			t.Error("Relay changed or lost the request body", err)
		}
		if r.Header.Get("Authorization") != "Token relay-secret" {
			t.Error("Missing relay authentication")
		}
		identifiers = append(identifiers, r.Header.Get("Idempotency-Key"))
		if attempts == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	entry, err := s.CaptureLive(ctx, []byte(liveExample), 1, "", "test", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ProcessLive(ctx); err != nil {
		t.Fatal(err)
	}
	if err = s.ForwardLive(ctx, server.URL, "relay-secret"); err == nil {
		t.Fatal("Expected relay failure")
	}
	if s.liveRelayDone(entry) {
		t.Fatal("Marked failed relay complete")
	}
	if err = s.ProcessLive(ctx); err != nil {
		t.Fatal(err)
	}
	if count(t, s, "source_projection_runs") != 1 {
		t.Fatal("Relay outage repeatedly rebuilt the same view")
	}
	pending, err := s.PendingLive(ctx)
	if err != nil || len(pending) != 1 {
		t.Fatal("Publication discarded the unfinished relay", err)
	}
	restarted := &Store{DB: s.DB, Root: s.Root}
	if err = restarted.ForwardLive(ctx, server.URL, "relay-secret"); err != nil {
		t.Fatal(err)
	}
	if !restarted.liveRelayDone(entry) {
		t.Fatal("Successful relay was not durable")
	}
	if len(identifiers) != 2 || identifiers[0] == "" || identifiers[0] != identifiers[1] {
		t.Fatal("Relay retries changed the delivery identifier")
	}
	if err = restarted.ForwardLive(ctx, server.URL, "relay-secret"); err != nil {
		t.Fatal(err)
	}
	if attempts != 2 {
		t.Fatal("Successful relay was sent again")
	}
	if err = restarted.ProcessLive(ctx); err != nil {
		t.Fatal(err)
	}
	pending, err = restarted.PendingLive(ctx)
	if err != nil || len(pending) != 0 {
		t.Fatal("Completed relay remained queued", err)
	}
	if count(t, s, "source_records") != 1 || count(t, s, "listens") != 1 {
		t.Fatal("Relay recovery duplicated the play")
	}
}

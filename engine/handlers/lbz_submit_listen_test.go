package handlers

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gabehf/koito/engine/middleware"
	"github.com/gabehf/koito/internal/models"
	"github.com/gabehf/koito/internal/sources"
)

func TestLiveHTTPAcknowledgesOnlyDurableSubmission(t *testing.T) {
	store := &sources.Store{Root: t.TempDir()}
	handler := preservedListenHandler(store)
	body := []byte(` {"listen_type":"single","payload":[{"listened_at":1793937600,"track_metadata":{"artist_name":"Artist","track_name":"Song","new_field":{"keep":true}}}],"new_envelope":42} `)
	send := func(data []byte, authenticated bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/apis/listenbrainz/1/submit-listens", bytes.NewReader(data))
		req.Header.Set("Authorization", "Token secret-not-for-storage")
		if authenticated {
			req = req.WithContext(context.WithValue(req.Context(), middleware.UserContextKey, &models.User{ID: 1, Role: models.UserRoleAdmin}))
		}
		response := httptest.NewRecorder()
		handler(response, req)
		return response
	}
	if response := send(body, false); response.Code != http.StatusUnauthorized {
		t.Fatal("Unauthorized submission accepted", response.Code)
	}
	if response := send(body, true); response.Code != http.StatusOK || !strings.Contains(response.Header().Get("Content-Type"), "json") {
		t.Fatal("Valid submission failed", response.Code, response.Body.String())
	}
	entries, err := store.PendingLive(context.Background())
	if err != nil || len(entries) != 1 {
		t.Fatal("Acknowledged request was not journaled", err)
	}
	original, err := store.ReadLiveRequest(entries[0])
	if err != nil || !bytes.Equal(original, body) {
		t.Fatal("Unknown envelope or record fields lost", err)
	}
	journal, err := os.ReadFile(filepath.Join(store.Root, "live", "pending", entries[0].ID+".json"))
	if err != nil || bytes.Contains(journal, []byte("secret-not-for-storage")) {
		t.Fatal("Stored authentication secret", err)
	}
	if response := send([]byte(`{"listen_type":"single","payload":[]}`), true); response.Code != http.StatusBadRequest {
		t.Fatal("Invalid submission accepted", response.Code)
	}
	if response := send(bytes.Repeat([]byte(" "), sources.MaxLiveRequest+1), true); response.Code != http.StatusRequestEntityTooLarge {
		t.Fatal("Oversized submission accepted", response.Code)
	}
	blocked := filepath.Join(t.TempDir(), "file")
	if err = os.WriteFile(blocked, []byte("blocked"), 0600); err != nil {
		t.Fatal(err)
	}
	store.Root = blocked
	if response := send(body, true); response.Code != http.StatusServiceUnavailable {
		t.Fatal("Storage failure was acknowledged", response.Code)
	}
}

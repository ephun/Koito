package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gabehf/koito/engine/middleware"
	"github.com/gabehf/koito/internal/db/sqlite"
	"github.com/gabehf/koito/internal/models"
	"github.com/gabehf/koito/internal/sources"
)

func TestImportsHTTP(t *testing.T) {
	db, err := sqlite.NewInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close(context.Background())
	if err = db.Exec(`INSERT INTO users(id,username,role,password) VALUES(1,'test','admin',x'00')`); err != nil {
		t.Fatal(err)
	}
	store := &sources.Store{DB: db.SourceDatabase(), Root: t.TempDir()}
	handler := ImportsHandler(store)
	for _, role := range []models.UserRole{"", models.UserRoleUser} {
		req := httptest.NewRequest("GET", "/apis/web/v1/imports", nil)
		if role != "" {
			req = req.WithContext(context.WithValue(req.Context(), middleware.UserContextKey, &models.User{ID: 1, Role: role}))
		}
		res := httptest.NewRecorder()
		handler(res, req)
		if res.Code != http.StatusForbidden {
			t.Fatalf("evidence accessible to %q", role)
		}
	}
	input := `[{"ts":"2020-01-02T03:04:05Z","ms_played":123,"master_metadata_track_name":"Song","master_metadata_album_artist_name":"Artist","reason_end":"trackdone"}]`
	for i := 0; i < 2; i++ {
		var body bytes.Buffer
		form := multipart.NewWriter(&body)
		form.WriteField("account", "me")
		part, e := form.CreateFormFile("file", "history.json")
		if e != nil {
			t.Fatal(e)
		}
		part.Write([]byte(input))
		form.Close()
		req := httptest.NewRequest("POST", "/apis/web/v1/imports", &body)
		req.Header.Set("Content-Type", form.FormDataContentType())
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserContextKey, &models.User{ID: 1, Role: models.UserRoleAdmin}))
		res := httptest.NewRecorder()
		handler(res, req)
		if res.Code != 200 {
			t.Fatal(res.Code, res.Body.String())
		}
		var result sources.ImportResult
		if e = json.Unmarshal(res.Body.Bytes(), &result); e != nil {
			t.Fatal(e)
		}
		if (i == 0 && result.Added != 1) || (i == 1 && result.Added != 0) {
			t.Fatal("HTTP import is not idempotent", result)
		}
	}
	for _, url := range []string{"/apis/web/v1/imports", "/apis/web/v1/imports/records?source=1"} {
		req := httptest.NewRequest("GET", url, nil).WithContext(context.WithValue(context.Background(), middleware.UserContextKey, &models.User{ID: 1, Role: models.UserRoleAdmin}))
		res := httptest.NewRecorder()
		handler(res, req)
		if res.Code != 200 {
			t.Fatal(res.Code, res.Body.String())
		}
	}
}

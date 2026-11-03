package handlers

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gabehf/koito/engine/middleware"
	"github.com/gabehf/koito/internal/db/sqlite"
	"github.com/gabehf/koito/internal/models"
	"github.com/gabehf/koito/internal/sources"
)

func TestFiltersHTTP(t *testing.T) {
	database, err := sqlite.NewInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close(context.Background())
	if err = database.Exec(`INSERT INTO users(id,username,role,password) VALUES(1,'test','admin',x'00')`); err != nil {
		t.Fatal(err)
	}
	store := &sources.Store{DB: database.SourceDatabase(), Root: t.TempDir()}
	original := `[{"name":"Song","artist":{"#text":"Artist"},"date":{"uts":"1577934065"},"unknown":{"retained":true}}]`
	upload, err := store.Import(context.Background(), strings.NewReader(original), "lastfm.json", 1)
	if err != nil {
		t.Fatal(err)
	}
	handler := FiltersHandler(store)
	request := func(method, path, body string, admin bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "/apis/web/v1/filters"+path, strings.NewReader(body))
		if admin {
			req = req.WithContext(context.WithValue(req.Context(), middleware.UserContextKey, &models.User{ID: 1, Role: models.UserRoleAdmin}))
		}
		response := httptest.NewRecorder()
		handler(response, req)
		return response
	}
	for _, path := range []string{"", "/plays", "/record?id=1"} {
		if res := request("GET", path, "", false); res.Code != 403 {
			t.Fatal("unauthenticated access", path, res.Code)
		}
	}
	for _, path := range []string{"/preview", "/apply"} {
		if res := request("POST", path, "{}", false); res.Code != 403 {
			t.Fatal("unauthenticated mutation", res.Code)
		}
	}
	res := request("GET", "", "", true)
	if res.Code != 200 {
		t.Fatal(res.Body.String())
	}
	var state sources.FilterState
	if err = json.Unmarshal(res.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	change := sources.FilterChange{BaseVersion: state.Current.ID, Config: state.Current.Config}
	raw, _ := json.Marshal(change)
	res = request("POST", "/preview", string(raw), true)
	if res.Code != 200 {
		t.Fatal(res.Code, res.Body.String())
	}
	var preview sources.FilterPreview
	if err = json.Unmarshal(res.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	change.Token = preview.Token
	raw, _ = json.Marshal(change)
	res = request("POST", "/apply", string(raw), true)
	if res.Code != 200 {
		t.Fatal(res.Code, res.Body.String())
	}
	res = request("POST", "/apply", string(raw), true)
	if res.Code != 409 {
		t.Fatal("stale preview accepted", res.Code, res.Body.String())
	}
	if res = request("POST", "/preview", `{"unknown":true}`, true); res.Code != 400 {
		t.Fatal("invalid change accepted")
	}
	fileRequest := httptest.NewRequest("GET", "/apis/web/v1/imports/file?hash="+upload.Hash, nil)
	fileRequest = fileRequest.WithContext(context.WithValue(fileRequest.Context(), middleware.UserContextKey, &models.User{ID: 1, Role: models.UserRoleAdmin}))
	fileResponse := httptest.NewRecorder()
	ImportsHandler(store)(fileResponse, fileRequest)
	if fileResponse.Code != 200 || fileResponse.Body.String() != original {
		t.Fatal("applying filters changed original download")
	}
}

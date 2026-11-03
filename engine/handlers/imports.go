package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gabehf/koito/engine/middleware"
	"github.com/gabehf/koito/internal/sources"
)

// ImportsHandler is additionally mounted behind session/API authentication.
// Evidence endpoints require admin access even when the public dashboard is open.
func ImportsHandler(store *sources.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.GetUserFromContext(r.Context())
		if user == nil || user.Role != "admin" {
			http.Error(w, "administrator login required", http.StatusForbidden)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		var value any
		var err error
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/apis/web/v1/imports":
			receipts, e := store.Receipts(r.Context())
			if e != nil {
				err = e
				break
			}
			summary, e := store.Summary(r.Context())
			err = e
			value = map[string]any{"receipts": receipts, "sources": summary, "policy": sources.Policy}
		case r.Method == http.MethodGet && r.URL.Path == "/apis/web/v1/imports/records":
			id, e := strconv.ParseInt(r.URL.Query().Get("source"), 10, 64)
			if e != nil || id <= 0 {
				http.Error(w, "source is required", 400)
				return
			}
			after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
			value, err = store.Records(r.Context(), id, after)
		case r.Method == http.MethodGet && r.URL.Path == "/apis/web/v1/imports/file":
			hash := r.URL.Query().Get("hash")
			path, e := store.FilePath(hash)
			if e != nil {
				http.Error(w, "invalid file", 400)
				return
			}
			var exists int
			if e = store.DB.QueryRowContext(r.Context(), `SELECT 1 FROM source_files WHERE hash=?`, hash).Scan(&exists); e != nil {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Disposition", `attachment; filename="`+hash+`"`)
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			http.ServeFile(w, r, path)
			return
		case r.Method == http.MethodPost && r.URL.Path == "/apis/web/v1/imports/rebuild":
			value, err = store.Rebuild(r.Context())
		case r.Method == http.MethodPost && r.URL.Path == "/apis/web/v1/imports":
			r.Body = http.MaxBytesReader(w, r.Body, sources.MaxFileSize+(1<<20))
			if e := r.ParseMultipartForm(8 << 20); e != nil {
				http.Error(w, "invalid upload or upload too large", 400)
				return
			}
			defer r.MultipartForm.RemoveAll()
			file, header, e := r.FormFile("file")
			if e != nil {
				http.Error(w, "file is required", 400)
				return
			}
			defer file.Close()
			value, err = store.Import(r.Context(), file, header.Filename, user.ID)
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnprocessableEntity)
				json.NewEncoder(w).Encode(map[string]any{"error": err.Error(), "import": value})
				return
			}
		default:
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		json.NewEncoder(w).Encode(value)
	}
}

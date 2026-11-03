package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gabehf/koito/engine/middleware"
	"github.com/gabehf/koito/internal/sources"
)

func FiltersHandler(store *sources.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.GetUserFromContext(r.Context())
		if user == nil || user.Role != "admin" {
			http.Error(w, "Administrator login required.", http.StatusForbidden)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		var value any
		var err error
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/apis/web/v1/filters":
			value, err = store.Filters(r.Context())
		case r.Method == http.MethodGet && r.URL.Path == "/apis/web/v1/filters/plays":
			after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
			afterKey := ""
			if cursor := r.URL.Query().Get("cursor"); cursor != "" {
				left, right, ok := strings.Cut(cursor, ":")
				parsed, e := strconv.ParseInt(left, 10, 64)
				if !ok || e != nil || parsed <= 0 || len(right) != 64 {
					http.Error(w, "Invalid play cursor.", 400)
					return
				}
				after = parsed
				afterKey = right
			}
			value, err = store.PlaysAfter(r.Context(), after, afterKey, r.URL.Query().Get("q"), r.URL.Query().Get("ambiguous") == "true")
		case r.Method == http.MethodGet && r.URL.Path == "/apis/web/v1/filters/record":
			id, e := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
			if e != nil || id <= 0 {
				w.WriteHeader(400)
				json.NewEncoder(w).Encode(map[string]string{"error": "Record ID is required."})
				return
			}
			value, err = store.Inspect(r.Context(), id)
		case r.Method == http.MethodPost && (r.URL.Path == "/apis/web/v1/filters/preview" || r.URL.Path == "/apis/web/v1/filters/apply" || r.URL.Path == "/apis/web/v1/filters/save"):
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
			decoder := json.NewDecoder(r.Body)
			decoder.DisallowUnknownFields()
			var change sources.FilterChange
			if e := decoder.Decode(&change); e != nil {
				w.WriteHeader(400)
				json.NewEncoder(w).Encode(map[string]string{"error": "Invalid filter change."})
				return
			}
			if r.URL.Path == "/apis/web/v1/filters/preview" {
				value, err = store.Preview(r.Context(), change)
			} else if r.URL.Path == "/apis/web/v1/filters/save" {
				value, err = store.Save(r.Context(), change)
			} else {
				value, err = store.Apply(r.Context(), change)
			}
		default:
			http.NotFound(w, r)
			return
		}
		if err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, sources.ErrStalePreview) {
				status = http.StatusConflict
			}
			w.WriteHeader(status)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		json.NewEncoder(w).Encode(value)
	}
}

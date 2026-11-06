package handlers

import (
	"context"
	"github.com/gabehf/koito/internal/db"
	"github.com/gabehf/koito/internal/utils"
	"net/http"
)

func GetGenresHandler(store db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		reader, ok := store.(interface {
			GetGenres(context.Context) ([]string, error)
		})
		if !ok {
			utils.WriteError(w, "genres unavailable", http.StatusNotImplemented)
			return
		}
		genres, err := reader.GetGenres(r.Context())
		if err != nil {
			utils.WriteError(w, "could not load genres", http.StatusInternalServerError)
			return
		}
		utils.WriteJSON(w, http.StatusOK, genres)
	}
}

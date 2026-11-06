package handlers

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gabehf/koito/internal/db"
	"github.com/gabehf/koito/internal/logger"
	"github.com/gabehf/koito/internal/models"
	"github.com/gabehf/koito/internal/utils"
)

func GetAlbumGroupHandler(store db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := utils.ParseIDParam(r, "id")
		if err != nil {
			utils.WriteError(w, "invalid album group identifier", http.StatusBadRequest)
			return
		}
		groups, ok := store.(interface {
			GetAlbumGroup(context.Context, int32) (*models.Album, error)
		})
		if !ok {
			utils.WriteError(w, "album groups unavailable", http.StatusNotImplemented)
			return
		}
		album, err := groups.GetAlbumGroup(r.Context(), id)
		if err != nil {
			utils.WriteError(w, "album group could not be found", http.StatusNotFound)
			return
		}
		utils.WriteJSON(w, http.StatusOK, album)
	}
}

func GetAlbumGroupInterestHandler(store db.ListenStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := utils.ParseIDParam(r, "id")
		if err != nil {
			utils.WriteError(w, "invalid album group identifier", http.StatusBadRequest)
			return
		}
		buckets, err := strconv.Atoi(r.URL.Query().Get("buckets"))
		if err != nil || buckets < 1 || buckets > 500 {
			utils.WriteError(w, "invalid bucket count", http.StatusBadRequest)
			return
		}
		interest, err := store.GetInterest(r.Context(), db.GetInterestOpts{AlbumGroupID: id, Buckets: buckets})
		if err != nil {
			utils.WriteError(w, "could not retrieve interest", http.StatusInternalServerError)
			return
		}
		utils.WriteJSON(w, http.StatusOK, interest)
	}
}

func GetAlbumHandler(store db.AlbumStore) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		l := logger.FromContext(ctx)

		l.Debug().Msg("GetAlbumHandler: Received request to retrieve album")

		id, err := utils.ParseIDParam(r, "id")
		if err != nil {
			l.Debug().AnErr("error", err).Msg("GetAlbumHandler: Invalid album id")
			utils.WriteError(w, "invalid album id", http.StatusBadRequest)
			return
		}

		l.Debug().Msgf("GetAlbumHandler: Retrieving album with ID %d", id)

		album, err := store.GetAlbum(ctx, db.GetAlbumOpts{ID: id})
		if err != nil {
			l.Err(err).Msgf("GetAlbumHandler: Failed to retrieve album with ID %d", id)
			utils.WriteError(w, "album with specified id could not be found", http.StatusNotFound)
			return
		}

		l.Debug().Msgf("GetAlbumHandler: Successfully retrieved album with ID %d", id)
		utils.WriteJSON(w, http.StatusOK, album)
	}
}

// GetAlbumAliasesHandler retrieves all aliases for a given album.
func GetAlbumAliasesHandler(store db.AlbumStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		l := logger.FromContext(ctx)

		albumID, err := utils.ParseIDParam(r, "id")
		if err != nil {
			l.Debug().AnErr("error", err).Msg("GetAlbumAliasesHandler: Invalid album id")
			utils.WriteError(w, "invalid album id", http.StatusBadRequest)
			return
		}

		aliases, err := store.GetAllAlbumAliases(ctx, albumID)
		if err != nil {
			l.Err(err).Msg("GetAlbumAliasesHandler: Failed to get album aliases")
			utils.WriteError(w, "failed to retrieve aliases", http.StatusInternalServerError)
			return
		}

		utils.WriteJSON(w, http.StatusOK, aliases)
	}
}

// GetArtistsForAlbumHandler retrieves all artists for a given album.
func GetArtistsForAlbumHandler(store db.ArtistStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		l := logger.FromContext(ctx)

		albumID, err := utils.ParseIDParam(r, "id")
		if err != nil {
			l.Debug().AnErr("error", err).Msg("GetArtistsForAlbumHandler: Invalid album id")
			utils.WriteError(w, "invalid album id", http.StatusBadRequest)
			return
		}

		artists, err := store.GetArtistsForAlbum(ctx, albumID)
		if err != nil {
			l.Err(err).Msg("GetArtistsForAlbumHandler: Failed to retrieve artists")
			utils.WriteError(w, "failed to retrieve artists", http.StatusInternalServerError)
			return
		}

		utils.WriteJSON(w, http.StatusOK, artists)
	}
}

// GetAlbumInterestHandler retrieves interest data for a given album.
func GetAlbumInterestHandler(store db.ListenStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		l := logger.FromContext(ctx)

		albumID, err := utils.ParseIDParam(r, "id")
		if err != nil {
			l.Debug().AnErr("error", err).Msg("GetAlbumInterestHandler: Invalid album id")
			utils.WriteError(w, "invalid album id", http.StatusBadRequest)
			return
		}

		buckets, err := strconv.Atoi(r.URL.Query().Get("buckets"))
		if err != nil {
			l.Debug().Msg("GetAlbumInterestHandler: Buckets is not an integer")
			utils.WriteError(w, "parameter 'buckets' must be an integer", http.StatusBadRequest)
			return
		}

		interest, err := store.GetInterest(ctx, db.GetInterestOpts{
			AlbumID: albumID,
			Buckets: buckets,
		})
		if err != nil {
			l.Err(err).Msg("GetAlbumInterestHandler: Failed to query interest")
			utils.WriteError(w, "failed to retrieve interest: "+err.Error(), http.StatusInternalServerError)
			return
		}

		utils.WriteJSON(w, http.StatusOK, interest)
	}
}

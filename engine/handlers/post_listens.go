package handlers

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"time"

	"github.com/gabehf/koito/engine/middleware"
	"github.com/gabehf/koito/internal/cfg"
	"github.com/gabehf/koito/internal/db"
	"github.com/gabehf/koito/internal/logger"
	"github.com/gabehf/koito/internal/sources"
	"github.com/gabehf/koito/internal/utils"
)

func SubmitListenWithIDHandler(store db.ListenStore) http.HandlerFunc {
	var defaultClientStr = "Koito Web UI"
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		l := logger.FromContext(ctx)
		l.Debug().Msg("SubmitListenWithIDHandler: Got request")

		u := middleware.GetUserFromContext(ctx)
		if u == nil {
			l.Debug().Msg("SubmitListenWithIDHandler: Unauthorized request (user context is nil)")
			utils.WriteError(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		raw, err := io.ReadAll(io.LimitReader(r.Body, sources.MaxLiveRequest+1))
		if err != nil || len(raw) > sources.MaxLiveRequest {
			utils.WriteError(w, "Invalid submission", http.StatusBadRequest)
			return
		}
		var body struct {
			TrackID int32  `json:"track_id"`
			Unix    int64  `json:"unix"`
			Client  string `json:"client"`
		}
		err = json.Unmarshal(raw, &body)
		if err != nil || body.TrackID <= 0 || body.Unix <= 0 {
			l.Debug().Msg("SubmitListenWithIDHandler: Invalid or missing required fields in request body")
			utils.WriteError(w, "track_id and unix (timestamp) must be provided", http.StatusBadRequest)
			return
		}

		if time.Now().Unix() < body.Unix {
			l.Debug().Msg("SubmitListenWithIDHandler: Timestamp is in the future")
			utils.WriteError(w, "invalid timestamp", http.StatusBadRequest)
			return
		}

		client := body.Client
		if client == "" {
			client = defaultClientStr
		}
		provider, ok := store.(interface{ SourceDatabase() *sql.DB })
		tracks, trackOK := store.(db.TrackStore)
		if ok && trackOK {
			track, err := tracks.GetTrack(ctx, db.GetTrackOpts{ID: body.TrackID})
			if err != nil || track == nil || len(track.Artists) == 0 {
				utils.WriteError(w, "Track is unavailable", http.StatusBadRequest)
				return
			}
			albumTitle := ""
			for _, album := range track.Albums {
				if album.ID == track.AlbumID {
					albumTitle = album.Title
					break
				}
			}
			preserved := &sources.Store{DB: provider.SourceDatabase(), Root: filepath.Join(cfg.ConfigDir(), "sources")}
			file, err := preserved.SaveFile(ctx, bytes.NewReader(raw))
			if err != nil {
				utils.WriteError(w, "Could not preserve submission", http.StatusServiceUnavailable)
				return
			}
			payload := map[string]any{"listen_type": "single", "payload": []any{map[string]any{"listened_at": body.Unix, "track_metadata": map[string]any{"artist_name": track.Artists[0].Name, "track_name": track.Title, "release_name": albumTitle, "additional_info": map[string]any{"music_service_name": "Koito", "submission_client": client, "duration": track.Duration, "event_id": "manual:" + file.Hash, "original_file_hash": file.Hash, "original_request": string(raw), "selected_track": track}}}}}
			encoded, err := json.Marshal(payload)
			if err == nil {
				_, err = preserved.CaptureLive(ctx, encoded, u.ID, "manual:"+file.Hash, r.UserAgent())
			}
			if err != nil {
				utils.WriteError(w, "Could not preserve submission", http.StatusServiceUnavailable)
				return
			}
			w.WriteHeader(http.StatusCreated)
			return
		}

		if err = store.SaveListen(ctx, db.SaveListenOpts{
			TrackID: body.TrackID,
			Time:    time.Unix(body.Unix, 0),
			UserID:  u.ID,
			Client:  client,
		}); err != nil {
			l.Err(err).Msg("SubmitListenWithIDHandler: Failed to submit listen")
			utils.WriteError(w, "failed to submit listen", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusCreated)
	}
}

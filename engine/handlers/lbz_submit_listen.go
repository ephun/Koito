package handlers

import (
	"database/sql"
	"io"
	"net/http"
	"path/filepath"

	"github.com/gabehf/koito/engine/middleware"
	"github.com/gabehf/koito/internal/cfg"
	"github.com/gabehf/koito/internal/db"
	"github.com/gabehf/koito/internal/logger"
	"github.com/gabehf/koito/internal/mbz"
	"github.com/gabehf/koito/internal/sources"
	"github.com/gabehf/koito/internal/utils"
)

type LbzListenType string

const (
	ListenTypeSingle     LbzListenType = "single"
	ListenTypePlayingNow LbzListenType = "playing_now"
	ListenTypeImport     LbzListenType = "import"
)

type LbzSubmitListenRequest struct {
	ListenType LbzListenType            `json:"listen_type,omitempty"`
	Payload    []LbzSubmitListenPayload `json:"payload,omitempty"`
}

type LbzSubmitListenPayload struct {
	ListenedAt int64        `json:"listened_at,omitempty"`
	TrackMeta  LbzTrackMeta `json:"track_metadata"`
}

type LbzTrackMeta struct {
	ArtistName     string            `json:"artist_name"` // required
	TrackName      string            `json:"track_name"`  // required
	ReleaseName    string            `json:"release_name,omitempty"`
	MBIDMapping    LbzMBIDMapping    `json:"mbid_mapping"`
	AdditionalInfo LbzAdditionalInfo `json:"additional_info,omitempty"`
}
type LbzArtist struct {
	ArtistMBID string `json:"artist_mbid"`
	ArtistName string `json:"artist_credit_name"`
}
type LbzMBIDMapping struct {
	ReleaseMBID   string      `json:"release_mbid"`
	RecordingMBID string      `json:"recording_mbid"`
	ArtistMBIDs   []string    `json:"artist_mbids"`
	Artists       []LbzArtist `json:"artists"`
}

type LbzAdditionalInfo struct {
	MediaPlayer             string   `json:"media_player,omitempty"`
	SubmissionClient        string   `json:"submission_client,omitempty"`
	SubmissionClientVersion string   `json:"submission_client_version,omitempty"`
	ReleaseMBID             string   `json:"release_mbid,omitempty"`
	ReleaseGroupMBID        string   `json:"release_group_mbid,omitempty"`
	ArtistMBIDs             []string `json:"artist_mbids,omitempty"`
	ArtistNames             []string `json:"artist_names,omitempty"`
	RecordingMBID           string   `json:"recording_mbid,omitempty"`
	DurationMs              int32    `json:"duration_ms,omitempty"`
	Duration                int32    `json:"duration,omitempty"`
	Tags                    []string `json:"tags,omitempty"`
	AlbumArtist             string   `json:"albumartist,omitempty"`
}

const (
	maxListensPerRequest = 1000
)

type submitListenHandlerStore interface {
	db.ArtistStore
	db.AlbumStore
	db.TrackStore
	db.ListenStore
}

func LbzSubmitListenHandler(store submitListenHandlerStore, _ mbz.MusicBrainzCaller) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		provider, ok := store.(interface{ SourceDatabase() *sql.DB })
		if !ok {
			utils.WriteError(w, "Live storage is unavailable", http.StatusServiceUnavailable)
			return
		}
		preserved := &sources.Store{DB: provider.SourceDatabase(), Root: filepath.Join(cfg.ConfigDir(), "sources")}
		relayURL := ""
		if cfg.LbzRelayEnabled() {
			relayURL = cfg.LbzRelayUrl()
		}
		preservedListenHandler(preserved, relayURL)(w, r)
	}
}

func preservedListenHandler(preserved *sources.Store, relayURL ...string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.GetUserFromContext(r.Context())
		if user == nil {
			utils.WriteError(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, sources.MaxLiveRequest+1))
		if err != nil {
			utils.WriteError(w, "Could not read submission", http.StatusBadRequest)
			return
		}
		if len(body) > sources.MaxLiveRequest {
			utils.WriteError(w, "Submission is too large", http.StatusRequestEntityTooLarge)
			return
		}
		if _, err = sources.DecodeLive(body); err != nil {
			utils.WriteError(w, err.Error(), http.StatusBadRequest)
			return
		}
		if _, err = preserved.CaptureLive(r.Context(), body, user.ID, r.Header.Get("Idempotency-Key"), r.UserAgent(), relayURL...); err != nil {
			logger.FromContext(r.Context()).Error().Err(err).Msg("Could not preserve live submission")
			utils.WriteError(w, "Could not preserve submission. Retry this request.", http.StatusServiceUnavailable)
			return
		}
		utils.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"github.com/gabehf/koito/internal/db"
	"github.com/gabehf/koito/internal/logger"
	"github.com/gabehf/koito/internal/memkv"
	"github.com/gabehf/koito/internal/utils"
)

type StatsResponse struct {
	ListenCount     int64   `json:"listen_count"`
	UndatedCount    int64   `json:"undated_count"`
	TrackCount      int64   `json:"track_count"`
	AlbumCount      int64   `json:"album_count"`
	ArtistCount     int64   `json:"artist_count"`
	MinutesListened int64   `json:"minutes_listened"`
	DaysActive      int     `json:"days_active"`
	LongestStreak   int     `json:"longest_streak"`
	AvgDailyPlays   float32 `json:"avg_daily_plays"`
	TracksPerArtist float32 `json:"tracks_per_artist"`
	AlbumsPerArtist float32 `json:"albums_per_artist"`
}

type statsStore interface {
	db.ListenStore
	db.TrackStore
	db.AlbumStore
	db.ArtistStore
}

func StatsHandler(store statsStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := logger.FromContext(r.Context())

		l.Debug().Msg("StatsHandler: Received request to retrieve statistics")

		tf := TimeframeFromRequest(r)

		l.Debug().Msg("StatsHandler: Fetching statistics")

		cacheKeyString := fmt.Sprintf("stats_%s_%s", r.URL.Query().Encode(), tf.Timezone.String())
		if provider, ok := store.(interface{ SourceDatabase() *sql.DB }); ok {
			var run int64
			if err := provider.SourceDatabase().QueryRowContext(r.Context(), `SELECT COALESCE(MAX(id),0) FROM source_projection_runs`).Scan(&run); err != nil {
				utils.WriteError(w, "Could not load statistics.", 500)
				return
			}
			cacheKeyString += fmt.Sprintf("_view_%d", run)
		}

		if cachedStatsI, ok := memkv.Store.Get(cacheKeyString); ok {
			if cachedStats, ok := cachedStatsI.(*StatsResponse); ok {
				l.Debug().Msg("StatsHandler: got stats from cache")
				utils.WriteJSON(w, http.StatusOK, cachedStats)
				return
			}
		}

		l.Debug().Msg("StatsHandler: cache missed for stats")

		listens, err := store.CountListens(r.Context(), tf)
		if err != nil {
			l.Err(err).Msg("StatsHandler: Failed to fetch listen count")
			utils.WriteError(w, "failed to get listens: "+err.Error(), http.StatusInternalServerError)
			return
		}

		tracks, err := store.CountTracks(r.Context(), tf)
		if err != nil {
			l.Err(err).Msg("StatsHandler: Failed to fetch track count")
			utils.WriteError(w, "failed to get tracks: "+err.Error(), http.StatusInternalServerError)
			return
		}

		albums, err := store.CountAlbums(r.Context(), tf)
		if err != nil {
			l.Err(err).Msg("StatsHandler: Failed to fetch album count")
			utils.WriteError(w, "failed to get albums: "+err.Error(), http.StatusInternalServerError)
			return
		}

		artists, err := store.CountArtists(r.Context(), tf)
		if err != nil {
			l.Err(err).Msg("StatsHandler: Failed to fetch artist count")
			utils.WriteError(w, "failed to get artists: "+err.Error(), http.StatusInternalServerError)
			return
		}

		timeListenedS, err := store.CountTimeListened(r.Context(), tf)
		if err != nil {
			l.Err(err).Msg("StatsHandler: Failed to fetch time listened")
			utils.WriteError(w, "failed to get time listened: "+err.Error(), http.StatusInternalServerError)
			return
		}

		activeDays, err := store.GetActiveDays(r.Context(), tf.Timezone)
		if err != nil {
			l.Err(err).Msg("StatsHandler: Failed to fetch active days")
			utils.WriteError(w, "failed to get active days: "+err.Error(), http.StatusInternalServerError)
			return
		}

		longestStreak, err := store.GetLongestListenStreak(r.Context(), db.ListenActivityOpts{Timezone: tf.Timezone})
		if err != nil {
			l.Err(err).Msg("StatsHandler: Failed to fetch longest streak")
			utils.WriteError(w, "failed to get longest streak: "+err.Error(), http.StatusInternalServerError)
			return
		}

		l.Debug().Msg("StatsHandler: Successfully fetched statistics")
		var undated int64
		if counter, ok := store.(interface {
			CountUndatedListens(context.Context, db.Timeframe) (int64, error)
		}); ok {
			undated, err = counter.CountUndatedListens(r.Context(), tf)
			if err != nil {
				utils.WriteError(w, "Could not load undated totals.", 500)
				return
			}
		}

		resp := &StatsResponse{
			ListenCount:     listens,
			UndatedCount:    undated,
			TrackCount:      tracks,
			AlbumCount:      albums,
			ArtistCount:     artists,
			MinutesListened: timeListenedS / 60,
			DaysActive:      activeDays,
			AvgDailyPlays:   statsRatio(listens-undated, int64(activeDays)),
			TracksPerArtist: statsRatio(tracks, artists),
			AlbumsPerArtist: statsRatio(albums, artists),
			LongestStreak:   longestStreak,
		}

		utils.WriteJSON(w, http.StatusOK, resp)

		// save to cache for 30 minutes
		memkv.Store.Set(cacheKeyString, resp, 30*time.Minute)
	}
}

func statsRatio(amount, total int64) float32 {
	if total == 0 {
		return 0
	}
	return float32(amount) / float32(total)
}

package sources

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"sort"
	"time"

	"github.com/gabehf/koito/internal/models"
)

// LiveNowPlaying combines the durable journal with committed deliveries. It
// remains current while publication is busy and recovers after process restarts.
func (s *Store) LiveNowPlaying(ctx context.Context, userID int32, now time.Time) (*models.Track, bool, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT d.id,d.file_hash,f.size,d.received_at,d.request_key,d.user_agent FROM live_submissions d JOIN source_accounts a ON a.id=d.source_id JOIN source_files f ON f.hash=d.file_hash WHERE a.user_id=? AND d.listen_type<>'import' AND d.received_at>=?`, userID, now.Add(-24*time.Hour).UTC().Format(time.RFC3339))
	if err != nil {
		return nil, false, err
	}
	entries := []LiveEnvelope{}
	seen := map[string]bool{}
	for rows.Next() {
		entry := LiveEnvelope{UserID: userID}
		var received string
		if err = rows.Scan(&entry.ID, &entry.File.Hash, &entry.File.Size, &received, &entry.RequestKey, &entry.UserAgent); err != nil {
			rows.Close()
			return nil, false, err
		}
		entry.ReceivedAt, err = time.Parse(time.RFC3339Nano, received)
		if err != nil {
			rows.Close()
			return nil, false, err
		}
		entries = append(entries, entry)
		seen[entry.ID] = true
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, false, err
	}
	pending, err := s.PendingLive(ctx)
	if err != nil {
		return nil, false, err
	}
	for _, entry := range pending {
		if entry.UserID == userID && !seen[entry.ID] {
			entries = append(entries, entry)
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].ReceivedAt.Equal(entries[j].ReceivedAt) {
			return entries[i].ID < entries[j].ID
		}
		return entries[i].ReceivedAt.Before(entries[j].ReceivedAt)
	})
	type state struct {
		value                 recordValues
		received, at, expires time.Time
	}
	states := map[string]state{}
	transport := map[string]bool{}
	handled := false
	for _, entry := range entries {
		data, err := s.ReadLiveRequest(entry)
		if err != nil {
			return nil, handled, err
		}
		request, err := DecodeLive(data)
		if err != nil {
			return nil, handled, err
		}
		if request.ListenType == "import" {
			continue
		}
		handled = true
		if entry.RequestKey != "" {
			key := entry.RequestKey + ":" + entry.File.Hash
			if transport[key] {
				continue
			}
			transport[key] = true
		}
		if len(request.Payload) == 0 {
			states = map[string]state{}
			continue
		}
		v := normalizeLegacyHistory(request.Payload[0], LiveListenBrainz)
		stream := v.OriginalService + "\x00" + v.Device + "\x00" + v.Forwarder
		previous, exists := states[stream]
		at := v.Time
		if at.IsZero() {
			at = entry.ReceivedAt
		}
		if request.ListenType == "single" {
			end := v.Time
			if v.DurationMS > 0 && v.DurationMS <= int64(24*time.Hour/time.Millisecond) {
				end = end.Add(time.Duration(v.DurationMS) * time.Millisecond)
			}
			if exists && v.Title == previous.value.Title && v.Artist == previous.value.Artist && (previous.value.Time.IsZero() || v.Time.IsZero() || !end.Before(previous.at)) {
				delete(states, stream)
			}
			continue
		}
		if exists && at.Before(previous.at) {
			continue
		}
		duration := 5 * time.Minute
		if v.DurationMS > 0 && v.DurationMS <= int64(24*time.Hour/time.Millisecond) {
			duration = time.Duration(v.DurationMS)*time.Millisecond + 15*time.Second
		}
		states[stream] = state{value: v, received: entry.ReceivedAt, at: at, expires: entry.ReceivedAt.Add(duration)}
	}
	var current *state
	for _, item := range states {
		if !item.expires.After(now) {
			continue
		}
		if current == nil || item.received.After(current.received) || item.received.Equal(current.received) && item.value.Title < current.value.Title {
			copy := item
			current = &copy
		}
	}
	if current == nil {
		return nil, handled, nil
	}
	track := &models.Track{Title: current.value.Title, Artists: []models.SimpleArtist{{Name: current.value.Artist}}}
	if seconds := current.value.DurationMS / 1000; seconds > 0 && seconds <= math.MaxInt32 {
		track.Duration = int32(seconds)
	}
	// Existing appearances supply links and artwork. A first-time track is still
	// visible immediately, before its catalog appearance has been published.
	var artistID, albumID, trackID int32
	err = s.DB.QueryRowContext(ctx, `SELECT t.id,t.release_id,at.artist_id FROM tracks t JOIN track_aliases ta ON ta.track_id=t.id JOIN artist_tracks at ON at.track_id=t.id JOIN artist_aliases aa ON aa.artist_id=at.artist_id JOIN release_aliases ra ON ra.release_id=t.release_id WHERE ta.alias=? AND aa.alias=? AND ra.alias=? ORDER BY ta.is_primary DESC,aa.is_primary DESC,t.id LIMIT 1`, current.value.Title, current.value.Artist, current.value.Album).Scan(&trackID, &albumID, &artistID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, handled, err
	}
	track.ID, track.AlbumID, track.Artists[0].ID = trackID, albumID, artistID
	return track, true, nil
}

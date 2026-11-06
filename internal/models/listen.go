package models

import (
	"time"
)

// a Listen is the same thing as a 'scrobble' but i despise the word scrobble so i will not use it
type Listen struct {
	ContextKind      string      `json:"context_kind,omitempty"`
	LocalDate        string      `json:"local_date,omitempty"`
	DateOnly         bool        `json:"date_only,omitempty"`
	TimeEstimated    bool        `json:"time_estimated,omitempty"`
	Tags             []string    `json:"tags,omitempty"`
	AlbumID          int32       `json:"album_id,omitempty"`
	Album            string      `json:"album,omitempty"`
	PlayKey          string      `json:"play_key,omitempty"`
	RecordID         int64       `json:"record_id,omitempty"`
	Evidence         string      `json:"evidence,omitempty"`
	AttributionBasis string      `json:"attribution_basis,omitempty"`
	Time             time.Time   `json:"time"`
	Track            SimpleTrack `json:"track"`
}

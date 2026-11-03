package models

import (
	"time"
)

// a Listen is the same thing as a 'scrobble' but i despise the word scrobble so i will not use it
type Listen struct {
	PlayKey  string      `json:"play_key,omitempty"`
	RecordID int64       `json:"record_id,omitempty"`
	Time     time.Time   `json:"time"`
	Track    SimpleTrack `json:"track"`
}

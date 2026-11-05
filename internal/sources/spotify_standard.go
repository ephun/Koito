package sources

import (
	"encoding/json"
	"time"
)

const SpotifyStandard = "spotify-standard-v1"

func parseSpotifyStandard(data []byte) ([]json.RawMessage, error) {
	var rows []json.RawMessage
	if json.Unmarshal(data, &rows) != nil || len(rows) == 0 {
		return nil, ErrUnsupportedFile
	}
	for _, row := range rows {
		var fields map[string]json.RawMessage
		if json.Unmarshal(row, &fields) != nil || fields["endTime"] == nil || fields["msPlayed"] == nil {
			return nil, ErrUnsupportedFile
		}
	}
	return rows, nil
}

func normalizeSpotifyStandard(raw json.RawMessage) recordValues {
	var row struct {
		End    string `json:"endTime"`
		Artist string `json:"artistName"`
		Title  string `json:"trackName"`
		Played int64  `json:"msPlayed"`
	}
	v := recordValues{Reason: "counted", Origin: "original", TimeMeaning: "end", Precision: int64(time.Minute)}
	if json.Unmarshal(raw, &row) != nil {
		v.Reason = "invalid fields"
		return v
	}
	v.Title, v.Artist, v.PlayedMS, v.TimeText = row.Title, row.Artist, row.Played, row.End
	// Spotify account exports document endTime as UTC. Retain the precision
	// supplied by the file rather than manufacturing seconds or identifiers.
	v.Time, v.Precision, v.Reason = interpretTime(row.End, "UTC")
	if v.PlayedMS < 0 {
		v.Reason = "invalid played duration"
	}
	if v.Title == "" || v.Artist == "" {
		v.Reason = "not an identified music track"
	}
	return v
}

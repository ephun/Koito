package sources

import (
	"encoding/json"
	"time"
)

// Adapt cumulative annotations for Count without changing the original fields.
// A database upload date does not establish its snapshot date.
func navidromeLibraryRecord(row databaseRecord) libraryEnvelope {
	media := object(row.Context["media"])
	values := map[string]any{
		"Persistent ID": row.Fields["item_id"], "Name": media["title"],
		"Artist": media["artist"], "Album": media["album"], "Play Count": row.Fields["play_count"],
	}
	if text := valueText(row.Fields["play_date"]); text != "" {
		for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999999Z07:00", "2006-01-02 15:04:05.999999999-07:00"} {
			if date, err := time.Parse(layout, text); err == nil && date.Year() >= 1970 {
				values["Play Date UTC"] = date.UTC().Format(time.RFC3339Nano)
				break
			}
		}
	}
	fields := map[string]json.RawMessage{}
	for key, value := range values {
		fields[key], _ = json.Marshal(value)
	}
	return libraryEnvelope{Kind: row.Kind, LibraryID: "navidrome:" + valueText(row.Fields["user_id"]), Fields: fields}
}

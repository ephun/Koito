package sources

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

func nativeKoitoTracks(ctx context.Context, db *sql.DB) (map[string]map[string]any, error) {
	rows, err := db.QueryContext(ctx, `SELECT t.id,
 COALESCE((SELECT alias FROM track_aliases WHERE track_id=t.id ORDER BY is_primary DESC,alias LIMIT 1),''),
 COALESCE((SELECT alias FROM release_aliases WHERE release_id=t.release_id ORDER BY is_primary DESC,alias LIMIT 1),''),
 COALESCE((SELECT aa.alias FROM artist_tracks a JOIN artist_aliases aa ON aa.artist_id=a.artist_id WHERE a.track_id=t.id ORDER BY a.is_primary DESC,aa.is_primary DESC,aa.alias LIMIT 1),'') FROM tracks t ORDER BY t.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := map[string]map[string]any{}
	for rows.Next() {
		var id, title, album, artist string
		if err = rows.Scan(&id, &title, &album, &artist); err != nil {
			return nil, err
		}
		values[id] = map[string]any{"title": title, "album": album, "artist": artist}
	}
	return values, rows.Err()
}

func normalizeNativeKoito(row databaseRecord) recordValues {
	track := object(row.Context["track"])
	v := recordValues{Title: valueText(track["title"]), Artist: valueText(track["artist"]), Album: valueText(track["album"]), Reason: "counted", Origin: "converted", TimeMeaning: "unknown", Precision: int64(time.Second), Forwarder: valueText(row.Fields["client"]), Endpoint: "koito"}
	seconds := numeric(row.Fields["listened_at"])
	v.TimeText = valueText(row.Fields["listened_at"])
	if seconds > 0 && seconds <= 253402300799 {
		v.Time = time.Unix(seconds, 0).UTC()
	} else {
		v.Reason = "invalid timestamp"
	}
	v.EventID = fmt.Sprintf("koito:%s:%s:%s", valueText(row.Fields["user_id"]), valueText(row.Fields["track_id"]), v.TimeText)
	client := strings.ToLower(v.Forwarder)
	if strings.Contains(client, "itunes") || strings.Contains(client, "setlist") || strings.HasPrefix(client, "sources:") {
		v.Origin = "derived"
		v.Reason = "legacy derived history"
	}
	if v.Reason == "counted" && (v.Title == "" || v.Artist == "") {
		v.Reason = "not an identified music track"
	}
	return v
}

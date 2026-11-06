package sources

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"time"
)

// Games, scores, and profiles are different records. Embedded references retain
// the complete related rows while allowing one game to represent one event.
type yargLookup struct {
	players map[string]map[string]any
	games   map[string]map[string]any
	scores  map[string][]any
}

func loadYARGContext(ctx context.Context, db *sql.DB) (*yargLookup, error) {
	out := &yargLookup{map[string]map[string]any{}, map[string]map[string]any{}, map[string][]any{}}
	for _, table := range []string{"Players", "GameRecords", "PlayerScores"} {
		err := readSQLTable(ctx, db, table, func(fields map[string]any, key string) error {
			id := valueText(fields["Id"])
			switch table {
			case "Players":
				out.players[id] = fields
			case "GameRecords":
				out.games[id] = fields
			case "PlayerScores":
				game := valueText(fields["GameRecordId"])
				score := map[string]any{"fields": fields, "player": out.players[valueText(fields["PlayerId"])]}
				if score["player"] == nil {
					score["issue"] = "missing player"
				}
				out.scores[game] = append(out.scores[game], score)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (lookup *yargLookup) attach(row *databaseRecord) {
	id := valueText(row.Fields["Id"])
	switch row.Table {
	case "GameRecords":
		row.Kind = "game"
		row.Context["scores"] = lookup.scores[id]
	case "Players":
		row.Kind = "game_player"
	case "PlayerScores":
		row.Kind = "game_score"
		row.Context["player"] = lookup.players[valueText(row.Fields["PlayerId"])]
		row.Context["game"] = lookup.games[valueText(row.Fields["GameRecordId"])]
		if row.Context["player"] == nil || row.Context["game"] == nil {
			row.Context["issue"] = "missing game or player"
		}
	}
}

func sqliteBlobHex(value any) string {
	encoded := object(value)["$sqlite_blob"]
	var data []byte
	switch v := encoded.(type) {
	case []byte:
		data = v
	case string:
		data, _ = base64.StdEncoding.DecodeString(v)
	}
	return hex.EncodeToString(data)
}

// DateTime ticks contain a wall-clock value, not its timezone. Keep every tick
// exact before an explicit Time filter interprets the local result time.
func yargWallTime(value any) (string, bool) {
	ticks, err := strconv.ParseInt(valueText(value), 10, 64)
	if err != nil || ticks <= 0 || ticks > 3155378975999999999 {
		return "", false
	}
	seconds, remainder := ticks/10000000, ticks%10000000
	wall := time.Unix(seconds-62135596800, remainder*100).UTC()
	return wall.Format("2006-01-02T15:04:05.0000000"), true
}

func sqliteBool(value any) *bool {
	if value == nil {
		return nil
	}
	text := valueText(value)
	if text != "0" && text != "1" && text != "true" && text != "false" {
		return nil
	}
	result := text == "1" || text == "true"
	return &result
}

func normalizeYARG(raw json.RawMessage, zone string) recordValues {
	var row databaseRecord
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&row) != nil {
		return recordValues{Reason: "invalid fields"}
	}
	v := recordValues{Reason: "game context", Origin: "original", OriginalService: "yarg"}
	if row.Kind != "game" {
		return v
	}
	f := row.Fields
	v.Title, v.Artist = valueText(f["SongName"]), valueText(f["SongArtist"])
	v.NativeTrackID = sqliteBlobHex(f["SongChecksum"])
	v.TimeMeaning = "result"
	v.EventID = fmt.Sprintf("yarg:%s:%s:%s", valueText(f["Date"]), v.NativeTrackID, sqliteBlobHex(f["ReplayChecksum"]))
	performance := &performanceValues{Kind: "gameplay", ID: v.EventID, ChartID: v.NativeTrackID, PlayerIDs: []string{}, PlayerNames: []string{}, Scores: []any{}, WithReplay: sqliteBool(f["PlayedWithReplay"]), HasBots: sqliteBool(f["HasBots"])}
	if speed, err := strconv.ParseFloat(valueText(f["SongSpeed"]), 64); err == nil && !math.IsNaN(speed) && !math.IsInf(speed, 0) {
		performance.Speed = &speed
	}
	seen := map[string]bool{}
	scores, _ := row.Context["scores"].([]any)
	for _, value := range scores {
		score := object(value)
		fields, player := object(score["fields"]), object(score["player"])
		id := valueText(fields["PlayerId"])
		performance.Scores = append(performance.Scores, value)
		if id != "" && !seen[id] {
			seen[id] = true
			performance.PlayerIDs = append(performance.PlayerIDs, id)
			if name := valueText(player["Name"]); name != "" {
				performance.PlayerNames = append(performance.PlayerNames, name)
			}
		}
	}
	v.Performance = performance
	var valid bool
	if v.TimeText, valid = yargWallTime(f["Date"]); !valid {
		v.Reason = "invalid timestamp"
		return v
	}
	v.Time, _, v.Reason = interpretTime(v.TimeText, zone)
	v.Precision = 100
	if v.Reason == "counted" {
		switch {
		case v.Title == "" || v.Artist == "":
			v.Reason = "not an identified music track"
		case performance.Speed == nil || *performance.Speed <= 0:
			v.Reason = "invalid game speed"
		case len(scores) == 0 || performance.WithReplay != nil && *performance.WithReplay:
			v.Reason = "not reported complete"
		}
	}
	return v
}

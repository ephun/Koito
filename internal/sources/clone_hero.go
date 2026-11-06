package sources

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
)

const CloneScores = "clone-hero-scores-v1"
const CloneCache = "clone-hero-cache-v1"

func gameBinaryParser(parser string) bool {
	return parser == CloneScores || parser == CloneCache || parser == YARGCache
}

func parseCloneHero(file File, data []byte) (parsedFile, error) {
	part := parsedFile{file: file, service: "clonehero"}
	if len(data) < 4 {
		return part, ErrUnsupportedFile
	}
	version := binary.LittleEndian.Uint32(data)
	r := &gameBinary{data: data}
	r.u32()
	emit := func(kind, table, key string, fields map[string]any) {
		raw, _ := json.Marshal(databaseRecord{Kind: kind, Table: table, Key: key, Fields: fields})
		part.records = append(part.records, raw)
	}
	switch version {
	case 20180220, 20211009:
		part.parser = CloneScores
		count := r.count()
		emit("game_file", "header", "header", map[string]any{"version": version, "songs": count})
		for song := 0; song < count && r.err == nil; song++ {
			start := r.pos
			checksum := ""
			if version == 20180220 {
				checksum = r.text()
				decoded, err := hex.DecodeString(checksum)
				if err != nil || len(decoded) != 16 {
					return part, errors.New("Invalid Clone Hero chart checksum.")
				}
				checksum = hex.EncodeToString(decoded)
			} else {
				checksum = hex.EncodeToString(r.take(16))
			}
			scores := int(r.u8())
			counter := r.take(3)
			emit("game_summary", "songs", fmt.Sprint(song), map[string]any{"chart_checksum": checksum, "score_count": scores, "counter_bytes": counter, "counter_meaning": "unverified", "version": version, "bytes": data[start:r.pos]})
			for score := 0; score < scores && r.err == nil; score++ {
				start = r.pos
				instrument, difficulty := r.u16(), r.u8()
				numerator, denominator, stars := r.u16(), r.u16(), r.u8()
				var flags uint32
				if version == 20180220 {
					flags = uint32(r.u8())
				} else {
					flags = r.u32()
				}
				points := r.u32()
				emit("game_score", "scores", fmt.Sprintf("%d:%d", song, score), map[string]any{"chart_checksum": checksum, "instrument_code": instrument, "difficulty_code": difficulty, "accuracy_numerator": numerator, "accuracy_denominator": denominator, "stars": stars, "flags": flags, "score": points, "version": version, "bytes": data[start:r.pos]})
			}
		}
	case 20221120:
		part.parser = CloneCache
		hash := hex.EncodeToString(r.take(16))
		categories := []string{"title", "artist", "album", "genre", "year", "charter", "playlist"}
		strings := make([][]string, len(categories))
		for category := range categories {
			if int(r.u8()) != category {
				return part, errors.New("Unsupported Clone Hero metadata order.")
			}
			count := r.count()
			for i := 0; i < count && r.err == nil; i++ {
				strings[category] = append(strings[category], r.text())
			}
		}
		count := r.count()
		emit("game_file", "header", "header", map[string]any{"version": version, "hash": hash, "strings": strings, "songs": count})
		for song := 0; song < count && r.err == nil; song++ {
			start := r.pos
			fields := map[string]any{"path": r.text(), "ini_modified_binary": r.u64(), "chart_modified_binary": r.u64(), "chart_file": r.text(), "encrypted": r.u8()}
			for category, name := range categories {
				index := r.u32()
				if uint64(index) >= uint64(len(strings[category])) {
					return part, errors.New("Invalid Clone Hero metadata reference.")
				}
				fields[name] = strings[category][index]
			}
			fields["available_charts"] = r.u64()
			fields["lyrics"] = r.u8()
			fields["intensities"] = r.take(12)
			fields["preview_start"] = r.u32()
			fields["icon"] = r.text()
			fields["album_track"], fields["playlist_track"] = r.u16(), r.u16()
			fields["modchart"], fields["video"], fields["pro_drums"], fields["five_lane"] = r.u8(), r.u8(), r.u8(), r.u8()
			fields["duration_ms"], fields["date_added_binary"] = r.u32(), r.u64()
			fields["top_playlist"], fields["sub_playlist"] = r.text(), r.text()
			fields["chart_checksum"] = hex.EncodeToString(r.take(16))
			fields["version"], fields["bytes"] = version, data[start:r.pos]
			emit("catalog_track", "songs", fmt.Sprint(song), fields)
		}
	default:
		return part, ErrUnsupportedFile
	}
	return part, r.finish()
}

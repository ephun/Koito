package sources

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

const YARGCache = "yarg-cache-v1"

func (r *gameBinary) slice() *gameBinary {
	size := r.u32()
	data := r.take(int(size))
	return &gameBinary{data: data, err: r.err}
}

func parseYARGCache(file File, data []byte) (parsedFile, error) {
	part := parsedFile{file: file, service: "yarg", parser: YARGCache}
	if len(data) < 4 {
		return part, ErrUnsupportedFile
	}
	version := binary.LittleEndian.Uint32(data)
	if version != 25031401 && version != 26042800 {
		return part, ErrUnsupportedFile
	}
	r := &gameBinary{data: data}
	r.u32()
	fullDirectory := r.u8()
	categories := []string{"title", "artist", "album", "genre"}
	if version == 26042800 {
		categories = append(categories, "subgenre")
	}
	categories = append(categories, "year", "charter", "playlist", "source")
	strings := make([][]string, len(categories))
	for category := range categories {
		block := r.slice()
		count := block.count()
		for i := 0; i < count && block.err == nil; i++ {
			strings[category] = append(strings[category], block.text())
		}
		if err := block.finish(); err != nil {
			return part, fmt.Errorf("Read YARG metadata: %w", err)
		}
	}
	emit := func(kind, table, key string, fields map[string]any) {
		raw, _ := json.Marshal(databaseRecord{Kind: kind, Table: table, Key: key, Fields: fields})
		part.records = append(part.records, raw)
	}
	emit("game_file", "header", "header", map[string]any{"version": version, "full_directory_playlists": fullDirectory, "categories": categories, "strings": strings})
	for section := 0; section < 5 && r.err == nil; section++ {
		count := r.count()
		for group := 0; group < count && r.err == nil; group++ {
			block := r.slice()
			if section != 3 {
				// These framed groups retain complete bytes independently of the
				// INI/SNG catalog. They establish no dated gameplay.
				emit("game_cache_context", "cache_groups", fmt.Sprintf("%d:%d", section, group), map[string]any{"section": section, "version": version, "bytes": block.data})
				continue
			}
			root := block.text()
			for kind := 0; kind < 2 && block.err == nil; kind++ {
				entries := block.count()
				for entry := 0; entry < entries && block.err == nil; entry++ {
					node := block.slice()
					fields := map[string]any{"root": root, "path": node.text(), "version": version}
					if kind == 0 {
						fields["chart_format"] = node.u8()
						fields["chart_modified_binary"] = node.u64()
						if node.u8() != 0 {
							fields["ini_modified_binary"] = node.u64()
						}
					} else {
						fields["modified_binary"], fields["sng_version"], fields["chart_format"] = node.u64(), node.u32(), node.u8()
					}
					fields["chart_checksum"] = hex.EncodeToString(node.take(20))
					fields["available_parts"] = node.take(42)
					for category, name := range categories {
						index := node.u32()
						if node.err != nil || uint64(index) >= uint64(len(strings[category])) {
							return part, fmt.Errorf("Invalid YARG %s reference.", name)
						}
						fields[name] = strings[category][index]
					}
					fields["master"], fields["video_loop"] = node.u8(), node.u8()
					fields["album_track"], fields["playlist_track"] = node.u32(), node.u32()
					fields["duration_ms"], fields["offset_ms"], fields["rating"] = node.u64(), node.u64(), node.u32()
					fields["preview_start"], fields["preview_end"], fields["video_start"], fields["video_end"] = node.u64(), node.u64(), node.u64(), node.u64()
					texts := []string{"loading_phrase", "link_bandcamp", "link_bluesky", "link_facebook", "link_instagram", "link_spotify", "link_twitter", "link_other", "link_youtube", "location", "credit_album_art", "credit_arranged", "credit_composed", "credit_courtesy", "credit_engineered", "credit_license", "credit_mastered", "credit_mixed", "credit_other", "credit_performed", "credit_produced", "credit_published", "credit_written", "charter_bass", "charter_drums", "charter_elite_drums", "charter_guitar", "charter_keys", "charter_lower_difficulties", "charter_pro_bass", "charter_pro_keys", "charter_pro_guitar", "charter_vocals"}
					if version == 26042800 {
						texts = []string{"covered_by", "loading_phrase", "secondary_year", "link_bandcamp", "link_bluesky", "link_facebook", "link_instagram", "link_newgrounds", "link_soundcloud", "link_spotify", "link_tiktok", "link_twitter", "link_other", "link_youtube", "location", "credit_album_art", "credit_arranged", "credit_composed", "credit_courtesy", "credit_engineered", "credit_license", "credit_mastered", "credit_mixed", "credit_other", "credit_performed", "credit_produced", "credit_published", "credit_written", "charter_audio", "charter_bass", "charter_drums", "charter_elite_drums", "charter_guitar", "charter_keys", "charter_lower_difficulties", "charter_pro_bass", "charter_pro_keys", "charter_pro_guitar", "charter_venue", "charter_vocals"}
					}
					for _, name := range texts {
						fields[name] = node.text()
					}
					fields["hopo_threshold"], fields["sustain_cutoff"], fields["overdrive_note"] = node.u64(), node.u64(), node.u32()
					fields["background"], fields["video"], fields["cover"] = node.text(), node.text(), node.text()
					fields["bytes"] = node.data
					if err := node.finish(); err != nil {
						return part, fmt.Errorf("Read YARG song: %w", err)
					}
					emit("catalog_track", "songs", fmt.Sprintf("%d:%d:%d", group, kind, entry), fields)
				}
			}
			if err := block.finish(); err != nil {
				return part, fmt.Errorf("Read YARG directory: %w", err)
			}
		}
	}
	return part, r.finish()
}

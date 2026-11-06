package sources

import (
	"context"
	"strings"

	"github.com/gabehf/koito/internal/filters"
	"github.com/gabehf/koito/internal/music"
)

// A chart checksum links catalog metadata to its game without replacing the
// original game fields. Conflicting catalog values remain separate candidates.
func attachGameCatalog(records []matchingRecord) {
	index := map[string][]int{}
	for i, r := range records {
		if (r.Parser == YARGCache || r.Parser == CloneCache) && r.Values.NativeTrackID != "" {
			index[r.Service+":"+r.Values.NativeTrackID] = append(index[r.Service+":"+r.Values.NativeTrackID], i)
		}
	}
	for i := range records {
		r := &records[i]
		if r.Values.Performance == nil || r.Values.Performance.Kind != "gameplay" {
			continue
		}
		candidates := index[r.Service+":"+r.Values.NativeTrackID]
		if len(candidates) == 0 {
			continue
		}
		ids, items := []filters.Value{}, []filters.Value{}
		r.GameCatalog = &music.GameCatalog{Records: []int64{}, Items: []any{}}
		albums, durations := map[string]bool{}, map[int64]bool{}
		for _, position := range candidates {
			catalog := records[position]
			ids = append(ids, filters.Integer(catalog.ID))
			plain := map[string]any{"record_id": catalog.ID, "file_hash": catalog.FileHash, "title": catalog.Values.Title, "artist": catalog.Values.Artist, "album": catalog.Values.Album, "duration_ms": catalog.Values.DurationMS}
			r.GameCatalog.Records = append(r.GameCatalog.Records, catalog.ID)
			r.GameCatalog.Items = append(r.GameCatalog.Items, plain)
			item, _ := filters.From(plain)
			items = append(items, item)
			if catalog.Values.Album != "" {
				albums[catalog.Values.Album] = true
			}
			if catalog.Values.DurationMS > 0 {
				durations[catalog.Values.DurationMS] = true
			}
		}
		if r.LibraryFields == nil {
			r.LibraryFields = filters.Env{}
		}
		r.LibraryFields["game.catalog.records"], r.LibraryFields["game.catalog.items"] = filters.Values(ids...), filters.Values(items...)
		if len(albums) == 1 {
			for album := range albums {
				r.LibraryFields["game.catalog.album"] = filters.String(album)
				r.GameCatalog.Album = album
			}
		}
		if len(durations) == 1 {
			for duration := range durations {
				r.LibraryFields["game.catalog.duration_ms"] = filters.Integer(duration)
				value := duration
				r.GameCatalog.DurationMS = &value
			}
		}
	}
}

func inspectionGameCatalog(ctx context.Context, reader sqlReader, records []matchingRecord) error {
	wanted := map[int64]bool{}
	for _, r := range records {
		if r.Values.Performance != nil && r.Values.Performance.Kind == "gameplay" && r.Values.NativeTrackID != "" {
			wanted[r.SourceID] = true
		}
	}
	if len(wanted) == 0 {
		return nil
	}
	args := []any{}
	for source := range wanted {
		args = append(args, source)
	}
	query := `SELECT id,source_id,file_hash,parser,payload FROM source_records WHERE source_id IN (` + strings.TrimSuffix(strings.Repeat("?,", len(args)), ",") + `) AND parser IN (?,?)`
	args = append(args, YARGCache, CloneCache)
	rows, err := reader.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	all := append([]matchingRecord{}, records...)
	included := map[int64]bool{}
	for _, record := range records {
		included[record.ID] = true
	}
	for rows.Next() {
		var r matchingRecord
		if err = rows.Scan(&r.ID, &r.SourceID, &r.FileHash, &r.Parser, &r.Payload); err != nil {
			rows.Close()
			return err
		}
		if included[r.ID] {
			continue
		}
		r.Service = "yarg"
		if r.Parser == CloneCache {
			r.Service = "clonehero"
		}
		r.Values = normalizeRecord([]byte(r.Payload), r.Parser, "")
		all = append(all, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	attachGameCatalog(all)
	copy(records, all[:len(records)])
	return nil
}

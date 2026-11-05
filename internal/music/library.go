package music

import (
	"context"
	"github.com/gabehf/koito/internal/filters"
)

func LibraryFields(s filters.Schema) filters.Schema {
	for key, kind := range map[string]filters.Type{"music.saved": filters.Boolean, "music.owned": filters.Boolean, "music.wanted": filters.Boolean, "music.playlists": filters.List, "music.membership_records": filters.List} {
		s.Fields[key] = kind
	}
	if s.Labels == nil {
		s.Labels = map[string]string{}
	}
	for key, label := range map[string]string{"music.saved": "Saved in imported library", "music.owned": "Owned in imported collection", "music.wanted": "On imported wantlist", "music.playlists": "Imported playlists", "music.membership_records": "Membership record identifiers"} {
		s.Labels[key] = label
	}
	return s
}

func EmptyLibraryEnvironment() filters.Env {
	return filters.Env{"music.saved": filters.Bool(false), "music.owned": filters.Bool(false), "music.wanted": filters.Bool(false), "music.playlists": filters.Values(), "music.membership_records": filters.Values()}
}

// Load once for catalog filtering. Membership links span the shared track
// history while album ownership remains attached to its release.
func CatalogLibraryEnvironment(ctx context.Context, db Database) (map[int64]filters.Env, error) {
	rows, err := db.QueryContext(ctx, `SELECT t.id,m.kind,m.name,m.record_id FROM source_music_states m JOIN tracks t ON t.id=m.track_id
 UNION SELECT t.track_id,m.kind,m.name,m.record_id FROM source_music_states m JOIN track_histories h ON h.track_id=m.track_id JOIN track_histories t ON t.music_track_id=h.music_track_id
 UNION SELECT t.id,m.kind,m.name,m.record_id FROM source_music_states m JOIN tracks t ON t.release_id=m.release_id WHERE m.track_id IS NULL
 ORDER BY 1,4`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]filters.Env{}
	lists := map[int64][]filters.Value{}
	records := map[int64][]filters.Value{}
	for rows.Next() {
		var track, id int64
		var kind, name string
		if err = rows.Scan(&track, &kind, &name, &id); err != nil {
			return nil, err
		}
		if out[track] == nil {
			out[track] = EmptyLibraryEnvironment()
		}
		env := out[track]
		switch kind {
		case "saved_tracks", "saved_albums":
			env["music.saved"] = filters.Bool(true)
		case "owned":
			env["music.owned"] = filters.Bool(true)
		case "wanted":
			env["music.wanted"] = filters.Bool(true)
		case "playlist":
			lists[track] = append(lists[track], filters.String(name))
		}
		records[track] = append(records[track], filters.Integer(id))
	}
	for id, env := range out {
		env["music.playlists"] = filters.Values(lists[id]...)
		env["music.membership_records"] = filters.Values(records[id]...)
	}
	return out, rows.Err()
}

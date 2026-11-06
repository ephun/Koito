package music

import (
	"context"
	"encoding/json"
	"github.com/gabehf/koito/internal/filters"
)

func loadGroupingAlbum(ctx context.Context, db Database, id int64) (groupingRelease, filters.Env, error) {
	r := groupingRelease{id: id}
	var raw string
	err := db.QueryRowContext(ctx, `SELECT r.title,r.various_artists,COALESCE((SELECT group_concat(artist_id,',') FROM(SELECT artist_id FROM artist_releases WHERE release_id=r.id ORDER BY artist_id)),''),COALESCE((SELECT json_group_array(name) FROM(SELECT a.name FROM artist_releases c JOIN artists_with_name a ON a.id=c.artist_id WHERE c.release_id=r.id ORDER BY c.artist_id)),'[]'),COALESCE((SELECT group_concat(artist_id,',') FROM(SELECT DISTINCT artist_id FROM displayed_artist_releases WHERE release_id=r.id ORDER BY artist_id)),'') FROM releases_with_title r WHERE r.id=?`, id).Scan(&r.title, &r.various, &r.credits, &raw, &r.reportCredits)
	if err != nil {
		return r, nil, err
	}
	var names []string
	if err = json.Unmarshal([]byte(raw), &names); err != nil {
		return r, nil, err
	}
	r.artist = raw
	if len(names) == 1 {
		r.artist = names[0]
	}
	env, err := NativeAlbumGroupingEnvironment(id, r.title, r.artist, r.credits, r.various)
	if err != nil {
		return r, nil, err
	}
	displays, err := AlbumDisplays(ctx, db, id)
	if err != nil {
		return r, nil, err
	}
	r.displayTitle, _ = displays[id]["display.album"].Text()
	return r, WithAlbumDisplay(env, displays[id]), nil
}

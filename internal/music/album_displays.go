package music

import (
	"context"
	"github.com/gabehf/koito/internal/filters"
)

// AlbumDisplays supplies current catalog presentation without changing original fields.
func AlbumDisplays(ctx context.Context, db Database, target int64) (map[int64]filters.Env, error) {
	query := `SELECT r.id,COALESCE(n.display_name,r.title) FROM releases_with_title r LEFT JOIN catalog_names n ON n.kind='album' AND n.item_id=r.id`
	args := []any{}
	if target != 0 {
		query += ` WHERE r.id=?`
		args = append(args, target)
	}
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	result := map[int64]filters.Env{}
	for rows.Next() {
		var id int64
		var title string
		if err = rows.Scan(&id, &title); err != nil {
			rows.Close()
			return nil, err
		}
		result[id] = filters.Env{"display.album": filters.String(title), "display.artist": filters.Absent("display.artist")}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	query = `SELECT a.release_id,COALESCE(n.display_name,r.name) FROM displayed_artist_releases a JOIN artists_with_name r ON r.id=a.artist_id LEFT JOIN catalog_names n ON n.kind='artist' AND n.item_id=a.artist_id`
	if target != 0 {
		query += ` WHERE a.release_id=?`
	}
	rows, err = db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	counts := map[int64]int{}
	for rows.Next() {
		var id int64
		var name string
		if err = rows.Scan(&id, &name); err != nil {
			rows.Close()
			return nil, err
		}
		counts[id]++
		if env := result[id]; env != nil {
			if counts[id] == 1 {
				env["display.artist"] = filters.String(name)
			} else {
				env["display.artist"] = filters.Absent("display.artist")
			}
		}
	}
	err = rows.Err()
	rows.Close()
	return result, err
}

func WithAlbumDisplay(env filters.Env, display filters.Env) filters.Env {
	for field, value := range display {
		env[field] = value
	}
	return env
}

func TrackDisplays(ctx context.Context, db Database, albumID int64) (map[int64]filters.Env, error) {
	query := `SELECT t.id,COALESCE(n.display_name,t.title),COALESCE(a.display_name,r.title) FROM tracks_with_title t JOIN releases_with_title r ON r.id=t.release_id LEFT JOIN catalog_names n ON n.kind='track' AND n.item_id=t.id LEFT JOIN catalog_names a ON a.kind='album' AND a.item_id=r.id`
	args := []any{}
	if albumID != 0 {
		query += ` WHERE t.release_id=?`
		args = append(args, albumID)
	}
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	result := map[int64]filters.Env{}
	for rows.Next() {
		var id int64
		var title, album string
		if err = rows.Scan(&id, &title, &album); err != nil {
			rows.Close()
			return nil, err
		}
		result[id] = filters.Env{"display.title": filters.String(title), "display.album": filters.String(album), "display.artist": filters.Absent("display.artist")}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	query = `SELECT a.track_id,COALESCE(n.display_name,r.name) FROM displayed_artist_tracks a JOIN tracks t ON t.id=a.track_id JOIN artists_with_name r ON r.id=a.artist_id LEFT JOIN catalog_names n ON n.kind='artist' AND n.item_id=a.artist_id`
	if albumID != 0 {
		query += ` WHERE t.release_id=?`
	}
	rows, err = db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	counts := map[int64]int{}
	for rows.Next() {
		var id int64
		var name string
		if err = rows.Scan(&id, &name); err != nil {
			rows.Close()
			return nil, err
		}
		counts[id]++
		if env := result[id]; env != nil {
			if counts[id] == 1 {
				env["display.artist"] = filters.String(name)
			} else {
				env["display.artist"] = filters.Absent("display.artist")
			}
		}
	}
	err = rows.Err()
	rows.Close()
	return result, err
}

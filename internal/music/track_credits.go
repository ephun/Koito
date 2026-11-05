package music

import (
	"context"
	"sort"
)

type trackCredit struct {
	Original []int64
	Matched  []int64
	Names    []string
}

// Load names once instead of correlating the primary-name view from every
// track. Original credit order and distinct matched credits remain separate.
func loadTrackCredits(ctx context.Context, db Database, trackID int64) (map[int64]trackCredit, error) {
	query := `SELECT id,name FROM artists_with_name`
	args := []any{}
	if trackID != 0 {
		query += ` WHERE id IN(SELECT artist_id FROM artist_tracks WHERE track_id=?)`
		args = append(args, trackID)
	}
	query += ` ORDER BY id`
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	names := map[int64][]string{}
	for rows.Next() {
		var id int64
		var name string
		if err = rows.Scan(&id, &name); err != nil {
			rows.Close()
			return nil, err
		}
		names[id] = append(names[id], name)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	query = `SELECT a.track_id,a.artist_id,COALESCE(mt.artist_id,ma.matched_artist_id,a.artist_id) FROM artist_tracks a LEFT JOIN music_track_artists mt ON mt.track_id=a.track_id AND mt.original_artist_id=a.artist_id LEFT JOIN music_artist_matches ma ON ma.artist_id=a.artist_id`
	args = nil
	if trackID != 0 {
		query += ` WHERE a.track_id=?`
		args = append(args, trackID)
	}
	query += ` ORDER BY a.track_id,a.artist_id`
	rows, err = db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	credits := map[int64]trackCredit{}
	matched := map[int64]map[int64]bool{}
	for rows.Next() {
		var track, artist, target int64
		if err = rows.Scan(&track, &artist, &target); err != nil {
			return nil, err
		}
		credit := credits[track]
		credit.Original = append(credit.Original, artist)
		credit.Names = append(credit.Names, names[artist]...)
		credits[track] = credit
		if matched[track] == nil {
			matched[track] = map[int64]bool{}
		}
		matched[track][target] = true
	}
	for track, credit := range credits {
		for id := range matched[track] {
			credit.Matched = append(credit.Matched, id)
		}
		sort.Slice(credit.Matched, func(i, j int) bool { return credit.Matched[i] < credit.Matched[j] })
		credits[track] = credit
	}
	return credits, rows.Err()
}

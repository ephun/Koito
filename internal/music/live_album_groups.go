package music

import (
	"context"
	"encoding/json"
)

func albumMemberships(ctx context.Context, db Database, album int64) ([]int64, error) {
	rows, err := db.QueryContext(ctx, `SELECT group_id FROM album_group_members WHERE release_id=?`, album)
	if err != nil {
		return nil, err
	}
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	return ids, err
}

func pruneInactiveAlbumGroups(ctx context.Context, db Database, programs GroupingPrograms, groups []int64) error {
	for _, group := range groups {
		live, err := albumGroupHasAssignment(ctx, db, programs, group)
		if err != nil {
			return err
		}
		if live {
			continue
		}
		if _, err = db.ExecContext(ctx, `DELETE FROM native_release_groups WHERE group_id=?`, group); err != nil {
			return err
		}
		if _, err = db.ExecContext(ctx, `DELETE FROM album_group_members WHERE group_id=?`, group); err != nil {
			return err
		}
		if err = refreshAlbumGroupCanonical(ctx, db, group); err != nil {
			return err
		}
	}
	return nil
}

func albumGroupHasAssignment(ctx context.Context, db Database, programs GroupingPrograms, group int64) (bool, error) {
	var source bool
	if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM music_release_groups WHERE group_id=?)`, group).Scan(&source); err != nil {
		return false, err
	}
	if source {
		return true, nil
	}
	if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM source_play_groupings s JOIN source_plays p ON p.play_key=s.play_key WHERE s.group_id=?) OR EXISTS(SELECT 1 FROM source_aggregate_groupings s JOIN source_aggregates a ON a.aggregate_key=s.aggregate_key WHERE s.group_id=?)`, group, group).Scan(&source); err != nil {
		return false, err
	}
	if source {
		return true, nil
	}
	var rawKey, label string
	if err := db.QueryRowContext(ctx, `SELECT group_key,title FROM album_groups WHERE id=?`, group).Scan(&rawKey, &label); err != nil {
		return false, err
	}
	var key []string
	if err := json.Unmarshal([]byte(rawKey), &key); err != nil {
		return false, err
	}
	rows, err := db.QueryContext(ctx, `SELECT release_id FROM native_release_groups WHERE group_id=?`, group)
	if err != nil {
		return false, err
	}
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return false, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return false, err
	}
	for _, id := range ids {
		r, env, e := loadGroupingAlbum(ctx, db, id)
		if e != nil {
			return false, e
		}
		if len(key) != 2 || r.reportCredits != key[0] {
			continue
		}
		value, assigned, _, e := programs.Resolve(ctx, env)
		if e != nil {
			return false, e
		}
		if assigned && value == label {
			return true, nil
		}
	}
	return false, nil
}

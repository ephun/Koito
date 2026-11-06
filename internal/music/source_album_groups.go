package music

import (
	"context"
	"github.com/gabehf/koito/internal/filters"
)

// AssignSourceAlbumGroups refreshes explicit source decisions after catalog presentation changes.
func AssignSourceAlbumGroups(ctx context.Context, db Database, programs GroupingPrograms, album int64, inputs []AlbumGroupingInput) error {
	var credits string
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(group_concat(artist_id,','),'') FROM(SELECT artist_id FROM displayed_artist_releases WHERE release_id=? ORDER BY artist_id)`, album).Scan(&credits); err != nil {
		return err
	}
	for _, input := range inputs {
		env := input.Env()
		env["catalog.album_id"] = filters.Integer(album)
		label, assigned, _, err := programs.Resolve(ctx, env)
		if err != nil {
			return err
		}
		table, column, key := "source_play_groupings", "play_key", input.PlayKey
		if input.AggregateKey != "" {
			table, column, key = "source_aggregate_groupings", "aggregate_key", input.AggregateKey
		}
		if !assigned {
			if _, err = db.ExecContext(ctx, `DELETE FROM `+table+` WHERE `+column+`=?`, key); err != nil {
				return err
			}
			continue
		}
		var id int64
		if label != "" {
			reportCredits := credits
			if input.Credits != "" {
				reportCredits = input.Credits
			}
			groupKey := albumGroupKey(reportCredits, label)
			if _, err = db.ExecContext(ctx, `INSERT INTO album_groups(group_key,title,canonical_release_id) VALUES(?,?,?) ON CONFLICT(group_key) DO NOTHING`, groupKey, label, album); err != nil {
				return err
			}
			if err = db.QueryRowContext(ctx, `SELECT id FROM album_groups WHERE group_key=?`, groupKey).Scan(&id); err != nil {
				return err
			}
			if _, err = db.ExecContext(ctx, `INSERT INTO album_group_members(group_id,release_id) VALUES(?,?) ON CONFLICT DO NOTHING`, id, album); err != nil {
				return err
			}
		}
		if _, err = db.ExecContext(ctx, `INSERT INTO `+table+`(`+column+`,group_id) VALUES(?,?) ON CONFLICT(`+column+`) DO UPDATE SET group_id=excluded.group_id`, key, id); err != nil {
			return err
		}
	}
	return nil
}

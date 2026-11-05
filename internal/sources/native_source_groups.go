package sources

import (
	"context"
	"database/sql"

	"github.com/gabehf/koito/internal/filters"
	"github.com/gabehf/koito/internal/music"
)

func refreshNativeTrackGroups(ctx context.Context, tx *sql.Tx, config RuleConfig, track int64) error {
	active := false
	for _, r := range config.Rules {
		if r.Enabled && r.Type == "presentation" && r.Subtype == "grouping" {
			active = true
		}
	}
	if !active {
		return nil
	}
	var album int64
	if err := tx.QueryRowContext(ctx, `SELECT release_id FROM tracks WHERE id=?`, track).Scan(&album); err != nil {
		return err
	}
	if err := refreshNativeSourceGroups(ctx, tx, config, album); err != nil {
		return err
	}
	return music.AssignAlbumGroup(ctx, tx, album)
}

func refreshNativeSourceGroups(ctx context.Context, tx *sql.Tx, config RuleConfig, album int64) error {
	programs, err := music.CompileGroupingPrograms(groupingFilters(config))
	if err != nil || !programs.Active() {
		return err
	}
	records, err := inspectedRecords(ctx, tx, config, ` WHERE r.id IN(SELECT p.representative_record FROM source_plays p JOIN tracks t ON t.id=p.track_id WHERE t.release_id=? UNION SELECT a.representative_record FROM source_aggregates a JOIN tracks t ON t.id=a.track_id WHERE t.release_id=?)`, album, album)
	if err != nil {
		return err
	}
	if len(records) == 0 {
		return nil
	}
	byID := map[int64]matchingRecord{}
	for _, r := range records {
		byID[r.ID] = r.matchingRecord
	}
	naming, err := compileNaming(config)
	if err != nil {
		return err
	}
	displays, err := music.TrackDisplays(ctx, tx, album)
	if err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `SELECT p.play_key,'',p.representative_record,p.track_id FROM source_plays p JOIN tracks t ON t.id=p.track_id WHERE t.release_id=? UNION ALL SELECT '',a.aggregate_key,a.representative_record,a.track_id FROM source_aggregates a JOIN tracks t ON t.id=a.track_id WHERE t.release_id=?`, album, album)
	if err != nil {
		return err
	}
	inputs := []music.AlbumGroupingInput{}
	for rows.Next() {
		var input music.AlbumGroupingInput
		var record, track int64
		if err = rows.Scan(&input.PlayKey, &input.AggregateKey, &record, &track); err != nil {
			rows.Close()
			return err
		}
		r := byID[record]
		v, e := applyNaming(ctx, naming, r, spotifyItem{Artist: r.Values.Artist, Album: r.Values.Album, Title: r.Values.Title})
		if e != nil {
			rows.Close()
			return e
		}
		env := music.WithAlbumDisplay(groupingEnvironment(r, v), displays[track])
		input.ReleaseID = album
		input.Env = func() filters.Env { return env }
		inputs = append(inputs, input)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	return music.AssignSourceAlbumGroups(ctx, tx, programs, album, inputs)
}

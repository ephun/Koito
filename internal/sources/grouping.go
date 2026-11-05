package sources

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/gabehf/koito/internal/filters"
	"github.com/gabehf/koito/internal/music"
)

func editionGroupingProgram() *filters.Program {
	pattern := literalExpr(`(?i)\s*[\[(](?:deluxe|expanded|remastered)[^)\]]*[)\]]\s*$`)
	album := fieldExpr("display.album")
	condition := operation("regex", album, pattern)
	value := operation("trim", operation("regex_replace", album, pattern, literalExpr("")))
	return &filters.Program{Steps: []filters.Statement{{If: &condition, Then: []filters.Statement{{Set: "group.album", Value: &value}}}}}
}

func groupingEnvironment(r matchingRecord, v spotifyItem) filters.Env {
	env := recordEnvironment(r)
	env["display.artist"], env["display.album"], env["display.title"] = filters.String(v.Artist), filters.String(v.Album), filters.String(v.Title)
	return env
}

func groupingFilters(config RuleConfig) []music.IdentityFilter {
	rules := []music.IdentityFilter{}
	for _, r := range config.Rules {
		rules = append(rules, music.IdentityFilter{ID: r.ID, Name: r.Name, Type: r.Type, Subtype: r.Subtype, Enabled: r.Enabled, Program: r.Program})
	}
	return rules
}

func publishAlbumGroups(ctx context.Context, tx *sql.Tx, config RuleConfig, byID map[int64]*matchingRecord, naming []namingProgram) error {
	raw, err := json.Marshal(config)
	if err != nil {
		return err
	}
	active := false
	for _, r := range config.Rules {
		if r.Enabled && r.Type == "presentation" && r.Subtype == "grouping" {
			active = true
		}
	}
	inputs := []music.AlbumGroupingInput{}
	if active {
		displays, e := music.TrackDisplays(ctx, tx, 0)
		if e != nil {
			return e
		}
		rows, e := tx.QueryContext(ctx, `SELECT p.play_key,'',t.release_id,p.representative_record,p.track_id FROM source_plays p JOIN tracks t ON t.id=p.track_id WHERE p.policy='filters-v1'
  UNION ALL SELECT '',a.aggregate_key,t.release_id,a.representative_record,a.track_id FROM source_aggregates a JOIN tracks t ON t.id=a.track_id`)
		if e != nil {
			return e
		}
		displayed := map[int64]spotifyItem{}
		for rows.Next() {
			var input music.AlbumGroupingInput
			var recordID, trackID int64
			if e = rows.Scan(&input.PlayKey, &input.AggregateKey, &input.ReleaseID, &recordID, &trackID); e != nil {
				rows.Close()
				return e
			}
			r := byID[recordID]
			v, ok := displayed[recordID]
			if !ok {
				v, e = applyNaming(ctx, naming, *r, spotifyItem{Artist: r.Values.Artist, Album: r.Values.Album, Title: r.Values.Title})
				if e != nil {
					rows.Close()
					return e
				}
				displayed[recordID] = v
			}
			display := displays[trackID]
			input.Env = func() filters.Env { return music.WithAlbumDisplay(groupingEnvironment(*r, v), display) }
			inputs = append(inputs, input)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
	}

	return music.RefreshAlbumGroups(ctx, tx, string(raw), inputs)
}

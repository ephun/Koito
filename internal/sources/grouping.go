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
		rows, e := tx.QueryContext(ctx, `SELECT p.play_key,'',t.release_id,p.representative_record FROM source_plays p JOIN tracks t ON t.id=p.track_id WHERE p.policy='filters-v1'
  UNION ALL SELECT '',a.aggregate_key,t.release_id,a.representative_record FROM source_aggregates a JOIN tracks t ON t.id=a.track_id`)
		if e != nil {
			return e
		}
		displayed := map[int64]spotifyItem{}
		for rows.Next() {
			var input music.AlbumGroupingInput
			var recordID int64
			if e = rows.Scan(&input.PlayKey, &input.AggregateKey, &input.ReleaseID, &recordID); e != nil {
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
			input.Env = func() filters.Env { return groupingEnvironment(*r, v) }
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

func previewGrouping(ctx context.Context, reader sqlReader, plan matchPlan, before, after RuleConfig) ([]NamingExample, error) {
	old, err := music.CompileGroupingPrograms(groupingFilters(before))
	if err != nil {
		return nil, err
	}
	next, err := music.CompileGroupingPrograms(groupingFilters(after))
	if err != nil {
		return nil, err
	}
	if !old.Active() && !next.Active() {
		return []NamingExample{}, nil
	}
	type release struct {
		id            int64
		title, artist string
	}
	catalog := []release{}
	byName := map[[2]string]int64{}
	rows, err := reader.QueryContext(ctx, `SELECT r.id,r.title,COALESCE((SELECT json_group_array(name) FROM (SELECT a.name FROM artist_releases ar JOIN artists_with_name a ON a.id=ar.artist_id WHERE ar.release_id=r.id ORDER BY ar.artist_id)),'[]') FROM releases_with_title r ORDER BY r.id`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var r release
		var raw string
		if err = rows.Scan(&r.id, &r.title, &raw); err != nil {
			rows.Close()
			return nil, err
		}
		var names []string
		if err = json.Unmarshal([]byte(raw), &names); err != nil {
			rows.Close()
			return nil, err
		}
		r.artist = raw
		if len(names) == 1 {
			r.artist = names[0]
		}
		catalog = append(catalog, r)
		key := [2]string{r.artist, r.title}
		if byName[key] == 0 {
			byName[key] = r.id
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	oldNames, err := compileNaming(before)
	if err != nil {
		return nil, err
	}
	newNames, err := compileNaming(after)
	if err != nil {
		return nil, err
	}
	examples := []NamingExample{}
	shown := map[[2]string]bool{}
	records := map[int64]matchingRecord{}
	for _, r := range plan.Records {
		records[r.ID] = r
	}
	ids := []int64{}
	for _, p := range plan.Plays {
		ids = append(ids, p.Representative)
	}
	for _, a := range plan.Aggregates {
		if a.Attributed > 0 {
			ids = append(ids, a.Representative)
		}
	}
	for _, id := range ids {
		r := records[id]
		v := spotifyItem{Artist: r.Values.Artist, Album: r.Values.Album, Title: r.Values.Title}
		a, e := applyNaming(ctx, oldNames, r, v)
		if e != nil {
			return nil, e
		}
		b, e := applyNaming(ctx, newNames, r, v)
		if e != nil {
			return nil, e
		}
		envA, envB := groupingEnvironment(r, a), groupingEnvironment(r, b)
		if albumID := byName[[2]string{a.Artist, a.Album}]; albumID > 0 {
			envA["catalog.album_id"] = filters.Integer(albumID)
		}
		if albumID := byName[[2]string{b.Artist, b.Album}]; albumID > 0 {
			envB["catalog.album_id"] = filters.Integer(albumID)
		}
		left, assigned, _, e := old.Resolve(ctx, envA)
		if e != nil {
			return nil, e
		}
		if !assigned || left == "" {
			left = a.Album
		}
		right, assigned, _, e := next.Resolve(ctx, envB)
		if e != nil {
			return nil, e
		}
		if !assigned || right == "" {
			right = b.Album
		}
		key := [2]string{left, right}
		if left != right && !shown[key] {
			shown[key] = true
			examples = append(examples, NamingExample{id, "Album group", left, right})
			if len(examples) == 12 {
				break
			}
		}
	}
	if len(examples) < 12 {
		for _, r := range catalog {
			env := filters.Env{"original.artist": filters.String(r.artist), "original.album": filters.String(r.title), "original.title": filters.String(""), "display.artist": filters.String(r.artist), "display.album": filters.String(r.title), "display.title": filters.String(""), "source.service": filters.String("koito"), "catalog.album_id": filters.Integer(r.id)}
			left, assigned, _, e := old.Resolve(ctx, env)
			if e != nil {
				return nil, e
			}
			if !assigned || left == "" {
				left = r.title
			}
			right, assigned, _, e := next.Resolve(ctx, env)
			if e != nil {
				return nil, e
			}
			if !assigned || right == "" {
				right = r.title
			}
			key := [2]string{left, right}
			if left != right && !shown[key] {
				shown[key] = true
				examples = append(examples, NamingExample{Field: "Album group", Before: left, After: right})
				if len(examples) == 12 {
					break
				}
			}
		}
	}
	return examples, nil
}

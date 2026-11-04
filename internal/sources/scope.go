package sources

import (
	"context"
	"database/sql"
	"time"

	"github.com/gabehf/koito/internal/filters"
	"github.com/gabehf/koito/internal/music"
)

func scopeItemsForConfig(ctx context.Context, tx *sql.Tx, config RuleConfig) ([]music.ScopeItem, error) {
	items, err := music.ScopeItems(ctx, tx, 0)
	if err != nil {
		return nil, err
	}
	programs, err := compileNaming(config)
	if err != nil {
		return nil, err
	}
	catalog, err := catalogNameItems(ctx, tx)
	if err != nil {
		return nil, err
	}
	resolved := map[string]map[int64]catalogNamingState{}
	for _, item := range catalog {
		value, _, e := resolveCatalogNaming(ctx, programs, item.kind, item.id, item.name, item.metadata)
		if e != nil {
			return nil, e
		}
		if resolved[item.kind] == nil {
			resolved[item.kind] = map[int64]catalogNamingState{}
		}
		resolved[item.kind][item.id] = value
	}
	for i := range items {
		item := &items[i]
		track := resolved["track"][item.ID]
		albumID, _ := item.Env["catalog.album_id"].Int64()
		item.Title = track.Name
		item.Env["display.title"] = filters.String(track.Name)
		item.Env["display.album"] = filters.String(resolved["album"][albumID].Name)
		item.Env["catalog.artist_ids"] = identifierList(track.Artists)
		artists := []filters.Value{}
		for _, id := range track.Artists {
			artists = append(artists, filters.String(resolved["artist"][id].Name))
		}
		item.Env["display.artists"] = filters.Values(artists...)
		delete(item.Env, "display.artist")
		if len(artists) == 1 {
			item.Env["display.artist"] = artists[0]
		}
	}
	return items, nil
}

func previewScope(ctx context.Context, tx *sql.Tx, before, after RuleConfig) ([]NamingExample, error) {
	examples := []NamingExample{}
	active := false
	for _, config := range []RuleConfig{before, after} {
		for _, rule := range config.Rules {
			if rule.Enabled && rule.Type == "inclusion" && rule.Subtype == "scope" {
				active = true
			}
		}
	}
	if !active {
		return examples, nil
	}
	old, err := music.CompileScopePrograms(groupingFilters(before))
	if err != nil {
		return nil, err
	}
	next, err := music.CompileScopePrograms(groupingFilters(after))
	if err != nil {
		return nil, err
	}
	oldItems, err := scopeItemsForConfig(ctx, tx, before)
	if err != nil {
		return nil, err
	}
	nextItems, err := scopeItemsForConfig(ctx, tx, after)
	if err != nil {
		return nil, err
	}
	if old.UsesPlays() || next.UsesPlays() {
		oldEnvs := map[int64]music.ScopeItem{}
		nextEnvs := map[int64]music.ScopeItem{}
		for _, item := range oldItems {
			oldEnvs[item.ID] = item
		}
		for _, item := range nextItems {
			nextEnvs[item.ID] = item
		}
		plays, e := music.ScopePlays(ctx, tx, 0)
		if e != nil {
			return nil, e
		}
		for _, play := range plays {
			a, _, e := old.Resolve(ctx, play.Environment(oldEnvs[play.TrackID].Env))
			if e != nil {
				return nil, e
			}
			b, _, e := next.Resolve(ctx, play.Environment(nextEnvs[play.TrackID].Env))
			if e != nil {
				return nil, e
			}
			if a == b {
				continue
			}
			label := func(included bool) string {
				if included {
					return "Included"
				}
				return "Excluded"
			}
			examples = append(examples, NamingExample{Field: nextEnvs[play.TrackID].Title + " at " + time.Unix(play.Time, 0).UTC().Format(time.RFC3339), Before: label(a), After: label(b)})
			if len(examples) == 12 {
				break
			}
		}
		if len(examples) < 12 {
			rows, e := tx.QueryContext(ctx, `SELECT DISTINCT track_id FROM source_aggregates WHERE undated_count>0 AND track_id IS NOT NULL ORDER BY track_id`)
			if e != nil {
				return nil, e
			}
			ids := []int64{}
			for rows.Next() {
				var id int64
				if e = rows.Scan(&id); e != nil {
					rows.Close()
					return nil, e
				}
				ids = append(ids, id)
			}
			e = rows.Err()
			rows.Close()
			if e != nil {
				return nil, e
			}
			for _, id := range ids {
				a, _, e := old.ResolveCatalog(ctx, oldEnvs[id].Env)
				if e != nil {
					return nil, e
				}
				b, _, e := next.ResolveCatalog(ctx, nextEnvs[id].Env)
				if e != nil {
					return nil, e
				}
				if a == b {
					continue
				}
				label := func(included bool) string {
					if included {
						return "Included"
					}
					return "Excluded"
				}
				examples = append(examples, NamingExample{Field: "Undated counts: " + nextEnvs[id].Title, Before: label(a), After: label(b)})
				if len(examples) == 12 {
					break
				}
			}
		}
		return examples, nil
	}
	for i, item := range oldItems {
		a, _, e := old.ResolveCatalog(ctx, item.Env)
		if e != nil {
			return nil, e
		}
		b, _, e := next.ResolveCatalog(ctx, nextItems[i].Env)
		if e != nil {
			return nil, e
		}
		if a == b {
			continue
		}
		label := func(included bool) string {
			if included {
				return "Included"
			}
			return "Excluded"
		}
		album, _ := nextItems[i].Env["display.album"].Text()
		examples = append(examples, NamingExample{Field: nextItems[i].Title + " on " + album, Before: label(a), After: label(b)})
		if len(examples) == 12 {
			break
		}
	}
	return examples, nil
}

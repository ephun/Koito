package music

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gabehf/koito/internal/filters"
)

func ScopeSchema() filters.Schema {
	return filters.Schema{
		Fields: map[string]filters.Type{
			"catalog.track_ids": filters.List, "catalog.track_id": filters.Number, "catalog.appearance_id": filters.Number, "catalog.album_id": filters.Number, "catalog.artist_ids": filters.List,
			"original.title": filters.Text, "original.album": filters.Text, "original.artist": filters.Text, "original.artists": filters.List,
			"display.title": filters.Text, "display.album": filters.Text, "display.artist": filters.Text, "display.artists": filters.List,
		},
		Targets: map[string]filters.Type{"view.include": filters.Boolean},
		Labels: map[string]string{
			"catalog.track_ids": "Track appearance identifiers", "catalog.track_id": "Track identifier", "catalog.appearance_id": "Album appearance identifier", "catalog.album_id": "Album identifier", "catalog.artist_ids": "Artist identifiers",
			"original.title": "Original track title", "original.album": "Original album", "original.artist": "Original artist", "original.artists": "Original artist credits",
			"display.title": "Displayed track title", "display.album": "Displayed album", "display.artist": "Displayed artist", "display.artists": "Displayed artist credits", "view.include": "Include in the view",
		},
		Help: map[string]string{"view.include": "Choose whether matching music contributes to listening history and totals.", "catalog.track_id": "Identifies the shared track history across album appearances."},
	}
}

type ScopePrograms struct{ programs []identityProgram }

func CompileScopePrograms(rules []IdentityFilter) (ScopePrograms, error) {
	result := ScopePrograms{}
	for _, rule := range rules {
		if !rule.Enabled || rule.Type != "inclusion" || rule.Subtype != "scope" {
			continue
		}
		if rule.Program == nil {
			return result, fmt.Errorf("Scope filters need IF and SET blocks.")
		}
		compiled, err := filters.CompileProgram(*rule.Program, ScopeSchema())
		if err != nil {
			return result, err
		}
		result.programs = append(result.programs, identityProgram{rule, compiled})
	}
	return result, nil
}

func (p ScopePrograms) Resolve(ctx context.Context, input filters.Env) (bool, []IdentityIssue, error) {
	env := filters.Env{}
	for key, value := range input {
		env[key] = value
	}
	included := true
	issues := []IdentityIssue{}
	for _, program := range p.programs {
		env["view.include"] = filters.Bool(included)
		out, err := program.compiled.Run(ctx, env)
		if err != nil {
			return included, issues, err
		}
		for _, issue := range out.Issues {
			issues = append(issues, IdentityIssue{program.rule.ID, program.rule.Name, issue})
		}
		if len(out.Issues) > 0 {
			continue
		}
		if value, ok := out.Values["view.include"]; ok {
			included, _ = value.Bool()
		}
	}
	return included, issues, nil
}

type ScopeItem struct {
	ID    int64
	Title string
	Env   filters.Env
}

// Scope is evaluated per album appearance. Shared-history identifiers permit
// one condition across releases, while appearance identifiers permit exceptions.
func ScopeItems(ctx context.Context, db Database, trackID int64) ([]ScopeItem, error) {
	original := map[string]map[int64]string{"artist": {}, "album": {}, "track": {}}
	displayed := map[string]map[int64]string{"artist": {}, "album": {}, "track": {}}
	rows, err := db.QueryContext(ctx, `SELECT 'artist',artist_id,alias FROM artist_aliases WHERE is_primary=1 UNION ALL SELECT 'album',release_id,alias FROM release_aliases WHERE is_primary=1 UNION ALL SELECT 'track',track_id,alias FROM track_aliases WHERE is_primary=1`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var kind, name string
		var id int64
		if err = rows.Scan(&kind, &id, &name); err != nil {
			rows.Close()
			return nil, err
		}
		original[kind][id] = name
		displayed[kind][id] = name
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = db.QueryContext(ctx, `SELECT kind,item_id,display_name FROM catalog_names`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var kind, name string
		var id int64
		if err = rows.Scan(&kind, &id, &name); err != nil {
			rows.Close()
			return nil, err
		}
		if displayed[kind] != nil {
			displayed[kind][id] = name
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	query := `SELECT t.id,t.release_id,COALESCE(h.music_track_id,-t.id) FROM tracks t LEFT JOIN track_histories h ON h.track_id=t.id`
	args := []any{}
	if trackID != 0 {
		query += ` WHERE t.id=?`
		args = append(args, trackID)
	}
	query += ` ORDER BY t.id`
	rows, err = db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	items := []ScopeItem{}
	indices := map[int64]int{}
	historyKeys := map[int64]int64{}
	for rows.Next() {
		var item ScopeItem
		var album, history int64
		if err = rows.Scan(&item.ID, &album, &history); err != nil {
			rows.Close()
			return nil, err
		}
		item.Title = displayed["track"][item.ID]
		item.Env = filters.Env{"catalog.appearance_id": filters.Integer(item.ID), "catalog.album_id": filters.Integer(album)}
		for _, field := range []struct {
			key, kind string
			id        int64
			names     map[string]map[int64]string
		}{{"original.title", "track", item.ID, original}, {"original.album", "album", album, original}, {"display.title", "track", item.ID, displayed}, {"display.album", "album", album, displayed}} {
			if name, ok := field.names[field.kind][field.id]; ok {
				item.Env[field.key] = filters.String(name)
			}
		}
		indices[item.ID] = len(items)
		historyKeys[item.ID] = history
		items = append(items, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	originals := map[int64][]filters.Value{}
	names := map[int64][]filters.Value{}
	artistIDs := map[int64][]filters.Value{}
	for _, isDisplayed := range []bool{false, true} {
		table := "artist_tracks"
		if isDisplayed {
			table = "displayed_artist_tracks"
		}
		query := `SELECT track_id,artist_id FROM ` + table
		args := []any{}
		if trackID != 0 {
			query += ` WHERE track_id=?`
			args = append(args, trackID)
		}
		query += ` ORDER BY track_id,is_primary DESC,artist_id`
		credits, e := db.QueryContext(ctx, query, args...)
		if e != nil {
			return nil, e
		}
		for credits.Next() {
			var id, artist int64
			if e = credits.Scan(&id, &artist); e != nil {
				credits.Close()
				return nil, e
			}
			if _, ok := indices[id]; !ok {
				continue
			}
			if isDisplayed {
				names[id] = append(names[id], filters.String(displayed["artist"][artist]))
				artistIDs[id] = append(artistIDs[id], filters.Integer(artist))
			} else {
				originals[id] = append(originals[id], filters.String(original["artist"][artist]))
			}
		}
		e = credits.Err()
		credits.Close()
		if e != nil {
			return nil, e
		}
	}
	groups := map[int64][]filters.Value{}
	canonical := map[int64]int64{}
	addMember := func(history, id int64) {
		groups[history] = append(groups[history], filters.Integer(id))
		if canonical[history] == 0 || id < canonical[history] {
			canonical[history] = id
		}
	}
	if trackID != 0 && len(items) > 0 {
		members, e := db.QueryContext(ctx, `SELECT m.track_id FROM track_histories m JOIN track_histories own ON own.music_track_id=m.music_track_id WHERE own.track_id=? ORDER BY m.track_id`, trackID)
		if e != nil {
			return nil, e
		}
		for members.Next() {
			var id int64
			if e = members.Scan(&id); e != nil {
				members.Close()
				return nil, e
			}
			addMember(historyKeys[trackID], id)
		}
		e = members.Err()
		members.Close()
		if e != nil {
			return nil, e
		}
		if len(groups[historyKeys[trackID]]) == 0 {
			addMember(historyKeys[trackID], trackID)
		}
	} else {
		for _, item := range items {
			addMember(historyKeys[item.ID], item.ID)
		}
	}
	for i := range items {
		item := &items[i]
		item.Env["catalog.track_id"] = filters.Integer(canonical[historyKeys[item.ID]])
		item.Env["catalog.track_ids"] = filters.Values(groups[historyKeys[item.ID]]...)
		item.Env["catalog.artist_ids"] = filters.Values(artistIDs[item.ID]...)
		item.Env["original.artists"] = filters.Values(originals[item.ID]...)
		item.Env["display.artists"] = filters.Values(names[item.ID]...)
		if len(originals[item.ID]) == 1 {
			item.Env["original.artist"] = originals[item.ID][0]
		}
		if len(names[item.ID]) == 1 {
			item.Env["display.artist"] = names[item.ID][0]
		}
	}
	return items, nil
}

func RefreshScope(ctx context.Context, db Database, trackID int64, config ...string) error {
	var raw string
	if len(config) > 0 {
		raw = config[0]
	} else if err := db.QueryRowContext(ctx, `SELECT v.config_json FROM filter_versions v JOIN filter_state s ON s.version_id=v.id WHERE s.id=1`).Scan(&raw); err != nil {
		return err
	}
	var settings struct {
		Rules []IdentityFilter `json:"rules"`
	}
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return err
	}
	programs, err := CompileScopePrograms(settings.Rules)
	if err != nil {
		return err
	}
	for _, table := range []string{"track_scope", "track_scope_issues"} {
		query := `DELETE FROM ` + table
		args := []any{}
		if trackID != 0 {
			query += ` WHERE track_id=?`
			args = append(args, trackID)
		}
		if _, err = db.ExecContext(ctx, query, args...); err != nil {
			return err
		}
	}
	if len(programs.programs) == 0 {
		return nil
	}
	items, err := ScopeItems(ctx, db, trackID)
	if err != nil {
		return err
	}
	for _, item := range items {
		included, issues, e := programs.Resolve(ctx, item.Env)
		if e != nil {
			return e
		}
		if !included {
			if _, err = db.ExecContext(ctx, `INSERT INTO track_scope(track_id,included) VALUES(?,0)`, item.ID); err != nil {
				return err
			}
		}
		if len(issues) > 0 {
			encoded, e := json.Marshal(issues)
			if e != nil {
				return e
			}
			if _, err = db.ExecContext(ctx, `INSERT INTO track_scope_issues(track_id,issues_json) VALUES(?,?)`, item.ID, string(encoded)); err != nil {
				return err
			}
		}
	}
	return nil
}

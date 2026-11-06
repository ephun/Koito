package sources

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/gabehf/koito/internal/filters"
	"github.com/gabehf/koito/internal/music"
	"strings"
)

func expressionReads(e filters.Expr, field string) bool {
	if e.Op == "field" && e.Field == field {
		return true
	}
	for _, arg := range e.Args {
		if expressionReads(arg, field) {
			return true
		}
	}
	return false
}
func statementsRead(steps []filters.Statement, field string) bool {
	for _, step := range steps {
		if step.If != nil && expressionReads(*step.If, field) || step.Value != nil && expressionReads(*step.Value, field) {
			return true
		}
		if statementsRead(step.Then, field) || statementsRead(step.Else, field) {
			return true
		}
		for _, branch := range step.ElseIf {
			if expressionReads(branch.If, field) || statementsRead(branch.Then, field) {
				return true
			}
		}
	}
	return false
}

// Catalog corrections explicitly select a catalog context. Record programs
// continue to use their original source context during projection.
func CatalogName(ctx context.Context, config RuleConfig, kind string, id int64, original string) (string, []FilterIssue, error) {
	programs, err := compileNamingDomain(config, true)
	if err != nil {
		return original, nil, err
	}
	return resolveCatalogName(ctx, programs, kind, id, original)
}

type catalogNamingState struct {
	Name             string
	Genres           []string
	Artists, Primary []int64
	CreditsAssigned  bool
	Aliases          []string
	AliasesAssigned  bool
}

func aliasNames(value filters.Value) ([]string, bool) {
	values, ok := value.Items()
	if !ok {
		return nil, false
	}
	names := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		name, ok := value.Text()
		if !ok || strings.TrimSpace(name) == "" {
			return nil, false
		}
		if !seen[name] {
			names = append(names, name)
			seen[name] = true
		}
	}
	return names, true
}
func aliasList(names []string) filters.Value {
	values := []filters.Value{}
	for _, name := range names {
		values = append(values, filters.String(name))
	}
	return filters.Values(values...)
}
func identifierList(ids []int64) filters.Value {
	values := []filters.Value{}
	for _, id := range ids {
		values = append(values, filters.Integer(id))
	}
	return filters.Values(values...)
}
func creditIdentifiers(value filters.Value) ([]int64, bool) {
	values, ok := value.Items()
	if !ok {
		return nil, false
	}
	ids := []int64{}
	seen := map[int64]bool{}
	for _, value := range values {
		id, ok := value.Int64()
		if !ok || id <= 0 {
			return nil, false
		}
		if !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	return ids, true
}
func resolveCatalogName(ctx context.Context, programs []namingProgram, kind string, id int64, original string, metadata ...filters.Env) (string, []FilterIssue, error) {
	result, issues, err := resolveCatalogNaming(ctx, programs, kind, id, original, metadata...)
	return result.Name, issues, err
}
func resolveCatalogNaming(ctx context.Context, programs []namingProgram, kind string, id int64, original string, metadata ...filters.Env) (catalogNamingState, []FilterIssue, error) {
	field := map[string]string{"artist": "artist", "album": "album", "track": "title"}[kind]
	if field == "" {
		return catalogNamingState{Name: original}, nil, fmt.Errorf("Unknown catalog context.")
	}
	target := "display." + field
	env := filters.Env{"catalog.kind": filters.String(kind), "catalog.id": filters.Integer(id), "original." + field: filters.String(original), target: filters.String(original)}
	for _, extra := range metadata {
		for key, value := range extra {
			env[key] = value
		}
	}
	current := catalogNamingState{Name: original}
	if kind == "artist" {
		env["catalog.artist_id"] = filters.Integer(id)
		labels, _ := env["source.genres"].Items()
		current.Genres = []string{}
		seen := map[string]bool{}
		for _, label := range labels {
			if text, ok := label.Text(); ok && strings.TrimSpace(text) != "" && !seen[text] {
				current.Genres = append(current.Genres, text)
				seen[text] = true
			}
		}
		env["artist.genres"] = aliasList(current.Genres)
	}
	current.Aliases, _ = aliasNames(env["original.aliases"])
	env["display.aliases"] = aliasList(current.Aliases)
	current.Artists, _ = creditIdentifiers(env["catalog.artist_ids"])
	current.Primary, _ = creditIdentifiers(env["original.primary_artist_ids"])
	env["display.artist_ids"] = identifierList(current.Artists)
	env["display.primary_artist_ids"] = identifierList(current.Primary)
	issues := []FilterIssue{}
	for _, program := range programs {
		_, genres := namingTargets(program.rule.Program)
		if !statementsRead(program.rule.Program.Steps, "catalog.kind") && (!genres || kind != "artist") {
			continue
		}
		if program.rule.Scope != (RuleScope{}) {
			issues = append(issues, FilterIssue{program.rule.ID, program.rule.Name, filters.Issue{Path: "catalog", Reason: "Put catalog conditions inside IF."}})
			continue
		}
		out, e := program.program.Run(ctx, env)
		if e != nil {
			return current, issues, e
		}
		for _, issue := range out.Issues {
			issues = append(issues, FilterIssue{program.rule.ID, program.rule.Name, issue})
		}
		if len(out.Issues) > 0 {
			continue
		}

		candidate := current
		valid := true
		invalidGenres := false
		for key, value := range out.Values {
			switch key {
			case target:
				text, ok := value.Text()
				if !ok || strings.TrimSpace(text) == "" {
					valid = false
				} else {
					candidate.Name = text
				}
			case "display.aliases":
				names, ok := aliasNames(value)
				if !ok {
					valid = false
				} else {
					candidate.Aliases = names
					candidate.AliasesAssigned = true
				}
			case "artist.genres":
				labels, ok := aliasNames(value)
				if kind != "artist" || !ok {
					valid = false
					invalidGenres = true
				} else {
					candidate.Genres = labels
				}
			case "display.artist_ids":
				ids, ok := creditIdentifiers(value)
				if kind == "artist" || !ok {
					valid = false
				} else {
					candidate.Artists = ids
					candidate.CreditsAssigned = true
				}
			case "display.primary_artist_ids":
				ids, ok := creditIdentifiers(value)
				if kind == "artist" || !ok {
					valid = false
				} else {
					candidate.Primary = ids
					candidate.CreditsAssigned = true
				}
			default:
				valid = false
			}
		}
		if candidate.CreditsAssigned {
			available, _ := creditIdentifiers(env["catalog.available_artist_ids"])
			known := map[int64]bool{}
			for _, id := range available {
				known[id] = true
			}
			credited := map[int64]bool{}
			for _, id := range candidate.Artists {
				credited[id] = true
				if !known[id] {
					valid = false
				}
			}
			for _, id := range candidate.Primary {
				if !credited[id] {
					valid = false
				}
			}
		}
		if !valid {
			issue := filters.Issue{Path: "catalog", Reason: "Choose nonempty names and aliases, existing artists, and primary artists included in the credits."}
			if invalidGenres {
				issue = filters.Issue{Path: "artist.genres", Reason: "Choose nonempty genre labels in an artist context."}
			}
			issues = append(issues, FilterIssue{program.rule.ID, program.rule.Name, issue})
			continue
		}
		current = candidate
		env["display.aliases"] = aliasList(current.Aliases)
		env[target] = filters.String(current.Name)
		env["display.artist_ids"] = identifierList(current.Artists)
		env["display.primary_artist_ids"] = identifierList(current.Primary)
		if kind == "artist" {
			env["artist.genres"] = aliasList(current.Genres)
		}

	}
	return current, issues, nil
}

func publishCatalogNames(ctx context.Context, tx *sql.Tx, config RuleConfig, targets ...catalogNameItem) error {
	programs, err := compileNamingDomain(config, true)
	if err != nil {
		return err
	}
	selected := []namingProgram{}
	for _, program := range programs {
		_, genres := namingTargets(program.rule.Program)
		if genres || statementsRead(program.rule.Program.Steps, "catalog.kind") {
			selected = append(selected, program)
		}
	}
	for _, query := range []string{`DELETE FROM catalog_aliases`, `DELETE FROM catalog_alias_sets`, `DELETE FROM catalog_names`, `DELETE FROM catalog_name_issues`, `DELETE FROM catalog_credits`, `DELETE FROM catalog_credit_sets`} {
		args := []any{}
		if len(targets) > 0 {
			query += " WHERE kind=? AND item_id=?"
			args = []any{targets[0].kind, targets[0].id}
		}
		if _, err = tx.ExecContext(ctx, query, args...); err != nil {
			return err
		}
	}
	if len(selected) == 0 {
		return nil
	}
	items := targets
	if len(items) == 0 {
		items, err = catalogNameItems(ctx, tx)
		if err != nil {
			return err
		}
	}
	if err = addCatalogGenreInputs(ctx, tx, items); err != nil {
		return err
	}
	for _, item := range items {
		resolved, issues, e := resolveCatalogNaming(ctx, selected, item.kind, item.id, item.name, item.metadata)
		issues = catalogDomainIssues(selected, issues, false)
		name := resolved.Name
		if e != nil {
			return e
		}

		if resolved.AliasesAssigned {
			if _, err = tx.ExecContext(ctx, `INSERT INTO catalog_alias_sets(kind,item_id) VALUES(?,?)`, item.kind, item.id); err != nil {
				return err
			}
			for _, alias := range resolved.Aliases {
				if _, err = tx.ExecContext(ctx, `INSERT INTO catalog_aliases(kind,item_id,alias) VALUES(?,?,?)`, item.kind, item.id, alias); err != nil {
					return err
				}
			}
		}
		if resolved.CreditsAssigned {
			if _, err = tx.ExecContext(ctx, `INSERT INTO catalog_credit_sets(kind,item_id) VALUES(?,?)`, item.kind, item.id); err != nil {
				return err
			}
			primary := map[int64]bool{}
			for _, id := range resolved.Primary {
				primary[id] = true
			}
			for position, id := range resolved.Artists {
				if _, err = tx.ExecContext(ctx, `INSERT INTO catalog_credits(kind,item_id,artist_id,is_primary,position) VALUES(?,?,?,?,?)`, item.kind, item.id, id, primary[id], position); err != nil {
					return err
				}
			}
		}
		if name != item.name {
			if _, err = tx.ExecContext(ctx, `INSERT INTO catalog_names(kind,item_id,display_name) VALUES(?,?,?)`, item.kind, item.id, name); err != nil {
				return err
			}
		}
		if len(issues) > 0 {
			raw, e := json.Marshal(issues)
			if e != nil {
				return e
			}
			if _, err = tx.ExecContext(ctx, `INSERT INTO catalog_name_issues(kind,item_id,issues_json) VALUES(?,?,?)`, item.kind, item.id, string(raw)); err != nil {
				return err
			}
		}
	}
	return nil
}

func catalogDomainIssues(programs []namingProgram, issues []FilterIssue, genres bool) []FilterIssue {
	selected := map[string]bool{}
	for _, program := range programs {
		names, labels := namingTargets(program.rule.Program)
		selected[program.rule.ID] = names
		if genres {
			selected[program.rule.ID] = labels
		}
	}
	result := []FilterIssue{}
	for _, issue := range issues {
		if selected[issue.FilterID] {
			result = append(result, issue)
		}
	}
	return result
}

type catalogNameItem struct {
	kind, name string
	id         int64
	albumID    int64
	metadata   filters.Env
}

func catalogNameItems(ctx context.Context, tx *sql.Tx) ([]catalogNameItem, error) {
	rows, err := tx.QueryContext(ctx, `SELECT 'artist',id,name,0 FROM artists_with_name UNION ALL SELECT 'album',id,title,id FROM releases_with_title UNION ALL SELECT 'track',id,title,release_id FROM tracks_with_title ORDER BY 1,2`)
	if err != nil {
		return nil, err
	}
	items := []catalogNameItem{}
	for rows.Next() {
		var entry catalogNameItem
		if err = rows.Scan(&entry.kind, &entry.id, &entry.name, &entry.albumID); err != nil {
			rows.Close()
			return nil, err
		}
		items = append(items, entry)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}

	indices := map[string]map[int64]int{}
	for i := range items {
		item := &items[i]
		item.metadata = filters.Env{}
		if indices[item.kind] == nil {
			indices[item.kind] = map[int64]int{}
		}
		indices[item.kind][item.id] = i
	}
	aliasRows, err := tx.QueryContext(ctx, `SELECT 'artist',artist_id,alias FROM artist_aliases UNION ALL SELECT 'album',release_id,alias FROM release_aliases UNION ALL SELECT 'track',track_id,alias FROM track_aliases ORDER BY 1,2,3`)
	if err != nil {
		return nil, err
	}
	aliases := map[int][]filters.Value{}
	for aliasRows.Next() {
		var kind, name string
		var id int64
		if err = aliasRows.Scan(&kind, &id, &name); err != nil {
			aliasRows.Close()
			return nil, err
		}
		if index, ok := indices[kind][id]; ok {
			aliases[index] = append(aliases[index], filters.String(name))
		}
	}
	err = aliasRows.Err()
	aliasRows.Close()
	if err != nil {
		return nil, err
	}
	for i := range items {
		items[i].metadata["original.aliases"] = filters.Values(aliases[i]...)
	}
	credits := map[int][]filters.Value{}
	creditIDs := map[int][]filters.Value{}
	primaryIDs := map[int][]filters.Value{}
	availableArtists := []filters.Value{}
	for _, item := range items {
		if item.kind == "artist" {
			availableArtists = append(availableArtists, filters.Integer(item.id))
		}
	}
	rows, err = tx.QueryContext(ctx, `SELECT 'album',ar.release_id,ar.artist_id,a.name,ar.is_primary FROM artist_releases ar JOIN artists_with_name a ON a.id=ar.artist_id UNION ALL SELECT 'track',at.track_id,at.artist_id,a.name,at.is_primary FROM artist_tracks at JOIN artists_with_name a ON a.id=at.artist_id ORDER BY 1,2,5 DESC,3`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var kind, name string
		var id, artist, primary int64
		if err = rows.Scan(&kind, &id, &artist, &name, &primary); err != nil {
			rows.Close()
			return nil, err
		}
		if index, ok := indices[kind][id]; ok {
			credits[index] = append(credits[index], filters.String(name))
			creditIDs[index] = append(creditIDs[index], filters.Integer(artist))
			if primary != 0 {
				primaryIDs[index] = append(primaryIDs[index], filters.Integer(artist))
			}
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	for i := range items {
		item := &items[i]
		if item.kind == "artist" {
			credits[i] = []filters.Value{filters.String(item.name)}
			creditIDs[i] = []filters.Value{filters.Integer(item.id)}
		}
		item.metadata["catalog.available_artist_ids"] = filters.Values(availableArtists...)
		item.metadata["original.primary_artist_ids"] = filters.Values(primaryIDs[i]...)
		item.metadata["original.artists"] = filters.Values(credits[i]...)
		item.metadata["catalog.artist_ids"] = filters.Values(creditIDs[i]...)
		if len(credits[i]) == 1 {
			item.metadata["original.artist"] = credits[i][0]
		}
		if item.kind == "track" {
			origin, e := music.TrackOrigin(ctx, tx, item.id)
			if e != nil {
				return nil, e
			}
			item.metadata["catalog.id"] = filters.Integer(origin)
			if album, ok := indices["album"][item.albumID]; ok {
				item.metadata["original.album"] = filters.String(items[album].name)
			}
		}
	}
	return items, nil
}

func previewCatalogNames(ctx context.Context, tx *sql.Tx, before, after RuleConfig) ([]NamingExample, error) {
	selectPrograms := func(config RuleConfig) ([]namingProgram, error) {
		programs, err := compileNamingDomain(config, true)
		if err != nil {
			return nil, err
		}
		selected := []namingProgram{}
		for _, program := range programs {
			_, genres := namingTargets(program.rule.Program)
			if genres || statementsRead(program.rule.Program.Steps, "catalog.kind") {
				selected = append(selected, program)
			}
		}
		return selected, nil
	}
	old, err := selectPrograms(before)
	if err != nil {
		return nil, err
	}
	next, err := selectPrograms(after)
	if err != nil {
		return nil, err
	}
	examples := []NamingExample{}
	if len(old) == 0 && len(next) == 0 {
		return examples, nil
	}
	items, err := catalogNameItems(ctx, tx)
	if err != nil {
		return nil, err
	}
	if err = addCatalogGenreInputs(ctx, tx, items); err != nil {
		return nil, err
	}
	artistNames := map[int64]string{}
	for _, item := range items {
		if item.kind == "artist" {
			artistNames[item.id] = item.name
		}
	}
	creditNames := func(ids []int64) string {
		names := []string{}
		for _, id := range ids {
			names = append(names, artistNames[id])
		}
		raw, _ := json.Marshal(names)
		return string(raw)
	}
	for _, item := range items {
		a, _, e := resolveCatalogNaming(ctx, old, item.kind, item.id, item.name, item.metadata)
		if e != nil {
			return nil, e
		}
		b, _, e := resolveCatalogNaming(ctx, next, item.kind, item.id, item.name, item.metadata)
		if e != nil {
			return nil, e
		}
		if a.Name != b.Name {
			examples = append(examples, NamingExample{Field: strings.ToUpper(item.kind[:1]) + item.kind[1:] + " name", Before: a.Name, After: b.Name})
			if len(examples) == 12 {
				break
			}
		}
		x, _ := json.Marshal(a.Aliases)
		y, _ := json.Marshal(b.Aliases)
		if string(x) != string(y) {
			examples = append(examples, NamingExample{Field: "Aliases: " + item.name, Before: string(x), After: string(y)})
		}
		for _, change := range []struct {
			field         string
			before, after []int64
		}{{"Artist credits", a.Artists, b.Artists}, {"Primary artist credits", a.Primary, b.Primary}} {
			x, _ := json.Marshal(change.before)
			y, _ := json.Marshal(change.after)
			if string(x) != string(y) {
				examples = append(examples, NamingExample{Field: change.field + ": " + item.name, Before: creditNames(change.before), After: creditNames(change.after)})
			}
		}
		if len(examples) >= 12 {
			return examples[:12], nil
		}

	}
	return examples, nil
}

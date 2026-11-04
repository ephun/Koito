package sources

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/gabehf/koito/internal/filters"
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
	programs, err := compileNaming(config)
	if err != nil {
		return original, nil, err
	}
	return resolveCatalogName(ctx, programs, kind, id, original)
}
func resolveCatalogName(ctx context.Context, programs []namingProgram, kind string, id int64, original string, metadata ...filters.Env) (string, []FilterIssue, error) {
	field := map[string]string{"artist": "artist", "album": "album", "track": "title"}[kind]
	if field == "" {
		return original, nil, fmt.Errorf("Unknown catalog context.")
	}
	target := "display." + field
	env := filters.Env{"catalog.kind": filters.String(kind), "catalog.id": filters.Integer(id), "original." + field: filters.String(original), target: filters.String(original)}
	for _, extra := range metadata {
		for key, value := range extra {
			env[key] = value
		}
	}
	current := original
	issues := []FilterIssue{}
	for _, program := range programs {
		if !statementsRead(program.rule.Program.Steps, "catalog.kind") {
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
		for key, value := range out.Values {
			text, ok := value.Text()
			if key != target || !ok || strings.TrimSpace(text) == "" {
				valid = false
				break
			}
			candidate = text
		}
		if !valid {
			issues = append(issues, FilterIssue{program.rule.ID, program.rule.Name, filters.Issue{Path: target, Reason: "Choose a nonempty name for this catalog context."}})
			continue
		}
		current = candidate
		env[target] = filters.String(current)
	}
	return current, issues, nil
}

func publishCatalogNames(ctx context.Context, tx *sql.Tx, config RuleConfig) error {
	programs, err := compileNaming(config)
	if err != nil {
		return err
	}
	selected := []namingProgram{}
	for _, program := range programs {
		if statementsRead(program.rule.Program.Steps, "catalog.kind") {
			selected = append(selected, program)
		}
	}
	for _, query := range []string{`DELETE FROM catalog_names`, `DELETE FROM catalog_name_issues`} {
		if _, err = tx.ExecContext(ctx, query); err != nil {
			return err
		}
	}
	if len(selected) == 0 {
		return nil
	}
	items, err := catalogNameItems(ctx, tx)
	if err != nil {
		return err
	}
	for _, item := range items {
		name, issues, e := resolveCatalogName(ctx, selected, item.kind, item.id, item.name, item.metadata)
		if e != nil {
			return e
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
	credits := map[int][]filters.Value{}
	creditIDs := map[int][]filters.Value{}
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
		item.metadata["original.artists"] = filters.Values(credits[i]...)
		item.metadata["catalog.artist_ids"] = filters.Values(creditIDs[i]...)
		if len(credits[i]) == 1 {
			item.metadata["original.artist"] = credits[i][0]
		}
		if item.kind == "track" {
			if album, ok := indices["album"][item.albumID]; ok {
				item.metadata["original.album"] = filters.String(items[album].name)
			}
		}
	}
	return items, nil
}

func previewCatalogNames(ctx context.Context, tx *sql.Tx, before, after RuleConfig) ([]NamingExample, error) {
	selectPrograms := func(config RuleConfig) ([]namingProgram, error) {
		programs, err := compileNaming(config)
		if err != nil {
			return nil, err
		}
		selected := []namingProgram{}
		for _, program := range programs {
			if statementsRead(program.rule.Program.Steps, "catalog.kind") {
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
	for _, item := range items {
		a, _, e := resolveCatalogName(ctx, old, item.kind, item.id, item.name, item.metadata)
		if e != nil {
			return nil, e
		}
		b, _, e := resolveCatalogName(ctx, next, item.kind, item.id, item.name, item.metadata)
		if e != nil {
			return nil, e
		}
		if a != b {
			examples = append(examples, NamingExample{Field: strings.ToUpper(item.kind[:1]) + item.kind[1:] + " name", Before: a, After: b})
			if len(examples) == 12 {
				break
			}
		}
	}
	return examples, nil
}

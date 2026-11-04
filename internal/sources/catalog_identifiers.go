package sources

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/gabehf/koito/internal/filters"
	"github.com/gabehf/koito/internal/music"
	"github.com/google/uuid"
	"sort"
)

func catalogIdentifier(ctx context.Context, config RuleConfig, kind string, id int64, original string) (string, bool, []FilterIssue, error) {
	programs, err := compileCatalogIdentifiers(config)
	if err != nil {
		return original, false, nil, err
	}
	return resolveCatalogIdentifier(ctx, programs, kind, id, original)
}

type catalogIdentifierProgram struct {
	rule    Rule
	program *filters.CompiledProgram
}

func compileCatalogIdentifiers(config RuleConfig) ([]catalogIdentifierProgram, error) {
	programs := []catalogIdentifierProgram{}
	for _, rule := range config.Rules {
		if !rule.Enabled || rule.Type != "matching" || rule.Subtype != "music" || rule.Program == nil || !statementsRead(rule.Program.Steps, "catalog.kind") {
			continue
		}
		compiled, err := filters.CompileProgram(*rule.Program, music.MatchingSchema())
		if err != nil {
			return nil, err
		}
		programs = append(programs, catalogIdentifierProgram{rule, compiled})
	}
	sort.SliceStable(programs, func(i, j int) bool {
		a, b := 0, 0
		if programs[i].rule.Priority != nil {
			a = *programs[i].rule.Priority
		}
		if programs[j].rule.Priority != nil {
			b = *programs[j].rule.Priority
		}
		return a < b
	})
	return programs, nil
}
func resolveCatalogIdentifier(ctx context.Context, programs []catalogIdentifierProgram, kind string, id int64, original string) (string, bool, []FilterIssue, error) {
	current := original
	assigned := false
	issues := []FilterIssue{}
	env := filters.Env{"catalog.kind": filters.String(kind), "catalog.id": filters.Integer(id), "original.identifier": filters.String(original), "music.identifier": filters.String(original)}
	for _, entry := range programs {
		rule, program := entry.rule, entry.program
		out, err := program.Run(ctx, env)
		if err != nil {
			return current, assigned, issues, err
		}
		for _, issue := range out.Issues {
			issues = append(issues, FilterIssue{rule.ID, rule.Name, issue})
		}
		if len(out.Issues) > 0 {
			continue
		}
		validContext := true
		for key := range out.Values {
			if key != "music.identifier" {
				validContext = false
			}
		}
		if !validContext {
			issues = append(issues, FilterIssue{rule.ID, rule.Name, filters.Issue{Path: "music.identifier", Reason: "Use identifier actions for this catalog context."}})
			continue
		}
		value, ok := out.Values["music.identifier"]
		if !ok {
			continue
		}
		text, valid := value.Text()
		if text != "" {
			parsed, e := uuid.Parse(text)
			if e != nil || parsed == uuid.Nil {
				valid = false
			} else {
				text = parsed.String()
			}
		}
		if !valid {
			issues = append(issues, FilterIssue{rule.ID, rule.Name, filters.Issue{Path: "music.identifier", Reason: "Choose a MusicBrainz identifier or empty text."}})
			continue
		}
		current = text
		assigned = true
		env["music.identifier"] = filters.String(current)
	}
	return current, assigned, issues, nil
}

func publishCatalogIdentifiers(ctx context.Context, tx *sql.Tx, config RuleConfig) error {
	programs, err := compileCatalogIdentifiers(config)
	if err != nil {
		return err
	}
	for _, q := range []string{`DELETE FROM catalog_identifiers`, `DELETE FROM catalog_identifier_issues`} {
		if _, err = tx.ExecContext(ctx, q); err != nil {
			return err
		}
	}
	if len(programs) == 0 {
		return nil
	}
	items, err := catalogIdentifierItems(ctx, tx)
	if err != nil {
		return err
	}
	for _, item := range items {
		value, assigned, issues, e := resolveCatalogIdentifier(ctx, programs, item.kind, item.id, item.original)
		if e != nil {
			return e
		}
		if assigned {
			if _, err = tx.ExecContext(ctx, `INSERT INTO catalog_identifiers(kind,item_id,identifier) VALUES(?,?,?)`, item.kind, item.id, value); err != nil {
				return err
			}
		}
		if len(issues) > 0 {
			raw, e := json.Marshal(issues)
			if e != nil {
				return e
			}
			if _, err = tx.ExecContext(ctx, `INSERT INTO catalog_identifier_issues(kind,item_id,issues_json) VALUES(?,?,?)`, item.kind, item.id, string(raw)); err != nil {
				return err
			}
		}
	}
	return nil
}

type catalogIdentifierItem struct {
	kind, original string
	id             int64
}

func catalogIdentifierItems(ctx context.Context, tx *sql.Tx) ([]catalogIdentifierItem, error) {
	rows, err := tx.QueryContext(ctx, `SELECT 'artist',id,COALESCE(musicbrainz_id,'') FROM artists UNION ALL SELECT 'album',id,COALESCE(musicbrainz_id,'') FROM releases UNION ALL SELECT 'track',t.id,COALESCE(t.musicbrainz_id,(SELECT rec.musicbrainz_id FROM track_recordings tr JOIN recordings rec ON rec.recording_key=tr.recording_key WHERE tr.track_id=t.id),'') FROM tracks t ORDER BY 1,2`)
	if err != nil {
		return nil, err
	}
	items := []catalogIdentifierItem{}
	for rows.Next() {
		var item catalogIdentifierItem
		if err = rows.Scan(&item.kind, &item.id, &item.original); err != nil {
			rows.Close()
			return nil, err
		}
		items = append(items, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}

	return items, nil
}

func previewCatalogIdentifiers(ctx context.Context, tx *sql.Tx, before, after RuleConfig) ([]NamingExample, error) {
	old, err := compileCatalogIdentifiers(before)
	if err != nil {
		return nil, err
	}
	next, err := compileCatalogIdentifiers(after)
	if err != nil {
		return nil, err
	}
	examples := []NamingExample{}
	if len(old) == 0 && len(next) == 0 {
		return examples, nil
	}
	items, err := catalogIdentifierItems(ctx, tx)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		a, _, _, e := resolveCatalogIdentifier(ctx, old, item.kind, item.id, item.original)
		if e != nil {
			return nil, e
		}
		b, _, _, e := resolveCatalogIdentifier(ctx, next, item.kind, item.id, item.original)
		if e != nil {
			return nil, e
		}
		if a != b {
			examples = append(examples, NamingExample{Field: item.kind + " MusicBrainz identifier", Before: a, After: b})
			if len(examples) == 12 {
				break
			}
		}
	}
	return examples, nil
}

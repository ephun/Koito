package sources

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/gabehf/koito/internal/filters"
	"github.com/gabehf/koito/internal/music"
)

func compileAlbumClassification(config RuleConfig) ([]namingProgram, error) {
	programs := []namingProgram{}
	for _, rule := range config.Rules {
		if !rule.Enabled || rule.Type != "presentation" || rule.Subtype != "grouping" || rule.Program == nil || !statementsRead(rule.Program.Steps, "catalog.kind") {
			continue
		}
		compiled, err := filters.CompileProgram(*rule.Program, music.GroupingSchema())
		if err != nil {
			return nil, err
		}
		programs = append(programs, namingProgram{rule, compiled})
	}
	return programs, nil
}

func publishAlbumClassification(ctx context.Context, tx *sql.Tx, config RuleConfig) error {
	programs, err := compileAlbumClassification(config)
	if err != nil {
		return err
	}
	for _, q := range []string{`DELETE FROM album_classification`, `DELETE FROM album_classification_issues`} {
		if _, err := tx.ExecContext(ctx, q); err != nil {
			return err
		}
	}
	if len(programs) == 0 {
		return nil
	}
	albums, err := classificationAlbums(ctx, tx)
	if err != nil {
		return err
	}
	for _, album := range albums {
		current, assigned, issues, e := resolveAlbumClassification(ctx, programs, album)
		if e != nil {
			return e
		}
		if assigned {
			if _, err = tx.ExecContext(ctx, `INSERT INTO album_classification(album_id,various_artists) VALUES(?,?)`, album.id, current); err != nil {
				return err
			}
		}
		if len(issues) > 0 {
			raw, e := json.Marshal(issues)
			if e != nil {
				return e
			}
			if _, err = tx.ExecContext(ctx, `INSERT INTO album_classification_issues(album_id,issues_json) VALUES(?,?)`, album.id, string(raw)); err != nil {
				return err
			}
		}
	}
	return nil
}

type classificationAlbum struct {
	id       int64
	title    string
	various  bool
	metadata filters.Env
}

func resolveAlbumClassification(ctx context.Context, programs []namingProgram, album classificationAlbum) (bool, bool, []FilterIssue, error) {
	current, assigned := album.various, false
	issues := []FilterIssue{}
	env := filters.Env{"catalog.kind": filters.String("album"), "catalog.id": filters.Integer(album.id), "catalog.album_id": filters.Integer(album.id), "original.album": filters.String(album.title), "display.album": filters.String(album.title), "original.various_artists": filters.Bool(album.various), "group.various_artists": filters.Bool(current)}
	for key, value := range album.metadata {
		env[key] = value
	}
	for _, program := range programs {
		out, e := program.program.Run(ctx, env)
		if e != nil {
			return current, assigned, issues, e
		}
		for _, issue := range out.Issues {
			issues = append(issues, FilterIssue{program.rule.ID, program.rule.Name, issue})
		}
		if len(out.Issues) > 0 {
			continue
		}
		if value, ok := out.Values["group.various_artists"]; ok {
			current, _ = value.Bool()
			assigned = true
			env["group.various_artists"] = value
		}
	}

	return current, assigned, issues, nil
}

func previewAlbumClassification(ctx context.Context, tx *sql.Tx, before, after RuleConfig) ([]NamingExample, error) {
	old, err := compileAlbumClassification(before)
	if err != nil {
		return nil, err
	}
	next, err := compileAlbumClassification(after)
	if err != nil {
		return nil, err
	}
	examples := []NamingExample{}
	if len(old) == 0 && len(next) == 0 {
		return examples, nil
	}
	albums, err := classificationAlbums(ctx, tx)
	if err != nil {
		return nil, err
	}
	for _, item := range albums {
		a, _, _, e := resolveAlbumClassification(ctx, old, item)
		if e != nil {
			return nil, e
		}
		b, _, _, e := resolveAlbumClassification(ctx, next, item)
		if e != nil {
			return nil, e
		}
		if a != b {
			examples = append(examples, NamingExample{Field: "Various Artists: " + item.title, Before: fmt.Sprint(a), After: fmt.Sprint(b)})
			if len(examples) == 12 {
				break
			}
		}
	}
	return examples, nil
}

func classificationAlbums(ctx context.Context, tx *sql.Tx) ([]classificationAlbum, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id,title,various_artists FROM releases_with_title ORDER BY id`)
	if err != nil {
		return nil, err
	}
	albums := []classificationAlbum{}
	for rows.Next() {
		var item classificationAlbum
		if err = rows.Scan(&item.id, &item.title, &item.various); err != nil {
			rows.Close()
			return nil, err
		}
		albums = append(albums, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}

	items, err := catalogNameItems(ctx, tx)
	if err != nil {
		return nil, err
	}
	metadata := map[int64]filters.Env{}
	for _, item := range items {
		if item.kind == "album" {
			metadata[item.id] = item.metadata
		}
	}
	for i := range albums {
		albums[i].metadata = metadata[albums[i].id]
	}
	return albums, nil
}

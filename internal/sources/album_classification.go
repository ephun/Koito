package sources

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/gabehf/koito/internal/filters"
	"github.com/gabehf/koito/internal/music"
)

func compileAlbumClassification(config RuleConfig) (music.GroupingPrograms, error) {
	return music.CompileGroupingPrograms(groupingFilters(config))
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
	if !programs.Active() {
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

func resolveAlbumClassification(ctx context.Context, programs music.GroupingPrograms, album classificationAlbum) (bool, bool, []FilterIssue, error) {
	env := music.AlbumGroupingEnvironment(album.id, album.title, album.various, album.metadata)
	result, issues, err := programs.ResolveValues(ctx, env)
	converted := []FilterIssue{}
	for _, issue := range issues {
		if programs.WritesClassification(issue.ID) {
			converted = append(converted, FilterIssue{issue.ID, issue.Name, issue.Issue})
		}
	}
	return result.VariousArtists, result.VariousAssigned, converted, err
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
	displays, err := music.AlbumDisplays(ctx, tx, 0)
	if err != nil {
		return nil, err
	}
	for i := range albums {
		albums[i].metadata = music.WithAlbumDisplay(albums[i].metadata, displays[albums[i].id])
	}
	return albums, nil
}

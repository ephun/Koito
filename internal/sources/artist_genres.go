package sources

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/gabehf/koito/internal/filters"
	"sort"
	"strings"
)

func artistGenreProgram(id int64) *filters.Program {
	condition := operation("eq", fieldExpr("catalog.artist_id"), literalExpr(id))
	value := fieldExpr("source.genres")
	return &filters.Program{Steps: []filters.Statement{{If: &condition, Then: []filters.Statement{{Set: "artist.genres", Value: &value}}}}}
}

func displayedNameProgram() *filters.Program {
	condition := literalExpr(true)
	value := fieldExpr("original.title")
	return &filters.Program{Steps: []filters.Statement{{If: &condition, Then: []filters.Statement{{Set: "display.title", Value: &value}}}}}
}

// Artist genre assignments use Naming programs over the artist's source-label
// collection. Their publication remains separate from the preserved evidence.
func ArtistGenres(ctx context.Context, config RuleConfig, artistID int64, name string, source []string) ([]string, []FilterIssue, error) {
	programs, err := compileNamingDomain(config, true)
	if err != nil {
		return nil, nil, err
	}
	return resolveArtistGenres(ctx, programs, artistID, name, source)
}

func resolveArtistGenres(ctx context.Context, programs []namingProgram, artistID int64, name string, source []string, evidence ...filters.Value) ([]string, []FilterIssue, error) {
	values := []filters.Value{}
	for _, label := range source {
		values = append(values, filters.String(label))
	}
	env := filters.Env{"catalog.kind": filters.String("artist"), "catalog.id": filters.Integer(artistID), "catalog.artist_id": filters.Integer(artistID), "original.artist": filters.String(name), "display.artist": filters.String(name), "source.genres": filters.Values(values...), "source.genre_records": filters.Values(evidence...), "artist.genres": filters.Values(values...)}
	current := append([]string{}, source...)
	issues := []FilterIssue{}
	for _, program := range programs {
		if program.rule.Scope != (RuleScope{}) {
			continue
		}
		out, e := program.program.Run(ctx, env)
		if e != nil {
			return nil, issues, e
		}
		for _, issue := range out.Issues {
			issues = append(issues, FilterIssue{program.rule.ID, program.rule.Name, issue})
		}
		if len(out.Issues) > 0 {
			continue
		}
		value, assigned := out.Values["artist.genres"]
		if !assigned {
			continue
		}
		items, ok := value.Items()
		next := []string{}
		for _, item := range items {
			text, valid := item.Text()
			if !valid || strings.TrimSpace(text) == "" {
				ok = false
				break
			}
			next = append(next, text)
		}
		if !ok {
			issues = append(issues, FilterIssue{program.rule.ID, program.rule.Name, filters.Issue{Path: "artist.genres", Reason: "Artist genres require a list of nonempty text labels."}})
			continue
		}
		current = next
		env["artist.genres"] = value
	}
	result := []string{}
	seen := map[string]bool{}
	for _, label := range current {
		if strings.TrimSpace(label) != "" && !seen[label] {
			seen[label] = true
			result = append(result, label)
		}
	}
	return result, issues, nil
}

func publishArtistGenres(ctx context.Context, tx *sql.Tx, config RuleConfig) error {
	programs, err := compileNamingDomain(config, true)
	if err != nil {
		return err
	}
	artists, err := loadArtistGenreInputs(ctx, tx)
	if err != nil {
		return err
	}
	for _, q := range []string{`DELETE FROM artist_genres`, `DELETE FROM artist_genre_issues`} {
		if _, err = tx.ExecContext(ctx, q); err != nil {
			return err
		}
	}
	insert, err := tx.PrepareContext(ctx, `INSERT INTO artist_genres(artist_id,genre) VALUES(?,?)`)
	if err != nil {
		return err
	}
	defer insert.Close()
	for _, artist := range artists {
		genres, issues, e := resolveArtistGenres(ctx, programs, artist.id, artist.name, artist.labels, artist.evidence...)
		if e != nil {
			return e
		}
		for _, genre := range genres {
			if _, err = insert.ExecContext(ctx, artist.id, genre); err != nil {
				return err
			}
		}
		if len(issues) > 0 {
			raw, e := json.Marshal(issues)
			if e != nil {
				return e
			}
			if _, err = tx.ExecContext(ctx, `INSERT INTO artist_genre_issues(artist_id,issues_json) VALUES(?,?)`, artist.id, string(raw)); err != nil {
				return err
			}
		}
	}
	return nil
}

type genreArtist struct {
	id       int64
	name     string
	labels   []string
	evidence []filters.Value
}

func loadArtistGenreInputs(ctx context.Context, tx *sql.Tx) ([]genreArtist, error) {
	rows, err := tx.QueryContext(ctx, `SELECT a.id,a.name FROM artists_with_name a ORDER BY a.id`)
	if err != nil {
		return nil, err
	}
	artists := []genreArtist{}
	indices := map[int64]int{}
	for rows.Next() {
		var a genreArtist
		if err = rows.Scan(&a.id, &a.name); err != nil {
			rows.Close()
			return nil, err
		}
		indices[a.id] = len(artists)
		artists = append(artists, a)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = tx.QueryContext(ctx, `SELECT DISTINCT a.artist_id,g.record_id,g.position,g.value,r.source_id,r.file_hash,g.field FROM source_genres g JOIN source_records r ON r.id=g.record_id JOIN artist_aliases a ON a.alias=json_extract(r.payload,'$.fields.Artist') WHERE json_type(r.payload,'$.fields.Artist')='text' AND NOT EXISTS(SELECT 1 FROM artist_aliases other WHERE other.alias=a.alias AND other.artist_id<>a.artist_id) ORDER BY a.artist_id,g.record_id,g.position`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id, record, position, sourceID int64
		var label, fileHash, field string
		if err = rows.Scan(&id, &record, &position, &label, &sourceID, &fileHash, &field); err != nil {
			rows.Close()
			return nil, err
		}
		if index, ok := indices[id]; ok {
			artists[index].labels = append(artists[index].labels, label)
			raw, e := json.Marshal(map[string]any{"record_id": record, "source_id": sourceID, "file_hash": fileHash, "field": field, "value": label})
			if e != nil {
				rows.Close()
				return nil, e
			}
			value, e := filters.FromJSON(raw)
			if e != nil {
				rows.Close()
				return nil, e
			}
			artists[index].evidence = append(artists[index].evidence, value)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}

	return artists, nil
}

func previewArtistGenres(ctx context.Context, tx *sql.Tx, before, after RuleConfig) ([]NamingExample, error) {
	old, err := compileNamingDomain(before, true)
	if err != nil {
		return nil, err
	}
	next, err := compileNamingDomain(after, true)
	if err != nil {
		return nil, err
	}
	examples := []NamingExample{}
	if len(old) == 0 && len(next) == 0 {
		return examples, nil
	}
	artists, err := loadArtistGenreInputs(ctx, tx)
	if err != nil {
		return nil, err
	}
	for _, artist := range artists {
		a, _, e := resolveArtistGenres(ctx, old, artist.id, artist.name, artist.labels, artist.evidence...)
		if e != nil {
			return nil, e
		}
		b, _, e := resolveArtistGenres(ctx, next, artist.id, artist.name, artist.labels, artist.evidence...)
		if e != nil {
			return nil, e
		}
		sort.Strings(a)
		sort.Strings(b)
		rawA, _ := json.Marshal(a)
		rawB, _ := json.Marshal(b)
		if string(rawA) != string(rawB) {
			examples = append(examples, NamingExample{Field: artist.name + " genres", Before: string(rawA), After: string(rawB)})
			if len(examples) == 12 {
				break
			}
		}
	}
	return examples, nil
}

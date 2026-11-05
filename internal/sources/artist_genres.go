package sources

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/gabehf/koito/internal/filters"
	"sort"
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
	state, issues, err := resolveCatalogNaming(ctx, programs, "artist", artistID, name, genreEnvironment(genreArtist{id: artistID, name: name, labels: source, evidence: evidence}))
	return state.Genres, issues, err
}

func genreEnvironment(artist genreArtist) filters.Env {
	values := []filters.Value{}
	for _, label := range artist.labels {
		values = append(values, filters.String(label))
	}
	return filters.Env{"catalog.artist_id": filters.Integer(artist.id), "source.genres": filters.Values(values...), "source.genre_records": filters.Values(artist.evidence...)}
}

func addCatalogGenreInputs(ctx context.Context, tx *sql.Tx, items []catalogNameItem) error {
	targets := []int64{}
	if len(items) == 1 {
		if items[0].kind != "artist" {
			return nil
		}
		targets = append(targets, items[0].id)
	}
	artists, err := loadArtistGenreInputs(ctx, tx, targets...)
	if err != nil {
		return err
	}
	byID := map[int64]filters.Env{}
	for _, artist := range artists {
		byID[artist.id] = genreEnvironment(artist)
	}
	for i := range items {
		if items[i].kind == "artist" {
			for key, value := range byID[items[i].id] {
				items[i].metadata[key] = value
			}
		}
	}
	return nil
}

func catalogArtistMetadata(ctx context.Context, tx *sql.Tx, targets ...int64) (map[int64]filters.Env, error) {
	items := []catalogNameItem{}
	if len(targets) > 0 {
		item, _, err := nativeCatalogItem(ctx, tx, "artist", targets[0])
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	} else {
		var err error
		items, err = catalogNameItems(ctx, tx)
		if err != nil {
			return nil, err
		}
	}
	result := map[int64]filters.Env{}
	for _, item := range items {
		if item.kind == "artist" {
			result[item.id] = item.metadata
		}
	}
	return result, nil
}

func resolveGenreArtist(ctx context.Context, programs []namingProgram, artist genreArtist, metadata filters.Env) ([]string, []FilterIssue, error) {
	state, issues, err := resolveCatalogNaming(ctx, programs, "artist", artist.id, artist.name, metadata, genreEnvironment(artist))
	return state.Genres, issues, err
}

func publishArtistGenres(ctx context.Context, tx *sql.Tx, config RuleConfig, targets ...int64) error {
	if len(targets) > 0 {
		var canonical int64
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE((SELECT matched_artist_id FROM music_artist_matches WHERE artist_id=?),?)`, targets[0], targets[0]).Scan(&canonical); err != nil {
			return err
		}
		targets = []int64{canonical}
	}
	programs, err := compileNamingDomain(config, true)
	if err != nil {
		return err
	}
	artists, err := loadArtistGenreInputs(ctx, tx, targets...)
	if err != nil {
		return err
	}
	metadata := map[int64]filters.Env{}
	if len(programs) > 0 {
		metadata, err = catalogArtistMetadata(ctx, tx, targets...)
		if err != nil {
			return err
		}
	}
	for _, q := range []string{`DELETE FROM artist_genres`, `DELETE FROM artist_genre_issues`} {
		args := []any{}
		if len(targets) > 0 {
			q += " WHERE artist_id=?"
			args = append(args, targets[0])
		}
		if _, err = tx.ExecContext(ctx, q, args...); err != nil {
			return err
		}
	}
	insert, err := tx.PrepareContext(ctx, `INSERT INTO artist_genres(artist_id,genre) VALUES(?,?)`)
	if err != nil {
		return err
	}
	defer insert.Close()
	for _, artist := range artists {
		genres, issues, e := resolveGenreArtist(ctx, programs, artist, metadata[artist.id])
		issues = catalogDomainIssues(programs, issues, true)
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

func loadArtistGenreInputs(ctx context.Context, tx *sql.Tx, targets ...int64) ([]genreArtist, error) {
	query := `SELECT a.id,a.name,COALESCE(m.matched_artist_id,a.id) FROM artists_with_name a LEFT JOIN music_artist_matches m ON m.artist_id=a.id`
	args := []any{}
	if len(targets) > 0 {
		query += ` WHERE a.id=?`
		args = append(args, targets[0])
	}
	query += ` ORDER BY a.id`
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	artists := []genreArtist{}
	indices := map[int64][]int{}
	for rows.Next() {
		var a genreArtist
		var canonical int64
		if err = rows.Scan(&a.id, &a.name, &canonical); err != nil {
			rows.Close()
			return nil, err
		}
		indices[canonical] = append(indices[canonical], len(artists))
		artists = append(artists, a)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	query = `SELECT DISTINCT COALESCE(m.matched_artist_id,a.artist_id),g.record_id,g.position,g.value,r.source_id,r.file_hash,g.field FROM source_genres g JOIN source_records r ON r.id=g.record_id JOIN artist_aliases a ON a.alias=json_extract(r.payload,'$.fields.Artist') LEFT JOIN music_artist_matches m ON m.artist_id=a.artist_id WHERE json_type(r.payload,'$.fields.Artist')='text' AND NOT EXISTS(SELECT 1 FROM artist_aliases other LEFT JOIN music_artist_matches om ON om.artist_id=other.artist_id WHERE other.alias=a.alias AND COALESCE(om.matched_artist_id,other.artist_id)<>COALESCE(m.matched_artist_id,a.artist_id))`
	if len(targets) > 0 {
		query += ` AND COALESCE(m.matched_artist_id,a.artist_id)=COALESCE((SELECT matched_artist_id FROM music_artist_matches WHERE artist_id=?),?)`
		args = append(args, targets[0])
	}
	query += ` ORDER BY 1,g.record_id,g.position`
	rows, err = tx.QueryContext(ctx, query, args...)
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
		for _, index := range indices[id] {
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
	metadata, err := catalogArtistMetadata(ctx, tx)
	if err != nil {
		return nil, err
	}
	for _, artist := range artists {
		a, _, e := resolveGenreArtist(ctx, old, artist, metadata[artist.id])
		if e != nil {
			return nil, e
		}
		b, _, e := resolveGenreArtist(ctx, next, artist, metadata[artist.id])
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

package sources

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	_ "modernc.org/sqlite"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const NavidromeDB = "navidrome-sqlite-v1"
const MultiDB = "multi-scrobbler-sqlite-v1"
const NormalizedDB = "normalized-history-sqlite-v1"
const KoitoDB = "koito-sqlite-v1"
const EmptyDB = "empty-sqlite-v1"
const YARGDB = "yarg-sqlite-v1"

type databaseRecord struct {
	Kind    string         `json:"kind"`
	Table   string         `json:"table"`
	Key     string         `json:"key"`
	Fields  map[string]any `json:"fields"`
	Context map[string]any `json:"context,omitempty"`
}

func databaseFormat(parser string) string {
	for _, family := range []string{NavidromeDB, MultiDB, NormalizedDB, KoitoDB, LedgerDB, EmptyDB, YARGDB} {
		if parser == family || strings.HasPrefix(parser, family+":") {
			return family
		}
	}
	return ""
}
func databaseParser(parser string) bool { return databaseFormat(parser) != "" }
func quoteSQL(name string) string       { return `"` + strings.ReplaceAll(name, `"`, `""`) + `"` }

func copySourceFile(ctx context.Context, src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	buffer := make([]byte, 128<<10)
	for {
		if err = ctx.Err(); err != nil {
			break
		}
		var n int
		n, err = in.Read(buffer)
		if n > 0 {
			if _, writeErr := out.Write(buffer[:n]); writeErr != nil {
				err = writeErr
				break
			}
		}
		if err == io.EOF {
			err = nil
			break
		}
		if err != nil {
			break
		}
	}
	closeErr := out.Close()
	if err != nil {
		return err
	}
	return closeErr
}

// SQLite may create shared-memory files while reading a WAL. All access happens
// in an isolated temporary directory, separate from the immutable uploads.
func (s *Store) parseSQLite(ctx context.Context, file, wal File) (part parsedFile, err error) {
	part.file = file
	working, err := os.MkdirTemp(s.Root, ".sqlite-*")
	if err != nil {
		return part, err
	}
	defer os.RemoveAll(working)
	src, _ := s.FilePath(file.Hash)
	dbPath := filepath.Join(working, "input.db")
	if err = copySourceFile(ctx, src, dbPath); err != nil {
		return part, err
	}
	if wal.Hash != "" {
		src, _ = s.FilePath(wal.Hash)
		if err = validateWAL(dbPath, src); err != nil {
			return part, err
		}
		if err = copySourceFile(ctx, src, dbPath+"-wal"); err != nil {
			return part, err
		}
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(dbPath)}
	if filepath.VolumeName(dbPath) != "" && !strings.HasPrefix(u.Path, "/") {
		u.Path = "/" + u.Path
	}
	db, err := sql.Open("sqlite", u.String()+"?mode=ro&_pragma=query_only(ON)&_pragma=trusted_schema(OFF)")
	if err != nil {
		return part, err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	var integrity string
	if err = db.QueryRowContext(ctx, `PRAGMA quick_check`).Scan(&integrity); err != nil {
		return part, fmt.Errorf("Read SQLite snapshot: %w", err)
	}
	if integrity != "ok" {
		return part, errors.New("SQLite snapshot is inconsistent.")
	}
	tables := map[string]string{}
	rows, err := db.QueryContext(ctx, `SELECT name,sql FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		return part, err
	}
	for rows.Next() {
		var name, definition string
		if err = rows.Scan(&name, &definition); err != nil {
			rows.Close()
			return part, err
		}
		tables[name] = definition
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return part, err
	}
	switch {
	case tables["GameRecords"] != "" && tables["PlayerScores"] != "" && tables["Players"] != "":
		part.service, part.parser = "yarg", YARGDB
	case tables["media_file"] != "" && tables["annotation"] != "":
		part.service, part.parser = "navidrome", NavidromeDB
	case tables["plays"] != "" && tables["components"] != "" && tables["play_inputs"] != "":
		part.service, part.parser = "multi-scrobbler", MultiDB
	case tables["raw_play_events"] != "":
		part.service, part.parser = "legacy", NormalizedDB
		var kind string
		if e := db.QueryRowContext(ctx, `SELECT source_type FROM raw_play_events GROUP BY source_type HAVING (SELECT COUNT(DISTINCT source_type) FROM raw_play_events)=1 LIMIT 1`).Scan(&kind); e == nil {
			part.service = normalizedService(kind)
		}
	case tables["observation"] != "" && tables["source_account"] != "":
		part.service, part.parser = "legacy", LedgerDB
	case tables["listens"] != "" && tables["tracks"] != "" && tables["track_aliases"] != "":
		part.service, part.parser = "koito", KoitoDB
	case len(tables) == 0:
		part.service, part.parser = "database", EmptyDB
	default:
		return part, ErrUnsupportedFile
	}
	if err = validateSQLSchema(ctx, db, part.parser, tables); err != nil {
		return part, err
	}
	if wal.Hash != "" {
		part.parser += ":wal:" + wal.Hash
	}
	spool, err := os.CreateTemp(s.Root, ".records-*")
	if err != nil {
		return part, err
	}
	part.recordFile = spool.Name()
	defer func() {
		if closeErr := spool.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			os.Remove(part.recordFile)
		}
	}()
	encoder := json.NewEncoder(spool)
	emit := func(row databaseRecord) error {
		if e := ctx.Err(); e != nil {
			return e
		}
		if e := encoder.Encode(row); e != nil {
			return e
		}
		part.recordCount++
		return nil
	}
	if err = emit(databaseRecord{Kind: "database", Table: "schema", Key: "schema", Fields: map[string]any{"tables": tables, "wal_hash": wal.Hash}}); err != nil {
		return part, err
	}
	if part.service == "navidrome" {
		part.snapshots = append(part.snapshots, parsedSnapshot{Group: "navidrome-catalog", Key: "catalog", Kind: "catalog", Name: "Navidrome", Metadata: json.RawMessage(`{"service":"navidrome"}`)})
	}
	lookup := map[string]map[string]map[string]any{}
	var games *yargLookup
	if part.service == "yarg" {
		games, err = loadYARGContext(ctx, db)
		if err != nil {
			return part, err
		}
	}
	if part.service == "koito" && tables["evidence_observations"] == "" {
		lookup["native_tracks"], err = nativeKoitoTracks(ctx, db)
		if err != nil {
			return part, err
		}
	}
	for _, table := range []string{"media_file", "components", "plays"} {
		if tables[table] == "" {
			continue
		}
		if table == "plays" && part.service != "multi-scrobbler" {
			continue
		}
		lookup[table] = map[string]map[string]any{}
		err = readSQLTable(ctx, db, table, func(fields map[string]any, key string) error {
			lookup[table][valueText(fields["id"])] = fields
			return nil
		})
		if err != nil {
			return part, err
		}
	}
	for _, table := range sortedTableNames(tables) {
		if sqliteStorageTable(table, tables[table], tables) {
			continue
		}
		if part.service == "koito" && !koitoMigrationTable(table) {
			continue
		}
		err = readSQLTable(ctx, db, table, func(fields map[string]any, key string) error {
			row := databaseRecord{Kind: "context", Table: table, Key: key, Fields: fields, Context: map[string]any{}}
			switch part.service {
			case "yarg":
				games.attach(&row)
			case "navidrome":
				if kind := map[string]string{"player": "player", "tag": "tag", "media_file_tags": "tag_link", "album_tags": "tag_link", "album": "catalog_album", "artist": "catalog_artist", "library": "catalog_library", "scrobble_buffer": "queue"}[table]; kind != "" {
					row.Kind = kind
				}
				if table == "media_file" {
					row.Kind = "catalog_track"
					part.snapshots[0].Ordinals = append(part.snapshots[0].Ordinals, part.recordCount)
				}
				if table == "scrobbles" {
					row.Kind = "play"
				}
				if table == "annotation" {
					row.Kind = "annotation"
					if valueText(fields["item_type"]) == "media_file" {
						row.Kind = "library_track"
					}
				}
				id := valueText(fields["media_file_id"])
				if table == "annotation" {
					id = valueText(fields["item_id"])
				}
				if media := lookup["media_file"][id]; media != nil {
					row.Context["media"] = media
				}
			case "multi-scrobbler":
				if table == "components" {
					row.Kind = "component"
				}
				if table == "plays" || table == "plays_historical" {
					component := lookup["components"][valueText(fields["componentId"])]
					row.Context["component"] = component
					if valueText(component["mode"]) == "source" && fields["parentId"] == nil {
						row.Kind = "play"
					} else {
						row.Kind = "delivery"
					}
					if parent := lookup["plays"][valueText(fields["parentId"])]; parent != nil {
						row.Context["parent"] = multiRootReference(lookup["plays"], parent)
					} else if fields["parentId"] != nil {
						row.Context["parent"] = map[string]any{"id": fields["parentId"], "issue": "missing parent record"}
					}
				} else if table == "play_inputs" || table == "play_events" || table == "play_queue_states" {
					row.Kind = map[string]string{"play_inputs": "source_input", "play_events": "delivery_event", "play_queue_states": "queue"}[table]
					if play := lookup["plays"][valueText(fields["playId"])]; play != nil {
						row.Context["parent"] = multiRootReference(lookup["plays"], play)
					}
				}
			case "legacy", "spotify", "lastfm", "itunes":
				if table == "raw_play_events" || table == "observation" {
					row.Kind = "legacy_record"
				}
				if table == "aggregate_evidence" {
					row.Kind = "aggregate"
				}
			case "koito":
				if table == "evidence_observations" {
					row.Kind = "legacy_record"
				}
				if table == "listens" {
					row.Kind = "legacy_view"
					if tables["evidence_observations"] == "" {
						row.Kind = "native_play"
						row.Context["track"] = lookup["native_tracks"][valueText(fields["track_id"])]
					}
				}
			}
			return emit(row)
		})
		if err != nil {
			return part, fmt.Errorf("Read %s: %w", table, err)
		}
	}
	return part, nil
}

func multiRootReference(plays map[string]map[string]any, play map[string]any) map[string]any {
	result := map[string]any{"id": play["id"], "uid": play["uid"], "parentId": play["parentId"]}
	seen := map[string]bool{}
	for play != nil {
		id := valueText(play["id"])
		if seen[id] {
			result["issue"] = "cyclic parent references"
			return result
		}
		seen[id] = true
		if play["parentId"] == nil {
			result["root_uid"] = play["uid"]
			return result
		}
		play = plays[valueText(play["parentId"])]
	}
	result["issue"] = "missing parent record"
	return result
}

func valueText(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case json.Number:
		return string(v)
	}
	return ""
}
func sortedTableNames(tables map[string]string) []string {
	names := []string{}
	for name := range tables {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
func sqliteStorageTable(name, definition string, tables map[string]string) bool {
	if strings.Contains(strings.ToUpper(definition), "CREATE VIRTUAL TABLE") || strings.HasPrefix(name, "sqlite_") {
		return true
	}
	for base, sql := range tables {
		if !strings.Contains(strings.ToUpper(sql), "CREATE VIRTUAL TABLE") {
			continue
		}
		for _, suffix := range []string{"_data", "_idx", "_content", "_docsize", "_config", "_segments", "_segdir", "_stat", "_node", "_rowid", "_parent"} {
			if name == base+suffix {
				return true
			}
		}
	}
	return false
}
func koitoMigrationTable(name string) bool {
	switch name {
	case "listens", "listen_metadata", "tracks", "artists", "releases", "artist_tracks", "artist_releases", "track_aliases", "artist_aliases", "release_aliases", "evidence_observations", "source_observation_provenance", "migration_observation_manifest", "source_observation_status":
		return true
	}
	return false
}

func readSQLTable(ctx context.Context, db *sql.DB, table string, consume func(map[string]any, string) error) error {
	info, err := db.QueryContext(ctx, "PRAGMA table_info("+quoteSQL(table)+")")
	if err != nil {
		return err
	}
	type pk struct {
		position int
		name     string
	}
	keys := []pk{}
	columns := []string{}
	for info.Next() {
		var cid, notnull, primary int
		var name, kind string
		var def any
		if err = info.Scan(&cid, &name, &kind, &notnull, &def, &primary); err != nil {
			info.Close()
			return err
		}
		columns = append(columns, name)
		if primary > 0 {
			keys = append(keys, pk{primary, name})
		}
	}
	err = info.Err()
	info.Close()
	if err != nil {
		return err
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].position < keys[j].position })
	order := []string{}
	for _, k := range keys {
		order = append(order, quoteSQL(k.name))
	}
	if len(order) == 0 {
		for _, c := range columns {
			order = append(order, quoteSQL(c))
		}
	}
	rows, err := db.QueryContext(ctx, "SELECT * FROM "+quoteSQL(table)+" ORDER BY "+strings.Join(order, ","))
	if err != nil {
		return err
	}
	defer rows.Close()
	occurrences := map[string]int{}
	for rows.Next() {
		values := make([]any, len(columns))
		dest := make([]any, len(columns))
		for i := range values {
			dest[i] = &values[i]
		}
		if err = rows.Scan(dest...); err != nil {
			return err
		}
		fields := map[string]any{}
		for i, column := range columns {
			value := values[i]
			if blob, ok := value.([]byte); ok {
				value = map[string]any{"$sqlite_blob": blob}
			}
			fields[column] = value
		}
		parts := []any{}
		for _, k := range keys {
			parts = append(parts, fields[k.name])
		}
		raw, _ := json.Marshal(parts)
		key := string(raw)
		if len(keys) == 0 {
			raw, _ = json.Marshal(fields)
			base := digest(raw)
			occurrences[base]++
			key = base + ":" + strconv.Itoa(occurrences[base])
		}
		if err = consume(fields, key); err != nil {
			return err
		}
	}
	return rows.Err()
}

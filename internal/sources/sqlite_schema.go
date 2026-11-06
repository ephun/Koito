package sources

import (
	"context"
	"database/sql"
	"fmt"
)

func requireSQLColumns(ctx context.Context, db *sql.DB, table string, required ...string) error {
	rows, err := db.QueryContext(ctx, "PRAGMA table_info("+quoteSQL(table)+")")
	if err != nil {
		return err
	}
	columns := map[string]bool{}
	for rows.Next() {
		var cid, notnull, primary int
		var name, kind string
		var def any
		if err = rows.Scan(&cid, &name, &kind, &notnull, &def, &primary); err != nil {
			rows.Close()
			return err
		}
		columns[name] = true
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, name := range required {
		if !columns[name] {
			return fmt.Errorf("Unsupported database schema: missing %s.%s.", table, name)
		}
	}
	return nil
}

func validateSQLSchema(ctx context.Context, db *sql.DB, parser string, tables map[string]string) error {
	required := map[string][]string{}
	switch parser {
	case YARGDB:
		required["GameRecords"] = []string{"Id", "Date", "SongChecksum", "GameVersion", "SongName", "SongArtist", "SongSpeed"}
		required["PlayerScores"] = []string{"Id", "GameRecordId", "PlayerId", "Score", "NotesHit", "NotesMissed"}
		required["Players"] = []string{"Id", "Name"}
	case NavidromeDB:
		required["media_file"] = []string{"id", "title", "artist", "album"}
		required["annotation"] = []string{"user_id", "item_id", "item_type", "play_count", "play_date"}
		if tables["scrobbles"] != "" {
			required["scrobbles"] = []string{"id", "media_file_id", "user_id", "submission_time"}
		}
	case MultiDB:
		required["components"] = []string{"id", "mode", "type", "name"}
		required["plays"] = []string{"id", "uid", "componentId", "parentId", "play", "state"}
		required["play_inputs"] = []string{"id", "playId", "data", "play"}
		if tables["plays_historical"] != "" {
			required["plays_historical"] = []string{"id", "uid", "componentId", "play"}
		}
		for _, table := range []string{"play_events", "play_queue_states"} {
			if tables[table] != "" {
				required[table] = []string{"id", "playId"}
			}
		}
	case NormalizedDB:
		required["raw_play_events"] = []string{"source_type", "played_at", "title_raw", "artist_raw", "payload_json"}
	case LedgerDB:
		required["observation"] = []string{"id", "source_service", "payload", "title_raw", "artist_raw"}
	case KoitoDB:
		required["listens"] = []string{"track_id", "listened_at", "user_id", "client"}
		required["tracks"] = []string{"id", "release_id"}
		required["track_aliases"] = []string{"track_id", "alias", "is_primary"}
		if tables["evidence_observations"] != "" {
			required["evidence_observations"] = []string{"id", "payload_json"}
		} else {
			required["artist_tracks"] = []string{"artist_id", "track_id", "is_primary"}
			required["artist_aliases"] = []string{"artist_id", "alias", "is_primary"}
			required["release_aliases"] = []string{"release_id", "alias", "is_primary"}
		}
	}
	for _, table := range sortedRequiredTables(required) {
		if err := requireSQLColumns(ctx, db, table, required[table]...); err != nil {
			return err
		}
	}
	return nil
}

func sortedRequiredTables(required map[string][]string) []string {
	tables := map[string]string{}
	for name := range required {
		tables[name] = ""
	}
	return sortedTableNames(tables)
}

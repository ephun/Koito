package sources

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
)

// Source evidence is rebuilt independently of personal listening decisions.
// Artist assignments and manual corrections consume this evidence separately.
func refreshSourceGenres(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `SELECT id,parser,payload FROM source_records WHERE kind='library_track' ORDER BY id`)
	if err != nil {
		return err
	}
	type entry struct {
		id     int64
		genres []SourceGenre
	}
	entries := []entry{}
	for rows.Next() {
		var id int64
		var parser, payload string
		if err = rows.Scan(&id, &parser, &payload); err != nil {
			rows.Close()
			return err
		}
		genres, e := SourceGenres(parser, []byte(payload))
		if e != nil {
			rows.Close()
			return e
		}
		if len(genres) > 0 {
			entries = append(entries, entry{id, genres})
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM source_genres`); err != nil {
		return err
	}
	statement, err := tx.PrepareContext(ctx, `INSERT INTO source_genres(record_id,position,field,value) VALUES(?,?,?,?)`)
	if err != nil {
		return err
	}
	defer statement.Close()
	for _, entry := range entries {
		for position, genre := range entry.genres {
			if _, err = statement.ExecContext(ctx, entry.id, position, genre.Field, genre.Value); err != nil {
				return err
			}
		}
	}
	return nil
}

// SourceGenre retains the supplied label and its location. A comma in a label
// does not establish that the source intended multiple genres.
type SourceGenre struct {
	Value string `json:"value"`
	Field string `json:"field"`
}

// SourceGenres reads supported genre evidence independently of whether the
// record contributes a play or a cumulative count. Unknown shapes stay in the
// preserved record rather than being converted into invented labels.
func SourceGenres(parser string, payload []byte) ([]SourceGenre, error) {
	result := []SourceGenre{}
	if parser != ITunesXML && parser != ITunesJSON {
		return result, nil
	}
	var record struct {
		Kind   string                     `json:"kind"`
		Fields map[string]json.RawMessage `json:"fields"`
	}
	if err := json.Unmarshal(payload, &record); err != nil {
		return nil, err
	}
	if record.Kind != "library_track" {
		return result, nil
	}
	raw, ok := record.Fields["Genre"]
	if !ok {
		return result, nil
	}
	var read func(json.RawMessage, string)
	read = func(raw json.RawMessage, path string) {
		var text string
		if json.Unmarshal(raw, &text) == nil && !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			result = append(result, SourceGenre{text, path})
			return
		}
		var repeated struct {
			Values []json.RawMessage `json:"$plist_repeated"`
		}
		if json.Unmarshal(raw, &repeated) == nil {
			for index, value := range repeated.Values {
				read(value, path+"/$plist_repeated/"+strconv.Itoa(index))
			}
		}
	}
	read(raw, "raw:/fields/Genre")
	return result, nil
}

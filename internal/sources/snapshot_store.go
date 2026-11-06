package sources

import (
	"context"
	"database/sql"
	"encoding/json"
)

type RecordSnapshot struct {
	ID       int64           `json:"id"`
	Name     string          `json:"name"`
	Kind     string          `json:"kind"`
	Position int             `json:"position"`
	Metadata json.RawMessage `json:"metadata"`
}

func recordSnapshots(ctx context.Context, reader sqlReader, id int64) ([]RecordSnapshot, error) {
	rows, err := reader.QueryContext(ctx, `SELECT s.id,s.name,s.kind,i.position,s.metadata FROM source_snapshots s JOIN source_snapshot_items i ON i.snapshot_id=s.id WHERE i.record_id=? ORDER BY s.id,i.position`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RecordSnapshot{}
	for rows.Next() {
		var item RecordSnapshot
		var metadata string
		if err = rows.Scan(&item.ID, &item.Name, &item.Kind, &item.Position, &metadata); err != nil {
			return nil, err
		}
		item.Metadata = json.RawMessage(metadata)
		out = append(out, item)
	}
	return out, rows.Err()
}

func storeSnapshots(ctx context.Context, tx *sql.Tx, sourceID int64, part parsedFile) error {
	for _, snapshot := range part.snapshots {
		id, err := insertID(ctx, tx, `INSERT INTO source_snapshots(source_id,file_hash,parser,set_key,group_key,kind,name,metadata) VALUES(?,?,?,?,?,?,?,?)`, sourceID, part.file.Hash, part.parser, snapshot.Key, snapshot.Group, snapshot.Kind, snapshot.Name, string(snapshot.Metadata))
		if err != nil {
			return err
		}
		for pos, ordinal := range snapshot.Ordinals {
			_, err = tx.ExecContext(ctx, `INSERT INTO source_snapshot_items(snapshot_id,position,record_id) SELECT ?,?,id FROM source_records WHERE source_id=? AND file_hash=? AND parser=? AND ordinal=?`, id, pos, sourceID, part.file.Hash, part.parser, ordinal)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

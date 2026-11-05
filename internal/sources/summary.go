package sources

import (
	"context"
	"database/sql"
	"time"
)

type SourceSummary struct {
	ID      int64  `json:"id"`
	Account string `json:"account"`
	Service string `json:"service"`
	Records int    `json:"records"`
	Plays   int    `json:"plays"`
	Undated int64  `json:"undated"`
	First   string `json:"first"`
	Last    string `json:"last"`
}

func (s *Store) Summary(ctx context.Context) ([]SourceSummary, error) {
	// Keep the summary on one snapshot. Read the compact record/source index
	// once instead of repeatedly fetching large payload pages for contributors.
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT id,account,service FROM source_accounts ORDER BY id`)
	if err != nil {
		return nil, err
	}
	out := []SourceSummary{}
	positions := map[int64]int{}
	for rows.Next() {
		var v SourceSummary
		if err = rows.Scan(&v.ID, &v.Account, &v.Service); err != nil {
			rows.Close()
			return nil, err
		}
		positions[v.ID] = len(out)
		out = append(out, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	recordSources := map[int64]int64{}
	rows, err = tx.QueryContext(ctx, `SELECT id,source_id FROM source_records INDEXED BY source_records_source`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var record, source int64
		if err = rows.Scan(&record, &source); err != nil {
			rows.Close()
			return nil, err
		}
		recordSources[record] = source
		out[positions[source]].Records++
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	// Interpretations support projections created before contributor links.
	// Retain that fallback, including partially populated links, without sorting
	// a union of every source record or counting duplicate support twice.
	rows, err = tx.QueryContext(ctx, `SELECT i.record_id,p.play_key,p.listened_at FROM source_plays p
 JOIN source_play_records i ON i.play_key=p.play_key WHERE p.policy=?
 UNION ALL
 SELECT i.record_id,p.play_key,p.listened_at FROM source_plays p
 CROSS JOIN source_interpretations i ON i.policy=p.policy AND i.play_key=p.play_key
 WHERE p.policy=? AND NOT EXISTS(SELECT 1 FROM source_play_records s WHERE s.play_key=i.play_key AND s.record_id=i.record_id)`, Policy, Policy)
	if err != nil {
		return nil, err
	}
	seen := map[int64]map[string]bool{}
	first, last := map[int64]int64{}, map[int64]int64{}
	for rows.Next() {
		var record, unix int64
		var key string
		if err = rows.Scan(&record, &key, &unix); err != nil {
			rows.Close()
			return nil, err
		}
		source := recordSources[record]
		if seen[source] == nil {
			seen[source] = map[string]bool{}
		}
		if seen[source][key] {
			continue
		}
		seen[source][key] = true
		out[positions[source]].Plays++
		if prior, ok := first[source]; !ok || unix < prior {
			first[source] = unix
		}
		if prior, ok := last[source]; !ok || unix > prior {
			last[source] = unix
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = tx.QueryContext(ctx, `SELECT r.source_id,SUM(g.undated_count) FROM source_aggregates g JOIN source_records r ON r.id=g.representative_record GROUP BY r.source_id`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var source, undated int64
		if err = rows.Scan(&source, &undated); err != nil {
			rows.Close()
			return nil, err
		}
		out[positions[source]].Undated = undated
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	for i := range out {
		v := &out[i]
		if v.Plays > 0 {
			v.First = time.Unix(first[v.ID], 0).UTC().Format("2006-01-02 15:04:05")
			v.Last = time.Unix(last[v.ID], 0).UTC().Format("2006-01-02 15:04:05")
		}
	}
	return out, tx.Commit()
}

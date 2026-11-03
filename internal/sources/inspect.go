package sources

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

type InspectedRecord struct {
	matchingRecord
	Reason  string `json:"reason"`
	PlayKey string `json:"play_key"`
}

type RecordInspection struct {
	Record       InspectedRecord   `json:"record"`
	Contributors []InspectedRecord `json:"contributors"`
	Candidates   []InspectedRecord `json:"candidates"`
}

const inspectedColumns = `r.id,r.source_id,a.service,r.file_hash,r.ordinal,r.parser,r.payload,a.user_id,COALESCE(i.reason,'not projected'),COALESCE(i.play_key,'')`
const inspectedTables = ` FROM source_records r JOIN source_accounts a ON a.id=r.source_id LEFT JOIN source_interpretations i ON i.record_id=r.id AND i.policy='filters-v1' `

func inspectedRecords(ctx context.Context, reader sqlReader, config RuleConfig, where string, args ...any) ([]InspectedRecord, error) {
	rows, err := reader.QueryContext(ctx, `SELECT `+inspectedColumns+inspectedTables+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []InspectedRecord{}
	for rows.Next() {
		var r InspectedRecord
		if err = rows.Scan(&r.ID, &r.SourceID, &r.Service, &r.FileHash, &r.Ordinal, &r.Parser, &r.Payload, &r.UserID, &r.Reason, &r.PlayKey); err != nil {
			return nil, err
		}
		interpretMatchingRecord(&r.matchingRecord, config)
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) Inspect(ctx context.Context, id int64) (RecordInspection, error) {
	var result RecordInspection
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	version, err := readRuleVersion(ctx, tx, 0)
	if err != nil {
		return result, err
	}
	rows, err := inspectedRecords(ctx, tx, version.Config, `WHERE r.id=?`, id)
	if err != nil {
		return result, err
	}
	if len(rows) == 0 {
		return result, fmt.Errorf("Unknown record.")
	}
	result.Record = rows[0]
	result.Contributors = []InspectedRecord{}
	result.Candidates = []InspectedRecord{}
	if result.Record.PlayKey != "" {
		result.Contributors, err = inspectedRecords(ctx, tx, version.Config, `WHERE i.play_key=? ORDER BY r.id`, result.Record.PlayKey)
		if err != nil {
			return result, err
		}
	}
	result.Candidates, err = inspectedRecords(ctx, tx, version.Config, `WHERE r.id IN (SELECT candidate_id FROM source_match_candidates WHERE record_id=?) ORDER BY r.id`, id)
	return result, err
}

type PlayListItem struct {
	Key      string `json:"key"`
	RecordID int64  `json:"record_id"`
	Title    string `json:"title"`
	Artist   string `json:"artist"`
	Album    string `json:"album"`
	Time     string `json:"time"`
	Records  int    `json:"records"`
	Reason   string `json:"reason"`
}

func (s *Store) Plays(ctx context.Context, after int64, query string, ambiguous bool) ([]PlayListItem, error) {
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	version, err := readRuleVersion(ctx, tx, 0)
	if err != nil {
		return nil, err
	}
	where := `WHERE r.id>? AND EXISTS(SELECT 1 FROM source_plays p WHERE p.policy='filters-v1' AND p.representative_record=r.id)`
	args := []any{after}
	if ambiguous {
		where += ` AND EXISTS(SELECT 1 FROM source_match_candidates c JOIN source_interpretations other ON other.record_id=c.record_id AND other.policy='filters-v1' WHERE other.play_key=i.play_key)`
	}
	if query != "" {
		where += ` AND (CAST(r.id AS TEXT)=? OR r.payload LIKE ? ESCAPE '\')`
		args = append(args, query, "%"+strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(query)+"%")
	}
	rows, err := inspectedRecords(ctx, tx, version.Config, where+` ORDER BY r.id LIMIT 100`, args...)
	if err != nil {
		return nil, err
	}
	out := []PlayListItem{}
	for _, r := range rows {
		var n int
		if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM source_interpretations WHERE policy=? AND play_key=?`, Policy, r.PlayKey).Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, PlayListItem{Key: r.PlayKey, RecordID: r.ID, Title: r.Values.Title, Artist: r.Values.Artist, Album: r.Values.Album, Time: r.Start.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"), Records: n, Reason: r.Reason})
	}
	return out, nil
}

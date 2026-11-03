package sources

import "context"

type SourceSummary struct {
	ID      int64  `json:"id"`
	Account string `json:"account"`
	Service string `json:"service"`
	Records int    `json:"records"`
	Plays   int    `json:"plays"`
	First   string `json:"first"`
	Last    string `json:"last"`
}

func (s *Store) Summary(ctx context.Context) ([]SourceSummary, error) {
	rows, err := s.DB.QueryContext(ctx, `WITH contributions AS (
 SELECT DISTINCT r.source_id,p.play_key,p.listened_at FROM source_records r
 JOIN source_interpretations i ON i.record_id=r.id AND i.policy=? AND i.play_key<>''
 JOIN source_plays p ON p.play_key=i.play_key AND p.policy=i.policy
 ) SELECT a.id,a.account,a.service,
 (SELECT COUNT(*) FROM source_records r WHERE r.source_id=a.id),
 (SELECT COUNT(*) FROM contributions c WHERE c.source_id=a.id),
 COALESCE((SELECT datetime(MIN(c.listened_at),'unixepoch') FROM contributions c WHERE c.source_id=a.id),''),
 COALESCE((SELECT datetime(MAX(c.listened_at),'unixepoch') FROM contributions c WHERE c.source_id=a.id),'')
 FROM source_accounts a ORDER BY a.id`, Policy)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SourceSummary{}
	for rows.Next() {
		var v SourceSummary
		if err = rows.Scan(&v.ID, &v.Account, &v.Service, &v.Records, &v.Plays, &v.First, &v.Last); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

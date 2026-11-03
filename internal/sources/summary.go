package sources

import "context"

type SourceSummary struct {
	ID      int64  `json:"id"`
	Account string `json:"account"`
	Records int    `json:"records"`
	Plays   int    `json:"plays"`
	First   string `json:"first"`
	Last    string `json:"last"`
}

func (s *Store) Summary(ctx context.Context) ([]SourceSummary, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT a.id,a.account,(SELECT COUNT(*) FROM source_records r WHERE r.source_id=a.id),(SELECT COUNT(*) FROM source_plays p WHERE p.source_id=a.id AND p.policy=?),COALESCE((SELECT datetime(MIN(p.listened_at),'unixepoch') FROM source_plays p WHERE p.source_id=a.id AND p.policy=?),''),COALESCE((SELECT datetime(MAX(p.listened_at),'unixepoch') FROM source_plays p WHERE p.source_id=a.id AND p.policy=?),'') FROM source_accounts a ORDER BY a.id`, Policy, Policy, Policy)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SourceSummary{}
	for rows.Next() {
		var v SourceSummary
		if err = rows.Scan(&v.ID, &v.Account, &v.Records, &v.Plays, &v.First, &v.Last); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

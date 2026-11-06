package sources

import (
	"context"
	"database/sql"
	"strconv"
)

// Publication executes the same few inserts for every record and play. Reuse
// their compiled statements within this transaction, including rollback paths.
type publicationStatements struct {
	tx         *sql.Tx
	statements map[string]*sql.Stmt
}

func (p *publicationStatements) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	statement := p.statements[query]
	if statement == nil {
		var err error
		statement, err = p.tx.PrepareContext(ctx, query)
		if err != nil {
			return nil, err
		}
		if p.statements == nil {
			p.statements = map[string]*sql.Stmt{}
		}
		p.statements[query] = statement
	}
	return statement.ExecContext(ctx, args...)
}

func (p *publicationStatements) Close() {
	for _, statement := range p.statements {
		statement.Close()
	}
}

// SQLite's small default page cache repeatedly evicts the indexes used by a
// large publication. Increase only this writer's cache and restore its setting
// before returning the connection to the pool. This does not change durability.
func publicationCache(ctx context.Context, tx *sql.Tx) (func(), error) {
	var previous, pageSize int64
	if err := tx.QueryRowContext(ctx, `PRAGMA cache_size`).Scan(&previous); err != nil {
		return nil, err
	}
	if err := tx.QueryRowContext(ctx, `PRAGMA page_size`).Scan(&pageSize); err != nil {
		return nil, err
	}
	const kibibytes = 65536
	if previous <= -kibibytes || previous > 0 && previous*pageSize >= kibibytes*1024 {
		return func() {}, nil
	}
	if _, err := tx.ExecContext(ctx, `PRAGMA cache_size=-65536`); err != nil {
		return nil, err
	}
	restored := false
	return func() {
		if restored {
			return
		}
		_, err := tx.ExecContext(ctx, `PRAGMA cache_size=`+strconv.FormatInt(previous, 10))
		restored = err == nil
	}, nil
}

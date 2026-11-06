package engine_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gabehf/koito/internal/cfg"
	"github.com/stretchr/testify/require"
)

func waitForLivePublication(t *testing.T) {
	t.Helper()
	require.Eventually(t, func() bool {
		files, err := os.ReadDir(filepath.Join(cfg.ConfigDir(), "sources", "live", "pending"))
		if errors.Is(err, os.ErrNotExist) {
			return true
		}
		if err != nil {
			return false
		}
		for _, file := range files {
			if strings.HasSuffix(file.Name(), ".json") {
				return false
			}
		}
		return true
	}, 20*time.Second, 20*time.Millisecond, "Accepted live submissions did not finish publication")
}

func resetMusicFixture(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	connection, err := store.SourceDatabase().Conn(ctx)
	require.NoError(t, err)
	defer connection.Close()
	_, err = connection.ExecContext(ctx, `PRAGMA foreign_keys=OFF`)
	require.NoError(t, err)
	defer connection.ExecContext(ctx, `PRAGMA foreign_keys=ON`)
	tx, err := connection.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT name,sql FROM sqlite_master WHERE type='trigger'`)
	require.NoError(t, err)
	type trigger struct{ name, sql string }
	triggers := []trigger{}
	for rows.Next() {
		var item trigger
		require.NoError(t, rows.Scan(&item.name, &item.sql))
		triggers = append(triggers, item)
	}
	require.NoError(t, rows.Err())
	rows.Close()
	quote := func(name string) string { return `"` + strings.ReplaceAll(name, `"`, `""`) + `"` }
	for _, item := range triggers {
		_, err = tx.ExecContext(ctx, `DROP TRIGGER `+quote(item.name))
		require.NoError(t, err)
	}
	rows, err = tx.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'`)
	require.NoError(t, err)
	tables := []string{}
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		tables = append(tables, name)
	}
	require.NoError(t, rows.Err())
	rows.Close()
	keep := map[string]bool{"users": true, "sessions": true, "api_keys": true, "goose_db_version": true, "filter_versions": true, "filter_state": true}
	for _, name := range tables {
		if keep[name] || strings.HasSuffix(name, "rollback_guard") {
			continue
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM `+quote(name))
		require.NoError(t, err, name)
	}
	for _, item := range triggers {
		_, err = tx.ExecContext(ctx, item.sql)
		require.NoError(t, err)
	}
	require.NoError(t, tx.Commit())
}

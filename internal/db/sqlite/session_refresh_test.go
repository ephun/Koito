package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSessionRefreshPreservesConcurrentWALSnapshot(t *testing.T) {
	database, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "sessions.db")+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(1000)")
	require.NoError(t, err)
	defer database.Close()
	database.SetMaxOpenConns(3)
	_, err = database.Exec(`CREATE TABLE users(id INTEGER PRIMARY KEY,username TEXT); INSERT INTO users VALUES(1,'before');
CREATE TABLE sessions(id TEXT PRIMARY KEY,user_id INTEGER,created_at INTEGER,expires_at INTEGER,persistent INTEGER)`)
	require.NoError(t, err)
	store := &Sqlite{db: database}
	ctx := context.Background()
	expires := time.Now().Add(30 * 24 * time.Hour).Truncate(time.Second)
	session, err := store.SaveSession(ctx, 1, expires, true)
	require.NoError(t, err)
	tx, err := database.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer tx.Rollback()
	var name string
	require.NoError(t, tx.QueryRow(`SELECT username FROM users WHERE id=1`).Scan(&name))
	// Another connection authenticates during the view's read transaction.
	// A gratuitous expiry update here would make the later write fail with 517.
	require.NoError(t, store.RefreshSession(ctx, session.ID, expires.Add(time.Minute)))
	_, err = tx.Exec(`UPDATE users SET username='after' WHERE id=1`)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	var saved int64
	require.NoError(t, database.QueryRow(`SELECT expires_at FROM sessions WHERE id=?`, session.ID.String()).Scan(&saved))
	require.Equal(t, expires.Unix(), saved)
	// A later visit renews the session. An explicit shortening also still works.
	require.NoError(t, store.RefreshSession(ctx, session.ID, expires.Add(25*time.Hour)))
	require.NoError(t, database.QueryRow(`SELECT expires_at FROM sessions WHERE id=?`, session.ID.String()).Scan(&saved))
	require.Equal(t, expires.Add(25*time.Hour).Unix(), saved)
	require.NoError(t, store.RefreshSession(ctx, session.ID, expires.Add(-time.Hour)))
	require.NoError(t, database.QueryRow(`SELECT expires_at FROM sessions WHERE id=?`, session.ID.String()).Scan(&saved))
	require.Equal(t, expires.Add(-time.Hour).Unix(), saved)
}

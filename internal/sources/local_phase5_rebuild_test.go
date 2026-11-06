package sources

import (
	"context"
	"database/sql"
	"os"
	"runtime/pprof"
	"testing"
	"time"

	migrations "github.com/gabehf/koito/db/migrations_sqlite"
	"github.com/gabehf/koito/internal/logger"
	"github.com/pressly/goose/v3"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

// Opt-in profiling uses a disposable database copy. The transaction rolls back
// after validating publication, leaving the copied originals unchanged too.
func TestLocalPhase5Rebuild(t *testing.T) {
	path := os.Getenv("KOITO_TEST_PHASE5_DB")
	if path == "" {
		t.Skip("set KOITO_TEST_PHASE5_DB to a disposable database copy")
	}
	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(ON)")
	require.NoError(t, err)
	defer db.Close()
	db.SetMaxOpenConns(1)
	goose.SetBaseFS(migrations.Files)
	require.NoError(t, goose.SetDialect("sqlite3"))
	require.NoError(t, goose.Up(db, "."))
	log := zerolog.New(os.Stdout).Level(zerolog.DebugLevel)
	ctx, cancel := context.WithTimeout(logger.NewContext(&log), 30*time.Minute)
	defer cancel()

	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer tx.Rollback()
	started := time.Now()
	version, err := readRuleVersion(ctx, tx, 0)
	require.NoError(t, err)
	records, err := loadMatchingRecords(ctx, tx, version.Config)
	require.NoError(t, err)
	t.Logf("Loaded %d records in %s", len(records), time.Since(started).Round(time.Millisecond))
	decisions, err := loadDecisions(ctx, tx)
	require.NoError(t, err)
	plan, err := calculateView(ctx, tx, records, version.Config, decisions)
	require.NoError(t, err)
	t.Logf("Matched %d plays after %s", len(plan.Plays), time.Since(started).Round(time.Millisecond))
	if path := os.Getenv("KOITO_TEST_PHASE5_PROFILE"); path != "" {
		file, err := os.Create(path)
		require.NoError(t, err)
		delay, _ := time.ParseDuration(os.Getenv("KOITO_TEST_PHASE5_PROFILE_DELAY"))
		done, stopped := make(chan struct{}), make(chan struct{})
		go func() {
			defer close(stopped)
			defer file.Close()
			select {
			case <-time.After(delay):
			case <-done:
				return
			}
			if err := pprof.StartCPUProfile(file); err != nil {
				t.Error(err)
				return
			}
			select {
			case <-time.After(45 * time.Second):
			case <-done:
			}
			pprof.StopCPUProfile()
			t.Log("CPU profile ready")
		}()
		defer func() { close(done); <-stopped }()
	}

	result, err := publishPlan(ctx, tx, plan, version.ID)
	require.NoError(t, err)
	t.Logf("Published %d plays after %s", result.Plays, time.Since(started).Round(time.Millisecond))
}

package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestNewSQLite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.db")

	database, err := NewSQLite(path)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, database.Close())
	})

	require.Equal(t, driverSQLite, database.Driver())
	require.NotNil(t, database.SQLDB())
	require.Equal(t, sqliteMaxOpenConns, database.SQLDB().Stats().MaxOpenConnections)
}

func TestNewPostgresWithMock(t *testing.T) {
	originalOpen := openSQL
	t.Cleanup(func() {
		openSQL = originalOpen
	})

	mockDB, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	require.NoError(t, err)

	mock.ExpectPing()
	mock.ExpectClose()

	openSQL = func(driverName, dsn string) (*sql.DB, error) {
		require.Equal(t, driverPostgres, driverName)
		require.Equal(t, "postgres://mock-host/agent", dsn)
		return mockDB, nil
	}

	database, err := NewPostgres("postgres://mock-host/agent")
	require.NoError(t, err)

	require.Equal(t, driverPostgres, database.Driver())
	require.Equal(t, postgresMaxOpenConns, database.SQLDB().Stats().MaxOpenConnections)
	require.NoError(t, database.Close())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRunMigrationsSQLiteAndCRUD(t *testing.T) {
	path := filepath.Join(t.TempDir(), "migrations.db")

	database, err := NewSQLite(path)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, database.Close())
	})

	wd, err := os.Getwd()
	require.NoError(t, err)
	repoRoot := filepath.Clean(filepath.Join(wd, "../.."))
	require.NoError(t, os.Chdir(repoRoot))
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(wd))
	})

	require.NoError(t, RunMigrations(database))
	require.NoError(t, RunMigrations(database))

	now := time.Now().UTC().Truncate(time.Second)

	_, err = database.SQLDB().Exec(
		`INSERT INTO inbounds(tag, type, listen, config_json, enabled, created_at, updated_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?)`,
		"vless-reality", "vless", "0.0.0.0:443", `{"transport":"tcp"}`, true, now, now,
	)
	require.NoError(t, err)

	_, err = database.SQLDB().Exec(
		`INSERT INTO users(id, sub_id, inbound_tag, email, traffic_limit, expiry, enabled, created_at, updated_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"user-1", "sub-1", "vless-reality", "user@example.com", int64(1024), nil, true, now, now,
	)
	require.NoError(t, err)

	_, err = database.SQLDB().Exec(
		`INSERT INTO traffic_stats(id, inbound_tag, sub_id, up_bytes, down_bytes, recorded_at)
		 VALUES(?, ?, ?, ?, ?, ?)`,
		"stat-1", "vless-reality", "sub-1", int64(11), int64(22), now,
	)
	require.NoError(t, err)

	var count int
	err = database.SQLDB().QueryRow(`SELECT COUNT(*) FROM users WHERE sub_id = ?`, "sub-1").Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count)

	err = database.SQLDB().QueryRow(`SELECT COUNT(*) FROM traffic_stats WHERE sub_id = ?`, "sub-1").Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestRunMigrationsPostgresRequiresBackupConfirmation(t *testing.T) {
	originalOpen := openSQL
	t.Cleanup(func() {
		openSQL = originalOpen
	})

	mockDB, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	require.NoError(t, err)

	mock.ExpectPing()
	mock.ExpectClose()

	openSQL = func(driverName, dsn string) (*sql.DB, error) {
		return mockDB, nil
	}

	database, err := NewPostgres("postgres://mock-host/agent")
	require.NoError(t, err)

	err = RunMigrations(database)
	require.Error(t, err)
	require.Contains(t, err.Error(), "backup confirmation")
	require.NoError(t, database.Close())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRollbackLastMigrationSQLite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollback.db")
	database, err := NewSQLite(path)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, database.Close())
	})

	wd, err := os.Getwd()
	require.NoError(t, err)
	repoRoot := filepath.Clean(filepath.Join(wd, "../.."))
	require.NoError(t, os.Chdir(repoRoot))
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(wd))
	})

	require.NoError(t, RunMigrations(database))
	require.NoError(t, RollbackLastMigration(database))

	_, err = database.SQLDB().Exec(`SELECT 1 FROM users LIMIT 1`)
	require.Error(t, err)
	require.True(t, containsAny(err, "no such table", "relation \"users\" does not exist"), fmt.Sprintf("unexpected error: %v", err))
}

func containsAny(err error, messages ...string) bool {
	if err == nil {
		return false
	}
	for _, msg := range messages {
		if msg != "" && strings.Contains(err.Error(), msg) {
			return true
		}
	}
	return false
}

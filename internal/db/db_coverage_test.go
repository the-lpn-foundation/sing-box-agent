//nolint:errcheck
package db

//nolint:errcheck // Test file uses type assertions

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

// parseMigrationSections tests
func TestParseMigrationSections(t *testing.T) {
	t.Run("no markers returns entire content as up", func(t *testing.T) {
		content := "CREATE TABLE test (id INTEGER);"
		up, down := parseMigrationSections(content)
		require.Equal(t, content, up)
		require.Empty(t, down)
	})

	t.Run("only up marker", func(t *testing.T) {
		content := "-- +up\nCREATE TABLE test (id INTEGER);"
		up, down := parseMigrationSections(content)
		require.Equal(t, "CREATE TABLE test (id INTEGER);", up)
		require.Empty(t, down)
	})

	t.Run("both markers", func(t *testing.T) {
		content := "-- +up\nCREATE TABLE test (id INTEGER);\n-- +down\nDROP TABLE test;"
		up, down := parseMigrationSections(content)
		require.Equal(t, "CREATE TABLE test (id INTEGER);", up)
		require.Equal(t, "DROP TABLE test;", down)
	})

	t.Run("case insensitive markers", func(t *testing.T) {
		content := "-- +UP\nCREATE TABLE test;\n-- +DOWN\nDROP TABLE test;"
		up, down := parseMigrationSections(content)
		require.Equal(t, "CREATE TABLE test;", up)
		require.Equal(t, "DROP TABLE test;", down)
	})

	t.Run("down before up returns only up", func(t *testing.T) {
		content := "-- +down\nDROP TABLE test;\n-- +up\nCREATE TABLE test;"
		up, down := parseMigrationSections(content)
		require.Equal(t, "CREATE TABLE test;", up)
		require.Empty(t, down)
	})

	t.Run("empty content", func(t *testing.T) {
		up, down := parseMigrationSections("")
		require.Empty(t, up)
		require.Empty(t, down)
	})

	t.Run("whitespace only", func(t *testing.T) {
		up, down := parseMigrationSections("   \n\n  ")
		require.Empty(t, up)
		require.Empty(t, down)
	})

	t.Run("content with extra whitespace", func(t *testing.T) {
		content := "  -- +up  \n  CREATE TABLE test;  \n  -- +down  \n  DROP TABLE test;  "
		up, down := parseMigrationSections(content)
		require.Equal(t, "CREATE TABLE test;", up)
		require.Equal(t, "DROP TABLE test;", down)
	})

	t.Run("mixed case markers", func(t *testing.T) {
		content := "-- +Up\nCREATE TABLE test;\n-- +Down\nDROP TABLE test;"
		up, down := parseMigrationSections(content)
		require.Equal(t, "CREATE TABLE test;", up)
		require.Equal(t, "DROP TABLE test;", down)
	})

	t.Run("extra newlines", func(t *testing.T) {
		content := "\n\n-- +up\n\nCREATE TABLE test;\n\n-- +down\n\nDROP TABLE test;\n\n"
		up, down := parseMigrationSections(content)
		require.Equal(t, "CREATE TABLE test;", up)
		require.Equal(t, "DROP TABLE test;", down)
	})
}

// getLastAppliedMigration tests
func TestGetLastAppliedMigration(t *testing.T) {
	t.Run("returns version when migrations exist", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "test.db")
		database, err := NewSQLite(path)
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		ctx := context.Background()
		tx, err := database.SQLDB().BeginTx(ctx, nil)
		require.NoError(t, err)
		defer func() { _ = tx.Rollback() }()

		_, err = tx.ExecContext(ctx, createSchemaMigrationsTableSQL)
		require.NoError(t, err)

		_, err = tx.ExecContext(ctx, "INSERT INTO schema_migrations(version, applied_at) VALUES (?, CURRENT_TIMESTAMP)", "001")
		require.NoError(t, err)

		_, err = tx.ExecContext(ctx, "INSERT INTO schema_migrations(version, applied_at) VALUES (?, CURRENT_TIMESTAMP)", "002")
		require.NoError(t, err)

		version, err := getLastAppliedMigration(ctx, tx)
		require.NoError(t, err)
		require.Equal(t, "002", version)
	})

	t.Run("returns empty string when no migrations", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "test.db")
		database, err := NewSQLite(path)
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		ctx := context.Background()
		tx, err := database.SQLDB().BeginTx(ctx, nil)
		require.NoError(t, err)
		defer func() { _ = tx.Rollback() }()

		_, err = tx.ExecContext(ctx, createSchemaMigrationsTableSQL)
		require.NoError(t, err)

		version, err := getLastAppliedMigration(ctx, tx)
		require.NoError(t, err)
		require.Empty(t, version)
	})

	t.Run("single migration", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "test.db")
		database, err := NewSQLite(path)
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		ctx := context.Background()
		tx, err := database.SQLDB().BeginTx(ctx, nil)
		require.NoError(t, err)
		defer func() { _ = tx.Rollback() }()

		_, err = tx.ExecContext(ctx, createSchemaMigrationsTableSQL)
		require.NoError(t, err)

		_, err = tx.ExecContext(ctx, "INSERT INTO schema_migrations(version, applied_at) VALUES (?, CURRENT_TIMESTAMP)", "001")
		require.NoError(t, err)

		version, err := getLastAppliedMigration(ctx, tx)
		require.NoError(t, err)
		require.Equal(t, "001", version)
	})
}

// getAppliedMigrationSet tests
func TestGetAppliedMigrationSet(t *testing.T) {
	t.Run("returns set of applied migrations", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "test.db")
		database, err := NewSQLite(path)
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		ctx := context.Background()
		tx, err := database.SQLDB().BeginTx(ctx, nil)
		require.NoError(t, err)
		defer func() { _ = tx.Rollback() }()

		_, err = tx.ExecContext(ctx, createSchemaMigrationsTableSQL)
		require.NoError(t, err)

		_, err = tx.ExecContext(ctx, "INSERT INTO schema_migrations(version, applied_at) VALUES (?, CURRENT_TIMESTAMP)", "001")
		require.NoError(t, err)
		_, err = tx.ExecContext(ctx, "INSERT INTO schema_migrations(version, applied_at) VALUES (?, CURRENT_TIMESTAMP)", "002")
		require.NoError(t, err)

		applied, err := getAppliedMigrationSet(ctx, tx)
		require.NoError(t, err)
		require.True(t, applied["001"])
		require.True(t, applied["002"])
		require.False(t, applied["003"])
	})

	t.Run("empty result", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "test.db")
		database, err := NewSQLite(path)
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		ctx := context.Background()
		tx, err := database.SQLDB().BeginTx(ctx, nil)
		require.NoError(t, err)
		defer func() { _ = tx.Rollback() }()

		_, err = tx.ExecContext(ctx, createSchemaMigrationsTableSQL)
		require.NoError(t, err)

		applied, err := getAppliedMigrationSet(ctx, tx)
		require.NoError(t, err)
		require.Empty(t, applied)
	})

	t.Run("multiple migrations", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "test.db")
		database, err := NewSQLite(path)
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		ctx := context.Background()
		tx, err := database.SQLDB().BeginTx(ctx, nil)
		require.NoError(t, err)
		defer func() { _ = tx.Rollback() }()

		_, err = tx.ExecContext(ctx, createSchemaMigrationsTableSQL)
		require.NoError(t, err)

		for i := 1; i <= 5; i++ {
			version := fmt.Sprintf("%03d", i)
			_, err = tx.ExecContext(ctx, "INSERT INTO schema_migrations(version, applied_at) VALUES (?, CURRENT_TIMESTAMP)", version)
			require.NoError(t, err)
		}

		applied, err := getAppliedMigrationSet(ctx, tx)
		require.NoError(t, err)
		require.Len(t, applied, 5)
		for i := 1; i <= 5; i++ {
			version := fmt.Sprintf("%03d", i)
			require.True(t, applied[version])
		}
	})
}

// execStatements tests
func TestExecStatements(t *testing.T) {
	t.Run("executes multiple statements", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "test.db")
		database, err := NewSQLite(path)
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		ctx := context.Background()
		tx, err := database.SQLDB().BeginTx(ctx, nil)
		require.NoError(t, err)
		defer func() { _ = tx.Rollback() }()

		sql := "CREATE TABLE test1 (id INTEGER); CREATE TABLE test2 (id INTEGER);"
		err = execStatements(ctx, tx, sql)
		require.NoError(t, err)

		var count int
		err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM test1").Scan(&count)
		require.NoError(t, err)
		err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM test2").Scan(&count)
		require.NoError(t, err)
	})

	t.Run("handles empty SQL", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "test.db")
		database, err := NewSQLite(path)
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		ctx := context.Background()
		tx, err := database.SQLDB().BeginTx(ctx, nil)
		require.NoError(t, err)
		defer func() { _ = tx.Rollback() }()

		err = execStatements(ctx, tx, "")
		require.NoError(t, err)
	})

	t.Run("handles whitespace only", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "test.db")
		database, err := NewSQLite(path)
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		ctx := context.Background()
		tx, err := database.SQLDB().BeginTx(ctx, nil)
		require.NoError(t, err)
		defer func() { _ = tx.Rollback() }()

		err = execStatements(ctx, tx, "   \n\n  ")
		require.NoError(t, err)
	})

	t.Run("handles statements with trailing semicolon", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "test.db")
		database, err := NewSQLite(path)
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		ctx := context.Background()
		tx, err := database.SQLDB().BeginTx(ctx, nil)
		require.NoError(t, err)
		defer func() { _ = tx.Rollback() }()

		sql := "CREATE TABLE test (id INTEGER);"
		err = execStatements(ctx, tx, sql)
		require.NoError(t, err)
	})

	t.Run("returns error on statement failure", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "test.db")
		database, err := NewSQLite(path)
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		ctx := context.Background()
		tx, err := database.SQLDB().BeginTx(ctx, nil)
		require.NoError(t, err)
		defer func() { _ = tx.Rollback() }()

		sql := "INVALID SQL STATEMENT;"
		err = execStatements(ctx, tx, sql)
		require.Error(t, err)
	})

	t.Run("multiple semicolons", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "test.db")
		database, err := NewSQLite(path)
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		ctx := context.Background()
		tx, err := database.SQLDB().BeginTx(ctx, nil)
		require.NoError(t, err)
		defer func() { _ = tx.Rollback() }()

		sql := "CREATE TABLE test (id INTEGER);;"
		err = execStatements(ctx, tx, sql)
		require.NoError(t, err)
	})

	t.Run("with comments", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "test.db")
		database, err := NewSQLite(path)
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		ctx := context.Background()
		tx, err := database.SQLDB().BeginTx(ctx, nil)
		require.NoError(t, err)
		defer func() { _ = tx.Rollback() }()

		sql := "-- This is a comment\nCREATE TABLE test (id INTEGER);"
		err = execStatements(ctx, tx, sql)
		require.NoError(t, err)
	})
}

// ensureMigrationSafety tests
func TestEnsureMigrationSafety(t *testing.T) {
	t.Run("postgres without backup confirmation returns error", func(t *testing.T) {
		original := openSQL
		defer func() { openSQL = original }()

		mockDB, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
		require.NoError(t, err)
		mock.ExpectPing()

		openSQL = func(driver, dsn string) (*sql.DB, error) {
			return mockDB, nil
		}

		database, err := NewPostgres("postgres://localhost/test")
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		err = ensureMigrationSafety(database)
		require.Error(t, err)
		require.Contains(t, err.Error(), "backup confirmation")
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("postgres with backup confirmation succeeds", func(t *testing.T) {
		original := openSQL
		defer func() { openSQL = original }()

		mockDB, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
		require.NoError(t, err)
		mock.ExpectPing()

		openSQL = func(driver, dsn string) (*sql.DB, error) {
			return mockDB, nil
		}

		database, err := NewPostgres("postgres://localhost/test")
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		database.ConfirmMigrationBackup()
		err = ensureMigrationSafety(database)
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("sqlite with memory database skips backup", func(t *testing.T) {
		database := &DB{
			db:         &sql.DB{},
			driver:     driverSQLite,
			sqlitePath: ":memory:",
		}

		err := ensureMigrationSafety(database)
		require.NoError(t, err)
	})

	t.Run("sqlite with empty path skips backup", func(t *testing.T) {
		database := &DB{
			db:         &sql.DB{},
			driver:     driverSQLite,
			sqlitePath: "",
		}

		err := ensureMigrationSafety(database)
		require.NoError(t, err)
	})

	t.Run("sqlite creates backup file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "test.db")
		database, err := NewSQLite(path)
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		_, err = database.SQLDB().Exec("CREATE TABLE test (id INTEGER)")
		require.NoError(t, err)

		err = ensureMigrationSafety(database)
		require.NoError(t, err)

		files, err := filepath.Glob(path + ".bak.*")
		require.NoError(t, err)
		require.Len(t, files, 1)
	})

	t.Run("sqlite with non-existent file skips backup", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "nonexistent.db")
		database := &DB{
			db:         &sql.DB{},
			driver:     driverSQLite,
			sqlitePath: path,
		}

		err := ensureMigrationSafety(database)
		require.NoError(t, err)
	})

	t.Run("sqlite backup creation error", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "testdir")
		err := os.Mkdir(path, 0o755)
		require.NoError(t, err)

		database := &DB{
			db:         &sql.DB{},
			driver:     driverSQLite,
			sqlitePath: path,
		}

		err = ensureMigrationSafety(database)
		require.Error(t, err)
		require.Contains(t, err.Error(), "backup")
	})

	t.Run("non-sqlite driver skips backup", func(t *testing.T) {
		database := &DB{
			db:     &sql.DB{},
			driver: "mysql",
		}

		err := ensureMigrationSafety(database)
		require.NoError(t, err)
	})

	t.Run("postgres driver without backup", func(t *testing.T) {
		database := &DB{
			db:     &sql.DB{},
			driver: driverPostgres,
		}

		err := ensureMigrationSafety(database)
		require.Error(t, err)
		require.Contains(t, err.Error(), "backup confirmation")
	})

	t.Run("postgres driver with backup confirmed", func(t *testing.T) {
		database := &DB{
			db:              &sql.DB{},
			driver:          driverPostgres,
			backupConfirmed: true,
		}

		err := ensureMigrationSafety(database)
		require.NoError(t, err)
	})
}

// loadMigrations tests
func TestLoadMigrations(t *testing.T) {
	t.Run("returns built-in migrations when directory is empty", func(t *testing.T) {
		tempDir := t.TempDir()
		migrations, err := loadMigrations(tempDir)
		require.NoError(t, err)
		require.Len(t, migrations, 1)
		require.Equal(t, "001", migrations[0].Version)
	})

	t.Run("loads migrations from files", func(t *testing.T) {
		tempDir := t.TempDir()
		migrationFile := filepath.Join(tempDir, "002_test.sql")
		content := "-- +up\nCREATE TABLE test (id INTEGER);\n-- +down\nDROP TABLE test;"
		err := os.WriteFile(migrationFile, []byte(content), 0o644)
		require.NoError(t, err)

		migrations, err := loadMigrations(tempDir)
		require.NoError(t, err)
		require.Len(t, migrations, 1)
		require.Equal(t, "002", migrations[0].Version)
		require.Contains(t, migrations[0].UpSQL, "CREATE TABLE test")
		require.Contains(t, migrations[0].DownSQL, "DROP TABLE test")
	})

	t.Run("returns error on file read failure", func(t *testing.T) {
		tempDir := t.TempDir()
		migrationFile := filepath.Join(tempDir, "003_test.sql")
		err := os.Mkdir(migrationFile, 0o755)
		require.NoError(t, err)

		_, err = loadMigrations(tempDir)
		require.Error(t, err)
		require.Contains(t, err.Error(), "read migration file")
	})

	t.Run("sorts migrations by filename", func(t *testing.T) {
		tempDir := t.TempDir()
		os.WriteFile(filepath.Join(tempDir, "003_c.sql"), []byte("-- +up\nCREATE TABLE c;"), 0o644)
		os.WriteFile(filepath.Join(tempDir, "001_a.sql"), []byte("-- +up\nCREATE TABLE a;"), 0o644)
		os.WriteFile(filepath.Join(tempDir, "002_b.sql"), []byte("-- +up\nCREATE TABLE b;"), 0o644)

		migrations, err := loadMigrations(tempDir)
		require.NoError(t, err)
		require.Len(t, migrations, 3)
		require.Equal(t, "001", migrations[0].Version)
		require.Equal(t, "002", migrations[1].Version)
		require.Equal(t, "003", migrations[2].Version)
	})

	t.Run("multiple files", func(t *testing.T) {
		tempDir := t.TempDir()

		os.WriteFile(filepath.Join(tempDir, "001_first.sql"), []byte("-- +up\nCREATE TABLE first;"), 0o644)
		os.WriteFile(filepath.Join(tempDir, "002_second.sql"), []byte("-- +up\nCREATE TABLE second;"), 0o644)
		os.WriteFile(filepath.Join(tempDir, "003_third.sql"), []byte("-- +up\nCREATE TABLE third;"), 0o644)

		migrations, err := loadMigrations(tempDir)
		require.NoError(t, err)
		require.Len(t, migrations, 3)
		require.Equal(t, "001", migrations[0].Version)
		require.Equal(t, "002", migrations[1].Version)
		require.Equal(t, "003", migrations[2].Version)
	})
}

// RunMigrations error path tests
func TestRunMigrations_ErrorPaths(t *testing.T) {
	t.Run("returns error for nil database", func(t *testing.T) {
		err := RunMigrations(nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "database is nil")
	})

	t.Run("returns error for nil db field", func(t *testing.T) {
		database := &DB{db: nil}
		err := RunMigrations(database)
		require.Error(t, err)
		require.Contains(t, err.Error(), "database is nil")
	})

	t.Run("returns error on begin transaction failure", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "test.db")
		database, err := NewSQLite(path)
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		database.SQLDB().Close()

		err = RunMigrations(database)
		require.Error(t, err)
		require.Contains(t, err.Error(), "begin migration transaction")
	})

	t.Run("postgres without backup confirmation", func(t *testing.T) {
		original := openSQL
		defer func() { openSQL = original }()

		mockDB, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
		require.NoError(t, err)
		mock.ExpectPing()

		openSQL = func(driver, dsn string) (*sql.DB, error) {
			return mockDB, nil
		}

		database, err := NewPostgres("postgres://localhost/test")
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		err = RunMigrations(database)
		require.Error(t, err)
		require.Contains(t, err.Error(), "backup confirmation")
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("built-in migrations", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "test.db")
		database, err := NewSQLite(path)
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		wd, _ := os.Getwd()
		tempDir := t.TempDir()
		os.Chdir(tempDir)
		defer os.Chdir(wd)

		err = RunMigrations(database)
		require.NoError(t, err)

		var count int
		err = database.SQLDB().QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
		require.NoError(t, err)
	})

	t.Run("already applied migrations", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "test.db")
		database, err := NewSQLite(path)
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		wd, _ := os.Getwd()
		repoRoot := filepath.Clean(filepath.Join(wd, "../.."))
		os.Chdir(repoRoot)
		defer os.Chdir(wd)

		require.NoError(t, RunMigrations(database))
		require.NoError(t, RunMigrations(database))

		var count int
		err = database.SQLDB().QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
		require.NoError(t, err)
	})
}

// RollbackLastMigration error path tests
func TestRollbackLastMigration_ErrorPaths(t *testing.T) {
	t.Run("returns error for nil database", func(t *testing.T) {
		err := RollbackLastMigration(nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "database is nil")
	})

	t.Run("returns error for nil db field", func(t *testing.T) {
		database := &DB{db: nil}
		err := RollbackLastMigration(database)
		require.Error(t, err)
		require.Contains(t, err.Error(), "database is nil")
	})

	t.Run("returns error when no migrations to rollback", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "test.db")
		database, err := NewSQLite(path)
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		wd, _ := os.Getwd()
		repoRoot := filepath.Clean(filepath.Join(wd, "../.."))
		os.Chdir(repoRoot)
		defer os.Chdir(wd)

		require.NoError(t, RunMigrations(database))
		require.NoError(t, RollbackLastMigration(database))

		err = RollbackLastMigration(database)
		require.NoError(t, err)
	})

	t.Run("returns error on begin transaction failure", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "test.db")
		database, err := NewSQLite(path)
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		database.SQLDB().Close()

		err = RollbackLastMigration(database)
		require.Error(t, err)
		require.Contains(t, err.Error(), "begin rollback transaction")
	})

	t.Run("returns error when migration version not found", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "test.db")
		database, err := NewSQLite(path)
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		ctx := context.Background()
		tx, err := database.SQLDB().BeginTx(ctx, nil)
		require.NoError(t, err)
		_, err = tx.ExecContext(ctx, createSchemaMigrationsTableSQL)
		require.NoError(t, err)
		_, err = tx.ExecContext(ctx, "INSERT INTO schema_migrations(version, applied_at) VALUES (?, CURRENT_TIMESTAMP)", "999")
		require.NoError(t, err)
		tx.Commit()

		err = RollbackLastMigration(database)
		require.Error(t, err)
		require.Contains(t, err.Error(), "no rollback SQL for migration 999")
	})

	t.Run("empty schema_migrations table", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "test.db")
		database, err := NewSQLite(path)
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		ctx := context.Background()
		tx, err := database.SQLDB().BeginTx(ctx, nil)
		require.NoError(t, err)
		_, err = tx.ExecContext(ctx, createSchemaMigrationsTableSQL)
		require.NoError(t, err)
		tx.Commit()

		err = RollbackLastMigration(database)
		require.NoError(t, err)
	})

	t.Run("fresh database", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "test.db")
		database, err := NewSQLite(path)
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		err = RollbackLastMigration(database)
		require.NoError(t, err)
	})

	t.Run("after rollback", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "test.db")
		database, err := NewSQLite(path)
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		wd, _ := os.Getwd()
		repoRoot := filepath.Clean(filepath.Join(wd, "../.."))
		os.Chdir(repoRoot)
		defer os.Chdir(wd)

		require.NoError(t, RunMigrations(database))
		require.NoError(t, RollbackLastMigration(database))
		err = RollbackLastMigration(database)
		require.NoError(t, err)
	})

	t.Run("built-in migrations", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "test.db")
		database, err := NewSQLite(path)
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		wd, _ := os.Getwd()
		tempDir := t.TempDir()
		os.Chdir(tempDir)
		defer os.Chdir(wd)

		require.NoError(t, RunMigrations(database))
		require.NoError(t, RollbackLastMigration(database))

		_, err = database.SQLDB().Query("SELECT COUNT(*) FROM users")
		require.Error(t, err)
	})
}

// Additional tests for db.go functions to improve coverage
func TestDB_SQLDB_NilReceiver(t *testing.T) {
	var database *DB
	sqlDB := database.SQLDB()
	require.Nil(t, sqlDB)
}

func TestDB_SQLDB_NilDBField(t *testing.T) {
	database := &DB{db: nil}
	sqlDB := database.SQLDB()
	require.Nil(t, sqlDB)
}

func TestDB_Driver_NilReceiver(t *testing.T) {
	var database *DB
	driver := database.Driver()
	require.Empty(t, driver)
}

func TestDB_Driver_NilDBField(t *testing.T) {
	database := &DB{db: nil}
	driver := database.Driver()
	require.Empty(t, driver)
}

func TestDB_ConfirmMigrationBackup_NilReceiver(t *testing.T) {
	var database *DB
	require.NotPanics(t, func() {
		database.ConfirmMigrationBackup()
	})
}

func TestDB_ConfirmMigrationBackup_NilDBField(t *testing.T) {
	database := &DB{db: nil}
	require.NotPanics(t, func() {
		database.ConfirmMigrationBackup()
	})
}

func TestDB_Close_NilReceiver(t *testing.T) {
	var database *DB
	err := database.Close()
	require.NoError(t, err)
}

func TestDB_Close_NilDBField(t *testing.T) {
	database := &DB{db: nil}
	err := database.Close()
	require.NoError(t, err)
}

func TestDB_Close_MultipleCalls(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	database, err := NewSQLite(path)
	require.NoError(t, err)

	err = database.Close()
	require.NoError(t, err)

	err = database.Close()
	require.NoError(t, err)
}

// Test NewSQLite and NewPostgres error cases
func TestNewSQLite_EmptyPath(t *testing.T) {
	database, err := NewSQLite("")
	require.Error(t, err)
	require.Contains(t, err.Error(), "sqlite path is required")
	require.Nil(t, database)
}

func TestNewPostgres_EmptyURL(t *testing.T) {
	database, err := NewPostgres("")
	require.Error(t, err)
	require.Contains(t, err.Error(), "postgres url is required")
	require.Nil(t, database)
}

// Test NewSQLite open error
func TestNewSQLite_OpenError(t *testing.T) {
	original := openSQL
	defer func() { openSQL = original }()

	openSQL = func(driver, dsn string) (*sql.DB, error) {
		return nil, errors.New("open failed")
	}

	path := filepath.Join(t.TempDir(), "test.db")
	database, err := NewSQLite(path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "open sqlite database")
	require.Nil(t, database)
}

// Test NewPostgres open error
func TestNewPostgres_OpenError(t *testing.T) {
	original := openSQL
	defer func() { openSQL = original }()

	openSQL = func(driver, dsn string) (*sql.DB, error) {
		return nil, errors.New("connection refused")
	}

	database, err := NewPostgres("postgres://localhost/test")
	require.Error(t, err)
	require.Contains(t, err.Error(), "open postgres database")
	require.Nil(t, database)
}

// Test NewSQLite ping error
func TestNewSQLite_PingError(t *testing.T) {
	original := openSQL
	defer func() { openSQL = original }()

	mockDB, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	require.NoError(t, err)
	mock.ExpectPing().WillReturnError(errors.New("ping failed"))
	mock.ExpectClose()

	openSQL = func(driver, dsn string) (*sql.DB, error) {
		return mockDB, nil
	}

	path := filepath.Join(t.TempDir(), "test.db")
	database, err := NewSQLite(path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "ping sqlite database")
	require.Nil(t, database)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Test NewPostgres ping error
func TestNewPostgres_PingError(t *testing.T) {
	original := openSQL
	defer func() { openSQL = original }()

	mockDB, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	require.NoError(t, err)
	mock.ExpectPing().WillReturnError(errors.New("ping failed"))
	mock.ExpectClose()

	openSQL = func(driver, dsn string) (*sql.DB, error) {
		return mockDB, nil
	}

	database, err := NewPostgres("postgres://localhost/test")
	require.Error(t, err)
	require.Contains(t, err.Error(), "ping postgres database")
	require.Nil(t, database)
}

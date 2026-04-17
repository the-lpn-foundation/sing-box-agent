package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type migration struct {
	Version string
	UpSQL   string
	DownSQL string
}

var builtInMigrations = []migration{
	{Version: "001", UpSQL: initialMigrationUpSQL, DownSQL: initialMigrationDownSQL},
}

func RunMigrations(database *DB) error {
	if database == nil || database.db == nil {
		return errors.New("database is nil")
	}

	if err := ensureMigrationSafety(database); err != nil {
		return err
	}

	migrations, err := loadMigrations("migrations")
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tx, err := database.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if _, err := tx.ExecContext(ctx, createSchemaMigrationsTableSQL); err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}

	applied, err := getAppliedMigrationSet(ctx, tx)
	if err != nil {
		return err
	}

	for _, item := range migrations {
		if applied[item.Version] {
			continue
		}

		if err := execStatements(ctx, tx, item.UpSQL); err != nil {
			return fmt.Errorf("apply migration %s: %w", item.Version, err)
		}

		if _, err := tx.ExecContext(
			ctx,
			"INSERT INTO schema_migrations(version, applied_at) VALUES (?, CURRENT_TIMESTAMP)",
			item.Version,
		); err != nil {
			if _, errPg := tx.ExecContext(
				ctx,
				"INSERT INTO schema_migrations(version, applied_at) VALUES ($1, CURRENT_TIMESTAMP)",
				item.Version,
			); errPg != nil {
				return fmt.Errorf("record migration %s: %w", item.Version, errPg)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migrations: %w", err)
	}

	return nil
}

func RollbackLastMigration(database *DB) error {
	if database == nil || database.db == nil {
		return errors.New("database is nil")
	}

	migrations, err := loadMigrations("migrations")
	if err != nil {
		return err
	}
	migrationByVersion := map[string]migration{}
	for _, item := range builtInMigrations {
		migrationByVersion[item.Version] = item
	}
	for _, item := range migrations {
		if item.DownSQL == "" {
			if fallback, ok := migrationByVersion[item.Version]; ok {
				item.DownSQL = fallback.DownSQL
			}
		}
		migrationByVersion[item.Version] = item
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tx, err := database.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin rollback transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if _, err := tx.ExecContext(ctx, createSchemaMigrationsTableSQL); err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}

	version, err := getLastAppliedMigration(ctx, tx)
	if err != nil {
		return err
	}
	if version == "" {
		return nil
	}

	item, ok := migrationByVersion[version]
	if !ok {
		return fmt.Errorf("no rollback SQL for migration %s", version)
	}

	if err := execStatements(ctx, tx, item.DownSQL); err != nil {
		return fmt.Errorf("rollback migration %s: %w", item.Version, err)
	}

	if _, err := tx.ExecContext(ctx, "DELETE FROM schema_migrations WHERE version = ?", version); err != nil {
		if _, errPg := tx.ExecContext(ctx, "DELETE FROM schema_migrations WHERE version = $1", version); errPg != nil {
			return fmt.Errorf("delete migration %s record: %w", item.Version, errPg)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit rollback: %w", err)
	}

	return nil
}

func loadMigrations(migrationsDir string) ([]migration, error) {
	glob := filepath.Join(migrationsDir, "*.sql")
	files, err := filepath.Glob(glob)
	if err != nil {
		return nil, fmt.Errorf("glob migrations: %w", err)
	}
	if len(files) == 0 {
		return builtInMigrations, nil
	}

	sort.Strings(files)
	loaded := make([]migration, 0, len(files))
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("read migration file %s: %w", file, err)
		}

		base := filepath.Base(file)
		parts := strings.SplitN(base, "_", 2)
		version := strings.TrimSuffix(parts[0], ".sql")
		upSQL, downSQL := parseMigrationSections(string(content))
		loaded = append(loaded, migration{Version: version, UpSQL: upSQL, DownSQL: downSQL})
	}

	return loaded, nil
}

func parseMigrationSections(content string) (string, string) {
	upMarker := "-- +up"
	downMarker := "-- +down"

	text := strings.TrimSpace(content)
	upIdx := strings.Index(strings.ToLower(text), upMarker)
	downIdx := strings.Index(strings.ToLower(text), downMarker)

	if upIdx == -1 && downIdx == -1 {
		return text, ""
	}

	if upIdx == -1 {
		upIdx = 0
	}
	if downIdx == -1 || downIdx < upIdx {
		up := strings.TrimSpace(text[upIdx+len(upMarker):])
		return up, ""
	}

	up := strings.TrimSpace(text[upIdx+len(upMarker) : downIdx])
	down := strings.TrimSpace(text[downIdx+len(downMarker):])
	return up, down
}

func getAppliedMigrationSet(ctx context.Context, tx *sql.Tx) (map[string]bool, error) {
	rows, err := tx.QueryContext(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return nil, fmt.Errorf("query applied migrations: %w", err)
	}
	defer func() { _ = rows.Close() }()

	applied := make(map[string]bool)
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return nil, fmt.Errorf("scan applied migration: %w", err)
		}
		applied[version] = true
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate applied migrations: %w", err)
	}

	return applied, nil
}

func getLastAppliedMigration(ctx context.Context, tx *sql.Tx) (string, error) {
	var version string
	err := tx.QueryRowContext(ctx, "SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1").Scan(&version)
	if err == nil {
		return version, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("query last migration: %w", err)
	}
	return "", nil
}

func execStatements(ctx context.Context, tx *sql.Tx, rawSQL string) error {
	trimmed := strings.TrimSpace(rawSQL)
	if trimmed == "" {
		return nil
	}

	chunks := strings.Split(trimmed, ";")
	for _, chunk := range chunks {
		stmt := strings.TrimSpace(chunk)
		if stmt == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}

	return nil
}

func ensureMigrationSafety(database *DB) error {
	if database.driver == driverPostgres && !database.backupConfirmed {
		return errors.New("postgres migrations require backup confirmation via ConfirmMigrationBackup")
	}

	if database.driver != driverSQLite {
		return nil
	}

	if database.sqlitePath == "" || strings.Contains(database.sqlitePath, ":memory:") {
		return nil
	}

	file, err := os.Open(database.sqlitePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open sqlite database for backup: %w", err)
	}
	defer func() { _ = file.Close() }()

	backupPath := fmt.Sprintf("%s.bak.%d", database.sqlitePath, time.Now().UTC().Unix())
	backup, err := os.Create(backupPath)
	if err != nil {
		return fmt.Errorf("create sqlite backup %s: %w", backupPath, err)
	}
	defer func() { _ = backup.Close() }()

	if _, err := io.Copy(backup, file); err != nil {
		return fmt.Errorf("copy sqlite backup to %s: %w", backupPath, err)
	}

	return nil
}

package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

const (
	driverSQLite   = "sqlite"
	driverPostgres = "pgx"
)

const (
	sqliteMaxOpenConns = 1
	sqliteMaxIdleConns = 1

	postgresMaxOpenConns = 20
	postgresMaxIdleConns = 10
)

var openSQL = sql.Open

type DB struct {
	db              *sql.DB
	driver          string
	dsn             string
	sqlitePath      string
	backupConfirmed bool
}

func NewSQLite(path string) (*DB, error) {
	if path == "" {
		return nil, errors.New("sqlite path is required")
	}

	raw, err := openSQL(driverSQLite, path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}

	raw.SetMaxOpenConns(sqliteMaxOpenConns)
	raw.SetMaxIdleConns(sqliteMaxIdleConns)
	raw.SetConnMaxLifetime(0)
	raw.SetConnMaxIdleTime(0)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := raw.PingContext(ctx); err != nil {
		_ = raw.Close()
		return nil, fmt.Errorf("ping sqlite database: %w", err)
	}

	return &DB{
		db:              raw,
		driver:          driverSQLite,
		dsn:             path,
		sqlitePath:      path,
		backupConfirmed: true,
	}, nil
}

func NewPostgres(url string) (*DB, error) {
	if url == "" {
		return nil, errors.New("postgres url is required")
	}

	raw, err := openSQL(driverPostgres, url)
	if err != nil {
		return nil, fmt.Errorf("open postgres database: %w", err)
	}

	raw.SetMaxOpenConns(postgresMaxOpenConns)
	raw.SetMaxIdleConns(postgresMaxIdleConns)
	raw.SetConnMaxLifetime(1 * time.Hour)
	raw.SetConnMaxIdleTime(15 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := raw.PingContext(ctx); err != nil {
		_ = raw.Close()
		return nil, fmt.Errorf("ping postgres database: %w", err)
	}

	return &DB{
		db:              raw,
		driver:          driverPostgres,
		dsn:             url,
		backupConfirmed: false,
	}, nil
}

func (d *DB) SQLDB() *sql.DB {
	if d == nil {
		return nil
	}
	return d.db
}

func (d *DB) Driver() string {
	if d == nil {
		return ""
	}
	return d.driver
}

func (d *DB) ConfirmMigrationBackup() {
	if d == nil {
		return
	}
	d.backupConfirmed = true
}

func (d *DB) Close() error {
	if d == nil || d.db == nil {
		return nil
	}
	return d.db.Close()
}

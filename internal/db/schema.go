package db

import "time"

type UserRow struct {
	ID           string
	SubID        string
	InboundTag   string
	Email        string
	TrafficLimit int64
	Expiry       *time.Time
	Enabled      bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type InboundRow struct {
	Tag        string
	Type       string
	Listen     string
	ConfigJSON string
	Enabled    bool
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type TrafficStatRow struct {
	ID         string
	InboundTag string
	SubID      string
	UpBytes    int64
	DownBytes  int64
	RecordedAt time.Time
}

const createSchemaMigrationsTableSQL = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version TEXT PRIMARY KEY,
    applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);`

const initialMigrationUpSQL = `
CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    sub_id TEXT NOT NULL UNIQUE,
    inbound_tag TEXT NOT NULL,
    email TEXT NOT NULL,
    traffic_limit BIGINT NOT NULL DEFAULT 0,
    expiry TIMESTAMP NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS inbounds (
    tag TEXT PRIMARY KEY,
    type TEXT NOT NULL,
    listen TEXT NOT NULL,
    config_json TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS traffic_stats (
    id TEXT PRIMARY KEY,
    inbound_tag TEXT NOT NULL,
    sub_id TEXT NOT NULL,
    up_bytes BIGINT NOT NULL DEFAULT 0,
    down_bytes BIGINT NOT NULL DEFAULT 0,
    recorded_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (inbound_tag) REFERENCES inbounds(tag) ON DELETE CASCADE,
    FOREIGN KEY (sub_id) REFERENCES users(sub_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_users_sub_id ON users(sub_id);
CREATE INDEX IF NOT EXISTS idx_users_inbound_tag ON users(inbound_tag);
CREATE INDEX IF NOT EXISTS idx_traffic_stats_inbound_tag ON traffic_stats(inbound_tag);
CREATE INDEX IF NOT EXISTS idx_traffic_stats_sub_id ON traffic_stats(sub_id);
CREATE INDEX IF NOT EXISTS idx_traffic_stats_recorded_at ON traffic_stats(recorded_at);
`

const initialMigrationDownSQL = `
DROP TABLE IF EXISTS traffic_stats;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS inbounds;
`

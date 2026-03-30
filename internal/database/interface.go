package database

import (
	"context"
	"database/sql"
)

// Store abstracts database operations across PostgreSQL and MSSQL.
type Store interface {
	// Connect opens a connection pool to the database.
	Connect(ctx context.Context, dsn string, maxOpen, maxIdle int, maxLifetime string) error

	// Ping checks connectivity using the pool.
	Ping(ctx context.Context) error

	// PingDedicated checks connectivity using a dedicated connection (not from pool).
	// Used by health checks to avoid pool exhaustion (DB-07).
	PingDedicated(ctx context.Context, dsn string) error

	// Close closes the connection pool.
	Close() error

	// ApplySchema runs idempotent DDL to create required tables and indexes.
	ApplySchema(ctx context.Context, dimension int) error

	// Pool returns the underlying *sql.DB for use by vectorstore/querystore.
	Pool() *sql.DB

	// Engine returns the engine name ("postgres" or "mssql").
	Engine() string
}

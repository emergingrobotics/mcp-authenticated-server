package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// Store implements database.Store for PostgreSQL.
type Store struct {
	db *sql.DB
}

func New() *Store {
	return &Store{}
}

func (s *Store) Connect(ctx context.Context, dsn string, maxOpen, maxIdle int, maxLifetime string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("postgres connect: %w", err)
	}

	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)

	if maxLifetime != "" {
		d, err := time.ParseDuration(maxLifetime)
		if err != nil {
			return fmt.Errorf("postgres: invalid conn_max_lifetime: %w", err)
		}
		db.SetConnMaxLifetime(d)
	}

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return fmt.Errorf("postgres ping: %w", err)
	}

	s.db = db
	return nil
}

func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

// PingDedicated opens a separate connection for health check (DB-07).
func (s *Store) PingDedicated(ctx context.Context, dsn string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("dedicated ping open: %w", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	return db.PingContext(ctx)
}

func (s *Store) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

func (s *Store) ApplySchema(ctx context.Context, dimension int) error {
	return applySchema(ctx, s.db, dimension)
}

func (s *Store) Pool() *sql.DB {
	return s.db
}

func (s *Store) Engine() string {
	return "postgres"
}

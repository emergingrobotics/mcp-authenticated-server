package mssql

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/microsoft/go-mssqldb"
)

// Store implements database.Store for MS SQL Server.
type Store struct {
	db *sql.DB
}

func New() *Store {
	return &Store{}
}

func (s *Store) Connect(ctx context.Context, dsn string, maxOpen, maxIdle int, maxLifetime string) error {
	db, err := sql.Open("sqlserver", dsn)
	if err != nil {
		return fmt.Errorf("mssql connect: %w", err)
	}

	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)

	if maxLifetime != "" {
		d, err := time.ParseDuration(maxLifetime)
		if err != nil {
			return fmt.Errorf("mssql: invalid conn_max_lifetime: %w", err)
		}
		db.SetConnMaxLifetime(d)
	}

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return fmt.Errorf("mssql ping: %w", err)
	}

	s.db = db
	return nil
}

func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func (s *Store) PingDedicated(ctx context.Context, dsn string) error {
	db, err := sql.Open("sqlserver", dsn)
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
	return applySchema(ctx, s.db)
}

func (s *Store) Pool() *sql.DB {
	return s.db
}

func (s *Store) Engine() string {
	return "mssql"
}

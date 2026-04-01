package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/emergingrobotics/mcp-authenticated-server/internal/config"
	"github.com/emergingrobotics/mcp-authenticated-server/internal/database"
	dbmssql "github.com/emergingrobotics/mcp-authenticated-server/internal/database/mssql"
	dbpostgres "github.com/emergingrobotics/mcp-authenticated-server/internal/database/postgres"
)

// SetupLogging configures the default slog logger at the given level.
func SetupLogging(level string) {
	var logLevel slog.Level
	switch strings.ToLower(level) {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})
	slog.SetDefault(slog.New(handler))
}

// LoadConfig reads and validates the TOML config file, exiting on error.
func LoadConfig(path string) *config.Config {
	cfg, err := config.Load(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}
	return cfg
}

// CreateStore returns a database.Store for the configured engine, exiting on error.
func CreateStore(cfg *config.Config) database.Store {
	switch cfg.Database.Engine {
	case "postgres":
		return dbpostgres.New()
	case "mssql":
		return dbmssql.New()
	default:
		fmt.Fprintf(os.Stderr, "unsupported database engine: %s\n", cfg.Database.Engine)
		os.Exit(1)
		return nil
	}
}

// ConnectDB opens a database connection using DATABASE_URL, exiting on error.
func ConnectDB(ctx context.Context, cfg *config.Config, store database.Store) {
	dsn := cfg.Database.URL
	if dsn == "" {
		dsn = os.Getenv("DATABASE_URL")
	}
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URL not set")
		os.Exit(1)
	}

	if err := store.Connect(ctx, dsn, cfg.Database.MaxOpenConns, cfg.Database.MaxIdleConns, cfg.Database.ConnMaxLifetime); err != nil {
		fmt.Fprintf(os.Stderr, "database connection failed: %v\n", err)
		os.Exit(1)
	}
}

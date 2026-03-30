package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/emergingrobotics/mcp-authenticated-server/internal/auth"
	"github.com/emergingrobotics/mcp-authenticated-server/internal/config"
	"github.com/emergingrobotics/mcp-authenticated-server/internal/database"
	dbmssql "github.com/emergingrobotics/mcp-authenticated-server/internal/database/mssql"
	dbpostgres "github.com/emergingrobotics/mcp-authenticated-server/internal/database/postgres"
	"github.com/emergingrobotics/mcp-authenticated-server/internal/embed"
	"github.com/emergingrobotics/mcp-authenticated-server/internal/guardrails"
	hydemod "github.com/emergingrobotics/mcp-authenticated-server/internal/hyde"
	"github.com/emergingrobotics/mcp-authenticated-server/internal/ingest"
	"github.com/emergingrobotics/mcp-authenticated-server/internal/querystore"
	rerankmod "github.com/emergingrobotics/mcp-authenticated-server/internal/rerank"
	"github.com/emergingrobotics/mcp-authenticated-server/internal/search"
	"github.com/emergingrobotics/mcp-authenticated-server/internal/server"
	"github.com/emergingrobotics/mcp-authenticated-server/internal/tools"
	"github.com/emergingrobotics/mcp-authenticated-server/internal/vectorstore"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const version = "0.1.0"

func main() {
	configPath := flag.String("config", "config.toml", "path to config file")
	flag.Parse()

	args := flag.Args()
	command := "serve"
	if len(args) > 0 {
		command = args[0]
	}

	switch command {
	case "serve":
		runServe(*configPath)
	case "ingest":
		runIngest(*configPath, args[1:])
	case "validate":
		runValidate(*configPath)
	case "schema":
		runSchema(*configPath)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\nCommands: serve, ingest, validate, schema\n", command)
		os.Exit(1)
	}
}

func setupLogging(level string) {
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

func loadConfig(path string) *config.Config {
	cfg, err := config.Load(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}
	return cfg
}

func createStore(cfg *config.Config) database.Store {
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

func connectDB(ctx context.Context, cfg *config.Config, store database.Store) {
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
		os.Exit(1) // ERR-02
	}
}

func runValidate(configPath string) {
	_, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid config: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("config is valid")
	os.Exit(0)
}

func runSchema(configPath string) {
	cfg := loadConfig(configPath)
	setupLogging(cfg.Server.LogLevel)

	ctx := context.Background()
	store := createStore(cfg)
	connectDB(ctx, cfg, store)
	defer store.Close()

	if err := store.ApplySchema(ctx, cfg.Embed.Dimension); err != nil {
		fmt.Fprintf(os.Stderr, "schema apply failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("schema applied successfully")
}

func runIngest(configPath string, args []string) {
	cfg := loadConfig(configPath)
	setupLogging(cfg.Server.LogLevel)

	ingestFlags := flag.NewFlagSet("ingest", flag.ExitOnError)
	dirs := &stringSlice{}
	ingestFlags.Var(dirs, "dir", "directory to ingest (repeatable)")
	drop := ingestFlags.Bool("drop", false, "drop and recreate tables")
	dryRun := ingestFlags.Bool("dry-run", false, "show what would be ingested")
	verbose := ingestFlags.Bool("verbose", false, "verbose output")
	ingestFlags.Parse(args)

	if *verbose {
		setupLogging("debug")
	}

	if len(*dirs) == 0 {
		fmt.Fprintln(os.Stderr, "at least one --dir is required")
		os.Exit(1)
	}

	ctx := context.Background()
	store := createStore(cfg)
	connectDB(ctx, cfg, store)
	defer store.Close()

	if err := store.ApplySchema(ctx, cfg.Embed.Dimension); err != nil {
		fmt.Fprintf(os.Stderr, "schema apply failed: %v\n", err)
		os.Exit(1)
	}

	embedClient := embed.NewClient(cfg.Embed.Host, cfg.Embed.Model, cfg.Embed.QueryPrefix, cfg.Embed.PassagePrefix)
	vs := vectorstore.NewPostgresStore(store.Pool(), cfg.Embed.Dimension)

	pipeline := ingest.NewPipeline(vs, embedClient, &ingest.MarkdownChunker{}, ingest.PipelineConfig{
		ChunkSize:         cfg.Ingest.ChunkSize,
		BatchSize:         cfg.Ingest.BatchSize,
		MaxFileSize:       cfg.Ingest.MaxFileSizeBytes,
		AllowedDirs:       cfg.Ingest.AllowedDirs,
		AllowedExtensions: cfg.Ingest.AllowedExtensions,
		ExcludedDirs:      cfg.Ingest.ExcludedDirs,
		PassagePrefix:     cfg.Embed.PassagePrefix,
	})

	if *dryRun {
		fmt.Println("dry-run mode: would ingest from:")
		for _, d := range *dirs {
			fmt.Printf("  %s\n", d)
		}
		return
	}

	for _, d := range *dirs {
		result, err := pipeline.Ingest(ctx, d, *drop, cfg.Embed.Dimension)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ingest error for %s: %v\n", d, err)
			continue
		}
		fmt.Printf("Ingested %s: %d docs, %d chunks, %d errors, %.2fs\n",
			d, result.DocumentsProcessed, result.ChunksCreated, result.Errors, result.DurationSeconds)
	}
}

func runServe(configPath string) {
	cfg := loadConfig(configPath)
	setupLogging(cfg.Server.LogLevel)

	ctx := context.Background()

	// Connect to database
	store := createStore(cfg)
	connectDB(ctx, cfg, store)
	defer store.Close()

	// Apply schema
	if err := store.ApplySchema(ctx, cfg.Embed.Dimension); err != nil {
		slog.Error("schema apply failed", "error", err)
		os.Exit(1)
	}

	// Initialize auth (ERR-03: exit on JWKS failure)
	validator := auth.NewCognitoValidator(
		cfg.Auth.Region, cfg.Auth.UserPoolID, cfg.Auth.ClientID, cfg.Auth.TokenUse,
	)
	if err := validator.FetchJWKS(ctx); err != nil {
		slog.Error("JWKS fetch failed at startup", "error", err)
		os.Exit(1)
	}

	authorizer := &auth.GroupAuthorizer{
		ToolGroups:   cfg.Auth.ToolGroups,
		ServerGroups: cfg.Auth.AllowedGroups,
	}

	// Create MCP server
	mcpServer := mcp.NewServer(&mcp.Implementation{
		Name:    "mcp-authenticated-server",
		Version: version,
	}, nil)

	// Register tools conditionally
	if cfg.VectorEnabled() {
		// Initialize embed client
		embedClient := embed.NewClient(cfg.Embed.Host, cfg.Embed.Model, cfg.Embed.QueryPrefix, cfg.Embed.PassagePrefix)

		// Compute topic vector at startup (PERF-05)
		var guard *guardrails.Guardrails
		if cfg.Guardrails.CorpusTopic != "" {
			topicEmbeddings, err := embedClient.Embed(ctx, []string{cfg.Guardrails.CorpusTopic})
			if err != nil {
				slog.Error("failed to embed corpus_topic", "error", err)
				os.Exit(1)
			}
			tg := guardrails.NewTopicGuard(topicEmbeddings[0], float32(cfg.Guardrails.MinTopicScore))
			guard = guardrails.New(tg, cfg.Guardrails.MinMatchScore)
			slog.Info("Level 1 guardrail enabled", "corpus_topic", cfg.Guardrails.CorpusTopic)
		} else {
			guard = guardrails.New(nil, cfg.Guardrails.MinMatchScore)
		}

		if cfg.Guardrails.MinMatchScore > 0 {
			slog.Info("Level 2 guardrail enabled", "min_match_score", cfg.Guardrails.MinMatchScore)
		}

		// Initialize HyDE
		var hydeGen hydemod.Generator
		if cfg.Hyde.Enabled {
			hydeGen = hydemod.NewAnthropicGenerator(cfg.Hyde.Model, cfg.Hyde.SystemPrompt, cfg.Hyde.BaseURL)
			slog.Info("HyDE enabled", "model", cfg.Hyde.Model)
		} else {
			hydeGen = &hydemod.NoopGenerator{}
		}

		// Initialize reranker
		var reranker rerankmod.Reranker
		if cfg.Reranker.Enabled {
			reranker = rerankmod.NewClient(cfg.Reranker.Host)
			slog.Info("reranker enabled", "host", cfg.Reranker.Host)
		} else {
			reranker = &rerankmod.NoopReranker{}
		}

		vs := vectorstore.NewPostgresStore(store.Pool(), cfg.Embed.Dimension)

		// Search pipeline
		searchPipeline := search.NewPipeline(embedClient, vs, guard, hydeGen, reranker, search.PipelineConfig{
			Probes:            cfg.Search.Probes,
			RetrievalPoolSize: cfg.Search.RetrievalPoolSize,
			RRFConstant:       cfg.Search.RRFConstant,
			QueryPrefix:       cfg.Embed.QueryPrefix,
			PassagePrefix:     cfg.Embed.PassagePrefix,
		})

		// Register search tool
		tools.Register(mcpServer, authorizer, tools.NewSearchTool(searchPipeline))
		slog.Info("registered tool: search_documents")

		// Register ingest tool
		ingestPipeline := ingest.NewPipeline(vs, embedClient, &ingest.MarkdownChunker{}, ingest.PipelineConfig{
			ChunkSize:         cfg.Ingest.ChunkSize,
			BatchSize:         cfg.Ingest.BatchSize,
			MaxFileSize:       cfg.Ingest.MaxFileSizeBytes,
			AllowedDirs:       cfg.Ingest.AllowedDirs,
			AllowedExtensions: cfg.Ingest.AllowedExtensions,
			ExcludedDirs:      cfg.Ingest.ExcludedDirs,
			PassagePrefix:     cfg.Embed.PassagePrefix,
		})
		tools.Register(mcpServer, authorizer, tools.NewIngestTool(ingestPipeline, cfg.Embed.Dimension))
		slog.Info("registered tool: ingest_documents")

	} else {
		if cfg.Database.Engine == "mssql" {
			slog.Info("vector features disabled: engine is mssql")
		} else {
			slog.Info("vector features disabled: embed.enabled is false")
		}
	}

	// Register query tool (always available)
	var qs querystore.QueryStore
	switch cfg.Database.Engine {
	case "postgres":
		qs = querystore.NewPostgresQueryStore(store.Pool(), cfg.Query.MaxResponseSizeBytes)
	case "mssql":
		qs = querystore.NewMSSQLQueryStore(store.Pool(), cfg.Query.MaxResponseSizeBytes)
	}
	tools.Register(mcpServer, authorizer, tools.NewQueryTool(qs, cfg.Query.DefaultLimit, cfg.Query.MaxLimit, cfg.Query.TimeoutDuration))
	slog.Info("registered tool: query_data")

	// Startup logging (OBS-05)
	slog.Info("server starting",
		"version", version,
		"addr", ":"+cfg.Server.Port,
		"database_engine", cfg.Database.Engine,
		"auth_issuer", cfg.IssuerURL(),
		"vector_enabled", cfg.VectorEnabled(),
	)

	// Create HTTP server
	dsn := cfg.Database.URL
	if dsn == "" {
		dsn = os.Getenv("DATABASE_URL")
	}
	srv := server.New(server.Config{
		Port:            cfg.Server.Port,
		TLSCert:         cfg.Server.TLSCert,
		TLSKey:          cfg.Server.TLSKey,
		ShutdownTimeout: cfg.Server.ShutdownTimeoutDuration,
		DSN:             dsn,
	}, store, mcpServer, validator)

	// SIGHUP handler for config reload (CFG-05)
	sighupChan := make(chan os.Signal, 1)
	signal.Notify(sighupChan, syscall.SIGHUP)
	go func() {
		for range sighupChan {
			slog.Info("received SIGHUP, reloading config")
			newCfg, err := config.Load(configPath)
			if err != nil {
				slog.Error("config reload failed, keeping previous config", "error", err)
				continue
			}
			reloaded := config.ExtractReloadable(newCfg)
			changed := config.ApplyReload(cfg, reloaded)
			if len(changed) > 0 {
				slog.Info("config reloaded", "changed_sections", changed)
				setupLogging(cfg.Server.LogLevel)
			} else {
				slog.Info("config reloaded, no changes")
			}
		}
	}()

	// Graceful shutdown (MCP-07)
	shutdownChan := make(chan os.Signal, 1)
	signal.Notify(shutdownChan, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		sig := <-shutdownChan
		slog.Info("shutdown signal received", "signal", sig)
		if err := srv.Shutdown(context.Background()); err != nil {
			slog.Error("shutdown error", "error", err)
		}
	}()

	// Start serving
	if cfg.Server.TLSCert != "" {
		if err := srv.StartTLS(cfg.Server.TLSCert, cfg.Server.TLSKey); err != nil {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	} else {
		if err := srv.Start(); err != nil {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}
}

// stringSlice implements flag.Value for repeatable string flags.
type stringSlice []string

func (s *stringSlice) String() string { return strings.Join(*s, ", ") }
func (s *stringSlice) Set(v string) error {
	*s = append(*s, v)
	return nil
}

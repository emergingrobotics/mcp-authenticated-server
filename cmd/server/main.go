package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/emergingrobotics/mcp-authenticated-server/internal/auth"
	"github.com/emergingrobotics/mcp-authenticated-server/internal/cli"
	"github.com/emergingrobotics/mcp-authenticated-server/internal/config"
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

	cfg := cli.LoadConfig(*configPath)
	cli.SetupLogging(cfg.Server.LogLevel)

	ctx := context.Background()

	// Connect to database
	store := cli.CreateStore(cfg)
	cli.ConnectDB(ctx, cfg, store)
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
			newCfg, err := config.Load(*configPath)
			if err != nil {
				slog.Error("config reload failed, keeping previous config", "error", err)
				continue
			}
			reloaded := config.ExtractReloadable(newCfg)
			changed := config.ApplyReload(cfg, reloaded)
			if len(changed) > 0 {
				slog.Info("config reloaded", "changed_sections", changed)
				cli.SetupLogging(cfg.Server.LogLevel)
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

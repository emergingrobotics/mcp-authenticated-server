package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/emergingrobotics/mcp-authenticated-server/internal/cli"
	"github.com/emergingrobotics/mcp-authenticated-server/internal/embed"
	"github.com/emergingrobotics/mcp-authenticated-server/internal/ingest"
	"github.com/emergingrobotics/mcp-authenticated-server/internal/vectorstore"
)

func main() {
	configPath := flag.String("config", "config.toml", "path to config file")
	dirs := &stringSlice{}
	flag.Var(dirs, "dir", "directory to ingest (repeatable)")
	drop := flag.Bool("drop", false, "drop and recreate tables")
	dryRun := flag.Bool("dry-run", false, "show what would be ingested")
	verbose := flag.Bool("verbose", false, "verbose output")
	flag.Parse()

	cfg := cli.LoadConfig(*configPath)
	cli.SetupLogging(cfg.Server.LogLevel)

	if *verbose {
		cli.SetupLogging("debug")
	}

	if len(*dirs) == 0 {
		if cfg.Ingest.DefaultDir != "" {
			*dirs = stringSlice{cfg.Ingest.DefaultDir}
		} else {
			fmt.Fprintln(os.Stderr, "at least one --dir is required (or set ingest.default_dir in config)")
			os.Exit(1)
		}
	}

	ctx := context.Background()
	store := cli.CreateStore(cfg)
	cli.ConnectDB(ctx, cfg, store)
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

type stringSlice []string

func (s *stringSlice) String() string { return strings.Join(*s, ", ") }
func (s *stringSlice) Set(v string) error {
	*s = append(*s, v)
	return nil
}

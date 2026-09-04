package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"

	"github.com/jabronicrab/go-dependable-agent-mcp/internal/catalog"
	"github.com/jabronicrab/go-dependable-agent-mcp/internal/mcpserver"
	"github.com/jabronicrab/go-dependable-agent-mcp/internal/preflight"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(parent context.Context, args []string) error {
	flags := flag.NewFlagSet("agent-dependency-preflight", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)

	configPath := flags.String(
		"config",
		"",
		"path to the operator-owned dependency catalog JSON file",
	)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *configPath == "" {
		return errors.New("-config is required")
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %v", flags.Args())
	}

	dependencyCatalog, err := loadCatalog(*configPath)
	if err != nil {
		return err
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelWarn,
	}))

	service := preflight.NewService(
		dependencyCatalog,
		preflight.NewChecker(),
	)
	server := mcpserver.New(service, logger)

	ctx, stop := signal.NotifyContext(parent, os.Interrupt)
	defer stop()

	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil &&
		!errors.Is(err, context.Canceled) {
		return fmt.Errorf("run MCP server: %w", err)
	}

	return nil
}

func loadCatalog(path string) (*catalog.Catalog, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open dependency catalog: %w", err)
	}
	defer file.Close()

	dependencyCatalog, err := catalog.Load(file)
	if err != nil {
		return nil, fmt.Errorf("load dependency catalog: %w", err)
	}

	return dependencyCatalog, nil
}

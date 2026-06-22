//go:build introspect

// Command introspect issues a GraphQL introspection query against the
// monday.com API and writes the result as an SDL file to schema/monday.graphql.
//
// Usage:
//
//	go run -tags=introspect ./tools/introspect
//
// The MONDAY_API_TOKEN environment variable must be set.
package main

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	apischema "github.com/mondaycom/mcli/internal/api/schema"
	"github.com/mondaycom/mcli/internal/config"
)

const (
	endpoint    = "https://api.monday.com/v2"
	outFile     = "schema/monday.graphql"
	httpTimeout = 60 * time.Second
)

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "introspect: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	token := os.Getenv("MONDAY_API_TOKEN")
	if token == "" {
		return fmt.Errorf("MONDAY_API_TOKEN is not set; export it before running make schema")
	}

	apiVersion := cmp.Or(os.Getenv("MONDAY_API_VERSION"), "2026-07")

	fmt.Fprintln(os.Stderr, "introspect: fetching Monday.com GraphQL schema…")

	sdl, err := apischema.FetchSchema(ctx, config.APIToken(token), endpoint, apiVersion)
	if err != nil {
		return fmt.Errorf("fetch schema: %w", err)
	}

	// outFile is written relative to the current working directory.
	// make schema is always invoked from the repo root.
	outPath := outFile

	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("create schema dir: %w", err)
	}
	if err := os.WriteFile(outPath, []byte(sdl), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", outPath, err)
	}

	lines := strings.Count(sdl, "\n")
	fmt.Fprintf(os.Stderr, "introspect: wrote %s (%d lines)\n", outPath, lines)
	return nil
}

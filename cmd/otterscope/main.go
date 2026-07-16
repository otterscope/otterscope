// Command otterscope is a lightweight, self-hosted observability and evals
// server for AI agents. See https://github.com/otterscope/otterscope.
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
	"time"

	"github.com/otterscope/otterscope/internal/pricing"
	"github.com/otterscope/otterscope/internal/sample"
	"github.com/otterscope/otterscope/internal/server"
	"github.com/otterscope/otterscope/internal/store"
)

var version = "dev" // set via -ldflags at release time

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "serve":
		if err := serve(os.Args[2:]); err != nil {
			slog.Error("fatal", "err", err)
			os.Exit(1)
		}
	case "sample":
		if err := sampleCmd(os.Args[2:]); err != nil {
			slog.Error("fatal", "err", err)
			os.Exit(1)
		}
	case "project":
		if err := projectCmd(os.Args[2:]); err != nil {
			slog.Error("fatal", "err", err)
			os.Exit(1)
		}
	case "version":
		fmt.Println(version)
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage: otterscope <command>

commands:
  serve                   start the ingest + UI server
  project add <name>      create a project and print its ingest key
  project list            list projects and their ingest keys
  sample                  seed demo data (services, runs, assertions)
  version                 print version`)
}

func serve(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	dbPath := fs.String("db", "otterscope.db", "path to the SQLite database file")
	uiAddr := fs.String("listen", ":8317", "address for the web UI and API")
	otlpAddr := fs.String("otlp", ":4318", "address for the OTLP/HTTP receiver")
	pricingPath := fs.String("pricing", "", "JSON file of pricing overrides, merged over built-in rates")
	retention := fs.Duration("retention", 0, "delete runs older than this (e.g. 720h = 30 days); 0 keeps everything")
	if err := fs.Parse(args); err != nil {
		return err
	}

	prices := pricing.Default()
	if *pricingPath != "" {
		data, err := os.ReadFile(*pricingPath)
		if err != nil {
			return fmt.Errorf("read pricing overrides: %w", err)
		}
		if err := prices.MergeJSON(data); err != nil {
			return err
		}
		slog.Info("pricing overrides loaded", "file", *pricingPath)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, err := store.Open(ctx, *dbPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer st.Close()
	slog.Info("store ready", "db", *dbPath)

	if *retention > 0 {
		go sweepLoop(ctx, st, *retention)
		slog.Info("retention sweep enabled", "keep", *retention)
	}

	srv := server.New(st, prices, version)
	return srv.Run(ctx, *uiAddr, *otlpAddr)
}

// sweepLoop deletes data older than keep, hourly (and once at startup).
func sweepLoop(ctx context.Context, st *store.Store, keep time.Duration) {
	tick := time.NewTicker(time.Hour)
	defer tick.Stop()
	for {
		if n, err := st.Sweep(ctx, time.Now().Add(-keep)); err != nil {
			slog.Error("retention sweep failed", "err", err)
		} else if n > 0 {
			slog.Info("retention sweep", "runsDeleted", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func sampleCmd(args []string) error {
	fs := flag.NewFlagSet("sample", flag.ExitOnError)
	dbPath := fs.String("db", "otterscope.db", "path to the SQLite database file")
	n := fs.Int("runs", 60, "number of sample runs to create")
	if err := fs.Parse(args); err != nil {
		return err
	}
	ctx := context.Background()
	st, err := store.Open(ctx, *dbPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer st.Close()
	if err := sample.Seed(ctx, st, *n); err != nil {
		return err
	}
	fmt.Printf("seeded %d sample runs into %s\n", *n, *dbPath)
	return nil
}

func projectCmd(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: otterscope project [add <name> | list] [-db path]")
	}
	sub := args[0]
	fs := flag.NewFlagSet("project", flag.ExitOnError)
	dbPath := fs.String("db", "otterscope.db", "path to the SQLite database file")
	rest := args[1:]
	var name string
	if sub == "add" {
		if len(rest) < 1 || strings.HasPrefix(rest[0], "-") {
			return fmt.Errorf("usage: otterscope project add <name> [-db path]")
		}
		name, rest = rest[0], rest[1:]
	}
	if err := fs.Parse(rest); err != nil {
		return err
	}

	ctx := context.Background()
	st, err := store.Open(ctx, *dbPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer st.Close()

	switch sub {
	case "add":
		p, err := st.CreateProject(ctx, name)
		if err != nil {
			return err
		}
		fmt.Printf("project %q created\ningest key: %s\n\nagents send it via:\n  export OTEL_EXPORTER_OTLP_HEADERS=\"Authorization=Bearer %s\"\n", p.Name, p.IngestKey, p.IngestKey)
		return nil
	case "list":
		projects, err := st.ListProjects(ctx)
		if err != nil {
			return err
		}
		for _, p := range projects {
			key := p.IngestKey
			if key == "" {
				key = "(none — open ingest)"
			}
			fmt.Printf("%-20s %s\n", p.Name, key)
		}
		return nil
	default:
		return fmt.Errorf("unknown subcommand %q", sub)
	}
}

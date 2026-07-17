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

	"github.com/otterscope/otterscope/internal/evals"
	"github.com/otterscope/otterscope/internal/ingest"
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
	case "renormalize":
		if err := renormalizeCmd(os.Args[2:]); err != nil {
			slog.Error("fatal", "err", err)
			os.Exit(1)
		}
	case "backup":
		if err := backupCmd(os.Args[2:]); err != nil {
			slog.Error("fatal", "err", err)
			os.Exit(1)
		}
	case "token":
		if err := tokenCmd(os.Args[2:]); err != nil {
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
  token add <name>        create a read-API token and print it
  token list              list read-API tokens
  backup -o <file>        write a consistent snapshot of the database
  renormalize             replay stored raw batches through the current
                          normalizer and pricing table (backfill after upgrades)
  version                 print version`)
}

func serve(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	dbPath := fs.String("db", "otterscope.db", "path to the SQLite database file")
	uiAddr := fs.String("listen", "127.0.0.1:8317", "address for the web UI and API (loopback by default; use :8317 to expose)")
	otlpAddr := fs.String("otlp", "127.0.0.1:4318", "address for the OTLP/HTTP receiver (loopback by default; use :4318 to expose)")
	pricingPath := fs.String("pricing", "", "JSON file of pricing overrides, merged over built-in rates")
	retention := fs.Duration("retention", 0, "delete runs older than this (e.g. 720h = 30 days); 0 keeps everything")
	judgeURL := fs.String("judge-url", "https://api.openai.com/v1", "OpenAI-compatible endpoint for llm_judge assertions")
	alertInterval := fs.Duration("alert-interval", time.Minute, "how often to evaluate alert rules; 0 disables alerting")
	readAuth := fs.Bool("read-auth", false, "require a read token (Bearer) on the API + MCP; create tokens with 'otterscope token add'")
	ingestRate := fs.Float64("ingest-rate", 0, "max OTLP batches/sec per ingest key (0 = unlimited)")
	ingestBurst := fs.Float64("ingest-burst", 0, "ingest burst allowance per key (0 = 2x rate)")
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

	judgeKey := os.Getenv("OTTERSCOPE_JUDGE_KEY")
	if judgeKey == "" {
		judgeKey = os.Getenv("OPENAI_API_KEY")
	}
	judge := evals.Endpoint{BaseURL: *judgeURL, Key: judgeKey}

	srv := server.New(st, prices, judge, *alertInterval, *readAuth, *ingestRate, *ingestBurst, version)
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

func renormalizeCmd(args []string) error {
	fs := flag.NewFlagSet("renormalize", flag.ExitOnError)
	dbPath := fs.String("db", "otterscope.db", "path to the SQLite database file")
	pricingPath := fs.String("pricing", "", "JSON file of pricing overrides, merged over built-in rates")
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
	}
	ctx := context.Background()
	st, err := store.Open(ctx, *dbPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer st.Close()
	n, err := ingest.Renormalize(ctx, st, prices)
	if err != nil {
		return err
	}
	fmt.Printf("replayed %d raw batches through the current normalizer and pricing table\n", n)
	return nil
}

func backupCmd(args []string) error {
	fs := flag.NewFlagSet("backup", flag.ExitOnError)
	dbPath := fs.String("db", "otterscope.db", "path to the SQLite database file")
	out := fs.String("o", "", "destination file for the backup (required; must not exist)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *out == "" {
		return fmt.Errorf("usage: otterscope backup -o <file> [-db path]")
	}
	ctx := context.Background()
	st, err := store.Open(ctx, *dbPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer st.Close()
	if err := st.Backup(ctx, *out); err != nil {
		return err
	}
	fmt.Printf("backed up %s to %s\n", *dbPath, *out)
	return nil
}

func tokenCmd(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: otterscope token [add <name> | list] [-db path]")
	}
	sub := args[0]
	fs := flag.NewFlagSet("token", flag.ExitOnError)
	dbPath := fs.String("db", "otterscope.db", "path to the SQLite database file")
	rest := args[1:]
	var name string
	if sub == "add" {
		if len(rest) < 1 || strings.HasPrefix(rest[0], "-") {
			return fmt.Errorf("usage: otterscope token add <name> [-db path]")
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
		t, err := st.CreateReadToken(ctx, name)
		if err != nil {
			return err
		}
		fmt.Printf("read token %q created\ntoken: %s\n\nuse it as:\n  curl -H \"Authorization: Bearer %s\" http://localhost:8317/api/runs\n", t.Name, t.Token, t.Token)
		return nil
	case "list":
		toks, err := st.ListReadTokens(ctx)
		if err != nil {
			return err
		}
		for _, t := range toks {
			fmt.Printf("%-20s %s\n", t.Name, t.Token)
		}
		return nil
	default:
		return fmt.Errorf("unknown subcommand %q", sub)
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

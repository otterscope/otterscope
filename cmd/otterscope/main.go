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
	"syscall"

	"github.com/otterscope/otterscope/internal/pricing"
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
  serve     start the ingest + UI server
  version   print version`)
}

func serve(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	dbPath := fs.String("db", "otterscope.db", "path to the SQLite database file")
	uiAddr := fs.String("listen", ":8317", "address for the web UI and API")
	otlpAddr := fs.String("otlp", ":4318", "address for the OTLP/HTTP receiver")
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

	srv := server.New(st, prices, version)
	return srv.Run(ctx, *uiAddr, *otlpAddr)
}

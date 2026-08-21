package main

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/otterscope/otterscope/internal/config"
)

// Every subcommand must resolve its database the same way `serve` does.
// Before #97 they hardcoded "otterscope.db", so running one against a
// configured deployment silently created and operated on an empty database
// in the working directory — `backup` in particular exited 0 after
// snapshotting nothing.
func TestSubcommandsResolveDBFromConfigAndEnv(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "otterscope.json")
	if err := os.WriteFile(cfgPath, []byte(`{"db":"/from/config.db"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		args []string
		env  map[string]string
		want string
	}{
		{"built-in default", nil, nil, "otterscope.db"},
		{"env", nil, map[string]string{"OTTERSCOPE_DB": "/from/env.db"}, "/from/env.db"},
		{"config file", []string{"-config", cfgPath}, nil, "/from/config.db"},
		{"config file via env", nil, map[string]string{"OTTERSCOPE_CONFIG": cfgPath}, "/from/config.db"},
		{"env beats config file", []string{"-config", cfgPath},
			map[string]string{"OTTERSCOPE_DB": "/from/env.db"}, "/from/env.db"},
		{"flag beats env", []string{"-db", "/from/flag.db"},
			map[string]string{"OTTERSCOPE_DB": "/from/env.db"}, "/from/flag.db"},
		{"flag beats config file", []string{"-config", cfgPath, "-db", "/from/flag.db"},
			nil, "/from/flag.db"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			cfg, configPath, err := loadConfig(tc.args)
			if err != nil {
				t.Fatalf("loadConfig: %v", err)
			}
			fs := flag.NewFlagSet("test", flag.ContinueOnError)
			dbPath := dbFlags(fs, cfg, configPath)
			if err := fs.Parse(tc.args); err != nil {
				t.Fatalf("parse: %v", err)
			}
			if *dbPath != tc.want {
				t.Errorf("db = %q, want %q", *dbPath, tc.want)
			}
		})
	}
}

// -config must be accepted by every subcommand's flag set, not just serve's,
// or `backup -config x.json` fails with "flag provided but not defined".
func TestSubcommandsAcceptConfigFlag(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	dbFlags(fs, config.Default(), "")
	if fs.Lookup("config") == nil {
		t.Error("-config not registered")
	}
	if fs.Lookup("db") == nil {
		t.Error("-db not registered")
	}
}

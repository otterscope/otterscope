// Package config resolves serve configuration from (in increasing priority)
// built-in defaults, a JSON config file, and OTTERSCOPE_* environment
// variables. The caller then lets explicit CLI flags override the result, so
// the final precedence is flags > env > file > defaults.
package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all `serve` options. Durations are resolved to time.Duration.
type Config struct {
	DB            string
	Listen        string
	OTLP          string
	Pricing       string
	JudgeURL      string
	Retention     time.Duration
	AlertInterval time.Duration
	ReadAuth      bool
	IngestRate    float64
	IngestBurst   float64
}

// Default returns the built-in configuration.
func Default() Config {
	return Config{
		DB:            "otterscope.db",
		Listen:        "127.0.0.1:8317",
		OTLP:          "127.0.0.1:4318",
		JudgeURL:      "https://api.openai.com/v1",
		AlertInterval: time.Minute,
	}
}

// fileConfig mirrors the JSON schema. Pointer fields distinguish "absent"
// (leave the current value) from "set to zero".
type fileConfig struct {
	DB            *string  `json:"db"`
	Listen        *string  `json:"listen"`
	OTLP          *string  `json:"otlp"`
	Pricing       *string  `json:"pricing"`
	JudgeURL      *string  `json:"judgeUrl"`
	Retention     *string  `json:"retention"`
	AlertInterval *string  `json:"alertInterval"`
	ReadAuth      *bool    `json:"readAuth"`
	IngestRate    *float64 `json:"ingestRate"`
	IngestBurst   *float64 `json:"ingestBurst"`
}

// Load builds the effective config: defaults, then the JSON file at path (if
// non-empty), then OTTERSCOPE_* env vars via getenv.
func Load(path string, getenv func(string) string) (Config, error) {
	c := Default()
	if path != "" {
		if err := c.applyFile(path); err != nil {
			return Config{}, err
		}
	}
	if err := c.applyEnv(getenv); err != nil {
		return Config{}, err
	}
	return c, nil
}

func (c *Config) applyFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read config %s: %w", path, err)
	}
	var f fileConfig
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return fmt.Errorf("parse config %s: %w", path, err)
	}
	setStr(&c.DB, f.DB)
	setStr(&c.Listen, f.Listen)
	setStr(&c.OTLP, f.OTLP)
	setStr(&c.Pricing, f.Pricing)
	setStr(&c.JudgeURL, f.JudgeURL)
	if f.ReadAuth != nil {
		c.ReadAuth = *f.ReadAuth
	}
	if f.IngestRate != nil {
		c.IngestRate = *f.IngestRate
	}
	if f.IngestBurst != nil {
		c.IngestBurst = *f.IngestBurst
	}
	if f.Retention != nil {
		d, err := time.ParseDuration(*f.Retention)
		if err != nil {
			return fmt.Errorf("config retention: %w", err)
		}
		c.Retention = d
	}
	if f.AlertInterval != nil {
		d, err := time.ParseDuration(*f.AlertInterval)
		if err != nil {
			return fmt.Errorf("config alertInterval: %w", err)
		}
		c.AlertInterval = d
	}
	return nil
}

func (c *Config) applyEnv(getenv func(string) string) error {
	envStr(&c.DB, getenv, "OTTERSCOPE_DB")
	envStr(&c.Listen, getenv, "OTTERSCOPE_LISTEN")
	envStr(&c.OTLP, getenv, "OTTERSCOPE_OTLP")
	envStr(&c.Pricing, getenv, "OTTERSCOPE_PRICING")
	envStr(&c.JudgeURL, getenv, "OTTERSCOPE_JUDGE_URL")
	if v := getenv("OTTERSCOPE_READ_AUTH"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("OTTERSCOPE_READ_AUTH: %w", err)
		}
		c.ReadAuth = b
	}
	if err := envFloat(&c.IngestRate, getenv, "OTTERSCOPE_INGEST_RATE"); err != nil {
		return err
	}
	if err := envFloat(&c.IngestBurst, getenv, "OTTERSCOPE_INGEST_BURST"); err != nil {
		return err
	}
	if err := envDur(&c.Retention, getenv, "OTTERSCOPE_RETENTION"); err != nil {
		return err
	}
	return envDur(&c.AlertInterval, getenv, "OTTERSCOPE_ALERT_INTERVAL")
}

func setStr(dst *string, v *string) {
	if v != nil {
		*dst = *v
	}
}

func envStr(dst *string, getenv func(string) string, key string) {
	if v := getenv(key); v != "" {
		*dst = v
	}
}

func envFloat(dst *float64, getenv func(string) string, key string) error {
	v := getenv(key)
	if v == "" {
		return nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return fmt.Errorf("%s: %w", key, err)
	}
	*dst = f
	return nil
}

func envDur(dst *time.Duration, getenv func(string) string, key string) error {
	v := getenv(key)
	if v == "" {
		return nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fmt.Errorf("%s: %w", key, err)
	}
	*dst = d
	return nil
}

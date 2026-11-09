// Package config loads the server configuration from flags and environment
// variables. Flags take precedence over environment variables, which take
// precedence over the defaults.
package config

import (
	"errors"
	"flag"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// Defaults used when neither a flag nor an environment variable is set.
const (
	DefaultListenAddr = ":8080"
	DefaultDBPath     = "soliloquy.db"
	DefaultBaseURL    = "http://localhost:8080"
)

// Environment variables used as a fallback when a flag is not given.
const (
	EnvListenAddr = "SOLILOQUY_LISTEN_ADDR"
	EnvDBPath     = "SOLILOQUY_DB_PATH"
	EnvBaseURL    = "SOLILOQUY_BASE_URL"
	// EnvAllowAnyOrigin is for QA and debugging servers only.
	EnvAllowAnyOrigin = "SOLILOQUY_ALLOW_ANY_ORIGIN"
)

// Config is the runtime configuration of the server.
type Config struct {
	// ListenAddr is the address passed to http.Server, for example ":8080".
	ListenAddr string
	// DBPath is the path to the SQLite database file.
	DBPath string
	// BaseURL is the public base URL of the site. It is used to build
	// absolute links such as invites and password resets.
	BaseURL string
	// AllowAnyOrigin turns off the Origin check on POST requests, which is
	// the CSRF defence. It exists so QA and debugging servers work at any
	// address. Never set it on a real deployment.
	AllowAnyOrigin bool
}

// Load reads configuration from environment variables and command-line
// arguments. getenv is normally os.Getenv; it is a parameter so tests can
// supply their own environment.
func Load(args []string, getenv func(string) string) (Config, error) {
	cfg := Config{
		ListenAddr: envOrDefault(getenv, EnvListenAddr, DefaultListenAddr),
		DBPath:     envOrDefault(getenv, EnvDBPath, DefaultDBPath),
		BaseURL:    envOrDefault(getenv, EnvBaseURL, DefaultBaseURL),
	}

	if value := strings.TrimSpace(getenv(EnvAllowAnyOrigin)); value != "" {
		allow, err := strconv.ParseBool(value)
		if err != nil {
			return Config{}, fmt.Errorf("invalid %s %q: %w", EnvAllowAnyOrigin, value, err)
		}
		cfg.AllowAnyOrigin = allow
	}

	flags := flag.NewFlagSet("soliloquy", flag.ContinueOnError)
	flags.StringVar(&cfg.ListenAddr, "listen", cfg.ListenAddr, "listen address (env "+EnvListenAddr+")")
	flags.StringVar(&cfg.DBPath, "db", cfg.DBPath, "SQLite database file path (env "+EnvDBPath+")")
	flags.StringVar(&cfg.BaseURL, "base-url", cfg.BaseURL, "public base URL used to build links (env "+EnvBaseURL+")")

	flags.BoolVar(&cfg.AllowAnyOrigin, "allow-any-origin", cfg.AllowAnyOrigin, "accept POST requests from any Origin; disables CSRF protection, for QA only (env "+EnvAllowAnyOrigin+")")

	if err := flags.Parse(args); err != nil {
		return Config{}, err
	}
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func (c *Config) validate() error {
	c.ListenAddr = strings.TrimSpace(c.ListenAddr)
	if c.ListenAddr == "" {
		return errors.New("listen address must not be empty")
	}

	c.DBPath = strings.TrimSpace(c.DBPath)
	if c.DBPath == "" {
		return errors.New("database path must not be empty")
	}

	c.BaseURL = strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	base, err := url.Parse(c.BaseURL)
	if err != nil {
		return fmt.Errorf("invalid base URL %q: %w", c.BaseURL, err)
	}
	if base.Scheme != "http" && base.Scheme != "https" {
		return fmt.Errorf("invalid base URL %q: scheme must be http or https", c.BaseURL)
	}
	if base.Host == "" {
		return fmt.Errorf("invalid base URL %q: host must not be empty", c.BaseURL)
	}

	return nil
}

func envOrDefault(getenv func(string) string, key, fallback string) string {
	if value := strings.TrimSpace(getenv(key)); value != "" {
		return value
	}
	return fallback
}

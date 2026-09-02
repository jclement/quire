// Package config loads quire's runtime configuration from environment
// variables (QUIRE_*), falling back to <data>/.quire/config.yaml. Environment
// wins over file so Docker deployments can override without editing the vault.
package config

import (
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/goccy/go-yaml"
)

// AuthMode selects how requests are authenticated. See DESIGN.md "Auth modes".
type AuthMode string

const (
	// AuthNone trusts the machine boundary: every request is the singleton
	// user. Only allowed on loopback addresses — Load enforces this.
	AuthNone AuthMode = "none"
	// AuthPasskey is the real deployment mode: WebAuthn + sessions.
	AuthPasskey AuthMode = "passkey"
	// AuthTokenOnly disables browser login; bearer tokens only.
	AuthTokenOnly AuthMode = "token-only"
)

// DefaultBaseURL is what BaseURL falls back to. It is exported so serve can
// warn when a deployment listens somewhere else but never set it — every URL
// quire hands out (OAuth discovery, share links) would then point at a host
// nobody can reach.
const DefaultBaseURL = "http://localhost:8321"

// Config is the resolved runtime configuration.
type Config struct {
	DataDir  string   `yaml:"data_dir"`
	Addr     string   `yaml:"addr"`
	BaseURL  string   `yaml:"base_url"`
	AuthMode AuthMode `yaml:"auth_mode"`
	LogLevel string   `yaml:"log_level"`

	// Git keeps the vault in a local git repository with debounced
	// auto-commits (commit-only; quire never pushes or merges). On by
	// default — QUIRE_GIT=false to opt out.
	Git bool `yaml:"git"`
}

// VaultDir is where the user's markdown lives — the only tree quire treats as
// user-owned content.
func (c Config) VaultDir() string { return filepath.Join(c.DataDir, "vault") }

// StateDir holds app-owned state (index.db, auth.db, config.yaml) — never
// mixed into the vault.
func (c Config) StateDir() string { return filepath.Join(c.DataDir, ".quire") }

// Load resolves configuration: defaults ← config.yaml ← environment.
// It validates the auth-mode/listen-address invariant and creates the data
// directories so callers can rely on them existing.
func Load() (Config, error) {
	cfg := Config{
		DataDir:  "./data",
		Addr:     "127.0.0.1:8321",
		BaseURL:  DefaultBaseURL,
		AuthMode: AuthNone,
		LogLevel: "info",
		Git:      true,
	}

	if v := os.Getenv("QUIRE_DATA_DIR"); v != "" {
		cfg.DataDir = v
	}

	// The config file lives inside the data dir, so DataDir must be resolved
	// (env or default) before we can read it.
	path := filepath.Join(cfg.StateDir(), "config.yaml")
	if raw, err := os.ReadFile(path); err == nil {
		if err := yaml.Unmarshal(raw, &cfg); err != nil {
			return Config{}, fmt.Errorf("parsing %s: %w", path, err)
		}
	}

	applyEnv(&cfg)

	if cfg.AuthMode != AuthNone && cfg.AuthMode != AuthPasskey && cfg.AuthMode != AuthTokenOnly {
		return Config{}, fmt.Errorf("invalid QUIRE_AUTH_MODE %q (want none|passkey|token-only)", cfg.AuthMode)
	}

	// The whole safety story of auth mode "none" is this invariant: it can
	// only ever listen on loopback. There is deliberately no override flag.
	if cfg.AuthMode == AuthNone && !isLoopback(cfg.Addr) {
		return Config{}, fmt.Errorf(
			"auth mode \"none\" requires a loopback listen address, got %q — set QUIRE_AUTH_MODE=passkey (or token-only) to listen on %s",
			cfg.Addr, cfg.Addr)
	}

	for _, dir := range []string{cfg.VaultDir(), cfg.StateDir()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return Config{}, fmt.Errorf("creating %s: %w", dir, err)
		}
	}
	return cfg, nil
}

func applyEnv(cfg *Config) {
	if v := os.Getenv("QUIRE_ADDR"); v != "" {
		cfg.Addr = v
	}
	if v := os.Getenv("QUIRE_BASE_URL"); v != "" {
		cfg.BaseURL = v
	}
	if v := os.Getenv("QUIRE_AUTH_MODE"); v != "" {
		cfg.AuthMode = AuthMode(v)
	}
	if v := os.Getenv("QUIRE_LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}
	if v := os.Getenv("QUIRE_GIT"); v != "" {
		cfg.Git = v != "false" && v != "0"
	}
}

// isLoopback reports whether addr's host part is a loopback address or
// localhost. Unparseable addresses are treated as non-loopback: fail closed.
func isLoopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

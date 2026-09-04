// Package config loads process configuration from environment variables.
// It fails fast at startup: a missing required value is an error returned
// from Load, never a nil-checked-later panic deep in a request handler.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the full set of settings apps/server and apps/worker need at
// startup. Nothing in this struct is optional at the type level — every
// field has either a required source or a documented default, resolved
// once in Load.
type Config struct {
	Server        ServerConfig
	Database      DatabaseConfig
	Session       SessionConfig
	Argon2        Argon2Config
	Logging       LoggingConfig
	Observability ObservabilityConfig
}

type ServerConfig struct {
	// HTTPPort is the port apps/server listens on inside its container.
	// TLS termination is the deployment operator's responsibility
	// (docs/architecture.md §12) — this process always speaks plain HTTP.
	HTTPPort int
	// AllowedOrigins is the explicit CORS allow-list. Never a wildcard
	// when credentials are involved (brief §59).
	AllowedOrigins []string
	// ShutdownTimeout bounds how long graceful shutdown waits for
	// in-flight requests to finish before forcing close.
	ShutdownTimeout time.Duration
	// WebDistDir is where apps/web's built static assets live, if at all
	// — server.Dockerfile's webbuild stage copies apps/web/dist here.
	// httpx.MountSPA no-ops when the directory doesn't exist (local `go
	// run`, `-migrate`), so this is safe to leave at its default anywhere
	// that isn't the Docker image.
	WebDistDir string
}

type DatabaseConfig struct {
	// DSN is a full PostgreSQL connection string. Required; no default,
	// since a wrong-but-present default (e.g. localhost) is more dangerous
	// than a startup failure demanding an explicit value.
	DSN string
	// MaxConns bounds the pgxpool connection pool size.
	MaxConns int32
	// AutoMigrate, if true, runs pending migrations at process startup.
	// Intended for self-hosted/CasaOS single-instance deployments; a
	// horizontally-scaled managed-cloud deployment should run migrations
	// as a separate release step and leave this false to avoid N
	// replicas racing to migrate simultaneously.
	AutoMigrate bool
}

type SessionConfig struct {
	// CookieName is the session cookie's name.
	CookieName string
	// Secure controls the cookie's Secure flag. Must be true in any
	// deployment reachable over the network; false is only acceptable for
	// local plain-HTTP development.
	Secure bool
	// IdleTimeout logs a session out after this long with no activity.
	IdleTimeout time.Duration
	// AbsoluteTimeout logs a session out this long after login,
	// regardless of activity.
	AbsoluteTimeout time.Duration
}

// Argon2Config holds the password-hashing parameters. Defaults here are
// the values benchmarked and recorded in docs/adr/0001-argon2id-parameters.md
// — see that ADR before changing them. Overridable via environment so a
// deployment on materially different hardware can re-tune without a code
// change, but the default must remain safe on modest hardware.
type Argon2Config struct {
	MemoryKiB   uint32
	Iterations  uint32
	Parallelism uint8
	SaltLength  uint32
	KeyLength   uint32
}

type LoggingConfig struct {
	// Level is one of debug, info, warn, error.
	Level string
}

type ObservabilityConfig struct {
	// OTLPEndpoint, if empty, falls back to a stdout trace exporter
	// (suitable for local/dev). Set to enable real OTLP export.
	OTLPEndpoint string
	// ServiceName identifies this process in traces/metrics.
	ServiceName string
}

// Load reads configuration from the process environment. It returns an
// error describing every missing required variable at once (not just the
// first one found), so a misconfigured deployment gets one useful error
// message instead of a fix-and-retry loop.
func Load() (Config, error) {
	var errs []string
	requireString := func(key string) string {
		v := os.Getenv(key)
		if v == "" {
			errs = append(errs, fmt.Sprintf("%s is required", key))
		}
		return v
	}
	optString := func(key, def string) string {
		if v := os.Getenv(key); v != "" {
			return v
		}
		return def
	}
	optInt := func(key string, def int) int {
		v := os.Getenv(key)
		if v == "" {
			return def
		}
		n, err := strconv.Atoi(v)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s must be an integer, got %q", key, v))
			return def
		}
		return n
	}
	optBool := func(key string, def bool) bool {
		v := os.Getenv(key)
		if v == "" {
			return def
		}
		b, err := strconv.ParseBool(v)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s must be a boolean, got %q", key, v))
			return def
		}
		return b
	}
	optDuration := func(key string, def time.Duration) time.Duration {
		v := os.Getenv(key)
		if v == "" {
			return def
		}
		d, err := time.ParseDuration(v)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s must be a duration (e.g. \"30m\"), got %q", key, v))
			return def
		}
		return d
	}

	cfg := Config{
		Server: ServerConfig{
			HTTPPort:        optInt("HTTP_PORT", 8080),
			AllowedOrigins:  splitCSV(optString("CORS_ALLOWED_ORIGINS", "")),
			ShutdownTimeout: optDuration("SHUTDOWN_TIMEOUT", 15*time.Second),
			WebDistDir:      optString("WEB_DIST_DIR", "/app/web"),
		},
		Database: DatabaseConfig{
			DSN:         requireString("DATABASE_DSN"),
			MaxConns:    int32(optInt("DATABASE_MAX_CONNS", 20)),
			AutoMigrate: optBool("DATABASE_AUTO_MIGRATE", false),
		},
		Session: SessionConfig{
			CookieName:      optString("SESSION_COOKIE_NAME", "bp_session"),
			Secure:          optBool("SESSION_COOKIE_SECURE", true),
			IdleTimeout:     optDuration("SESSION_IDLE_TIMEOUT", 30*time.Minute),
			AbsoluteTimeout: optDuration("SESSION_ABSOLUTE_TIMEOUT", 12*time.Hour),
		},
		Argon2: Argon2Config{
			// Defaults match docs/adr/0001-argon2id-parameters.md.
			// Do not change these without updating that ADR and
			// re-benchmarking.
			MemoryKiB:   uint32(optInt("ARGON2_MEMORY_KIB", 65536)),
			Iterations:  uint32(optInt("ARGON2_ITERATIONS", 3)),
			Parallelism: uint8(optInt("ARGON2_PARALLELISM", 2)),
			SaltLength:  uint32(optInt("ARGON2_SALT_LENGTH", 16)),
			KeyLength:   uint32(optInt("ARGON2_KEY_LENGTH", 32)),
		},
		Logging: LoggingConfig{
			Level: optString("LOG_LEVEL", "info"),
		},
		Observability: ObservabilityConfig{
			OTLPEndpoint: optString("OTEL_EXPORTER_OTLP_ENDPOINT", ""),
			ServiceName:  optString("OTEL_SERVICE_NAME", "billing-server"),
		},
	}

	if cfg.Argon2.MemoryKiB < 19*1024 {
		errs = append(errs, fmt.Sprintf(
			"ARGON2_MEMORY_KIB (%d) is below the brief's minimum floor of 19456 (19 MiB)",
			cfg.Argon2.MemoryKiB))
	}
	if cfg.Argon2.Iterations < 2 {
		errs = append(errs, fmt.Sprintf(
			"ARGON2_ITERATIONS (%d) is below the brief's minimum floor of 2", cfg.Argon2.Iterations))
	}

	if len(errs) > 0 {
		return Config{}, fmt.Errorf("config: %s", strings.Join(errs, "; "))
	}
	return cfg, nil
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

package config

import (
	"os"
	"testing"
)

func clearEnv(t *testing.T, keys ...string) {
	t.Helper()
	for _, k := range keys {
		old, had := os.LookupEnv(k)
		os.Unsetenv(k)
		t.Cleanup(func() {
			if had {
				os.Setenv(k, old)
			}
		})
	}
}

var allKeys = []string{
	"HTTP_PORT", "CORS_ALLOWED_ORIGINS", "SHUTDOWN_TIMEOUT",
	"DATABASE_DSN", "DATABASE_MAX_CONNS", "DATABASE_AUTO_MIGRATE",
	"SESSION_COOKIE_NAME", "SESSION_COOKIE_SECURE", "SESSION_IDLE_TIMEOUT", "SESSION_ABSOLUTE_TIMEOUT",
	"ARGON2_MEMORY_KIB", "ARGON2_ITERATIONS", "ARGON2_PARALLELISM", "ARGON2_SALT_LENGTH", "ARGON2_KEY_LENGTH",
	"LOG_LEVEL", "OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_SERVICE_NAME",
}

func TestLoadMissingRequiredFailsFast(t *testing.T) {
	clearEnv(t, allKeys...)
	_, err := Load()
	if err == nil {
		t.Fatal("expected error when DATABASE_DSN is unset, got nil")
	}
}

func TestLoadDefaultsWithMinimalEnv(t *testing.T) {
	clearEnv(t, allKeys...)
	os.Setenv("DATABASE_DSN", "postgres://user:pass@localhost:5432/billing")
	t.Cleanup(func() { os.Unsetenv("DATABASE_DSN") })

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.HTTPPort != 8080 {
		t.Errorf("HTTPPort = %d, want 8080", cfg.Server.HTTPPort)
	}
	if cfg.Argon2.MemoryKiB != 65536 {
		t.Errorf("Argon2.MemoryKiB = %d, want 65536", cfg.Argon2.MemoryKiB)
	}
	if !cfg.Session.Secure {
		t.Error("Session.Secure should default true")
	}
}

func TestLoadRejectsArgon2BelowFloor(t *testing.T) {
	clearEnv(t, allKeys...)
	os.Setenv("DATABASE_DSN", "postgres://user:pass@localhost:5432/billing")
	os.Setenv("ARGON2_MEMORY_KIB", "1024")
	t.Cleanup(func() {
		os.Unsetenv("DATABASE_DSN")
		os.Unsetenv("ARGON2_MEMORY_KIB")
	})

	if _, err := Load(); err == nil {
		t.Fatal("expected error for below-floor Argon2 memory, got nil")
	}
}

func TestSplitCSV(t *testing.T) {
	got := splitCSV(" https://a.example , https://b.example ,,")
	want := []string{"https://a.example", "https://b.example"}
	if len(got) != len(want) {
		t.Fatalf("splitCSV = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("splitCSV[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

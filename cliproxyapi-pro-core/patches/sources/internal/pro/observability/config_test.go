package observability

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigForPathResolvesSharedDatabasePrecedence(t *testing.T) {
	t.Setenv("USAGE_DB_PATH", "")
	t.Setenv("USAGE_DATA_DIR", "")
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if got, want := LoadConfigForPath(configPath).DBPath, filepath.Join(filepath.Dir(configPath), "usage", "usage.sqlite"); got != want {
		t.Fatalf("config-relative DB path = %q, want %q", got, want)
	}
	if got := LoadConfigForPath("").DBPath; got != "/CLIProxyAPI/usage/usage.sqlite" {
		t.Fatalf("legacy default DB path = %q", got)
	}

	t.Setenv("USAGE_DATA_DIR", filepath.Join(t.TempDir(), "data"))
	if got, want := LoadConfigForPath(configPath).DBPath, filepath.Join(os.Getenv("USAGE_DATA_DIR"), "usage.sqlite"); got != want {
		t.Fatalf("data-dir DB path = %q, want %q", got, want)
	}
	t.Setenv("USAGE_DB_PATH", filepath.Join(t.TempDir(), "explicit.sqlite"))
	if got, want := LoadConfigForPath(configPath).DBPath, os.Getenv("USAGE_DB_PATH"); got != want {
		t.Fatalf("explicit DB path = %q, want %q", got, want)
	}
}

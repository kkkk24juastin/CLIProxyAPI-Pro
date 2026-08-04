package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveConfigPreserveCommentsUpdateExistingScalarsRejectsMissingPathWithoutWriting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	original := "# keep\nusage-statistics-enabled: false\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	missing, err := SaveConfigPreserveCommentsUpdateExistingScalars(path, []ExistingScalarUpdate{
		{Path: []string{"usage-statistics-enabled"}, Value: true},
		{Path: []string{"routing", "strategy"}, Value: "fill-first"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) != 1 || missing[0] != "routing.strategy" {
		t.Fatalf("missing = %#v", missing)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != original {
		t.Fatalf("config changed despite missing path:\n%s", got)
	}
}

func TestSaveConfigPreserveCommentsUpdateExistingScalarsOnlyChangesExistingValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("# keep\nusage-statistics-enabled: false\nrouting:\n  strategy: round-robin\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	missing, err := SaveConfigPreserveCommentsUpdateExistingScalars(path, []ExistingScalarUpdate{
		{Path: []string{"usage-statistics-enabled"}, Value: true},
		{Path: []string{"routing", "strategy"}, Value: "fill-first"},
	})
	if err != nil || len(missing) != 0 {
		t.Fatalf("update = %#v, %v", missing, err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	if !strings.Contains(text, "# keep") || !strings.Contains(text, "usage-statistics-enabled: true") || !strings.Contains(text, "strategy: fill-first") {
		t.Fatalf("unexpected config:\n%s", text)
	}
	if strings.Contains(text, "credential-concurrency") || strings.Contains(text, "ws-auth") {
		t.Fatalf("unexpected default keys added:\n%s", text)
	}
}

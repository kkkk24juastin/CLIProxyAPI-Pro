package embeddedusage

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPluginStorageMigrationAdoptsLegacyDatabaseInPlace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.sqlite")
	legacy := openStoreAt(t, path)
	if _, err := legacy.db.Exec(`insert into dead_letter_events(payload, error, created_at_ms) values('legacy', 'test', 1)`); err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cfg := LoadConfig()
	cfg.DBPath = path
	cfg.LegacyDBPath = path
	service, err := StartWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("StartWithConfig() error = %v", err)
	}
	t.Cleanup(cancel)
	var payload string
	if err = service.store.db.QueryRow(`select payload from dead_letter_events limit 1`).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if payload != "legacy" {
		t.Fatalf("payload = %q, want legacy", payload)
	}
	assertPluginStorageOwner(t, service.store, path, "adopted-in-place")
}

func TestPluginStorageMigrationCopiesLegacyDatabaseBeforeStarting(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "legacy.sqlite")
	target := filepath.Join(root, "plugin", "usage.sqlite")
	legacy := openStoreAt(t, source)
	if _, err := legacy.db.Exec(`insert into dead_letter_events(payload, error, created_at_ms) values('copied', 'test', 1)`); err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}

	migration, err := preparePluginStorage(context.Background(), Config{DBPath: target, LegacyDBPath: source})
	if err != nil {
		t.Fatalf("preparePluginStorage() error = %v", err)
	}
	if migration.Mode != "copied" {
		t.Fatalf("migration mode = %q, want copied", migration.Mode)
	}
	store := openStoreAt(t, target)
	if err = store.completePluginStorageMigration(context.Background(), migration); err != nil {
		t.Fatal(err)
	}
	var payload string
	if err = store.db.QueryRow(`select payload from dead_letter_events limit 1`).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if payload != "copied" {
		t.Fatalf("payload = %q, want copied", payload)
	}
	assertPluginStorageOwner(t, store, target, "copied")
}

func TestPluginStorageMigrationIsIdempotent(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "legacy.sqlite")
	target := filepath.Join(root, "plugin.sqlite")
	legacy := openStoreAt(t, source)
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}
	first, err := preparePluginStorage(context.Background(), Config{DBPath: target, LegacyDBPath: source})
	if err != nil {
		t.Fatal(err)
	}
	store := openStoreAt(t, target)
	first.CompletedAtMS = 123456
	if err = store.completePluginStorageMigration(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := preparePluginStorage(context.Background(), Config{DBPath: target, LegacyDBPath: source})
	if err != nil {
		t.Fatal(err)
	}
	if second != first {
		t.Fatalf("second migration = %#v, want original marker %#v", second, first)
	}
}

func TestPluginStorageMigrationInPlaceIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.sqlite")
	store := openStoreAt(t, path)
	first, err := preparePluginStorage(context.Background(), Config{DBPath: path, LegacyDBPath: path})
	if err != nil || first.Mode != "adopted-in-place" {
		t.Fatalf("first migration = %#v, %v", first, err)
	}
	first.CompletedAtMS = 234567
	if err = store.completePluginStorageMigration(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := preparePluginStorage(context.Background(), Config{DBPath: path, LegacyDBPath: path})
	if err != nil || second != first {
		t.Fatalf("second migration = %#v, %v; want original marker %#v", second, err, first)
	}
}

func TestPluginStorageMigrationRestartPreservesAuditRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.sqlite")
	cfg := LoadConfig()
	cfg.DBPath = path
	cfg.LegacyDBPath = path
	ctx, cancel := context.WithCancel(context.Background())
	first, err := StartWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	want := first.StorageMigration()
	cancel()
	first.Wait()

	ctx, cancel = context.WithCancel(context.Background())
	second, err := StartWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	got := second.StorageMigration()
	cancel()
	second.Wait()
	if got != want {
		t.Fatalf("migration after restart = %#v, want %#v", got, want)
	}
}

func TestPluginStorageMigrationFollowsMovedOwnedDatabase(t *testing.T) {
	root := t.TempDir()
	original := filepath.Join(root, "original.sqlite")
	moved := filepath.Join(root, "moved.sqlite")
	store := openStoreAt(t, original)
	want := PluginStorageMigration{
		Version: pluginStorageMigrationVersion, Owner: pluginStorageOwnerPlugin,
		Mode: "copied", SourcePath: filepath.Join(root, "legacy.sqlite"),
		TargetPath: original, CompletedAtMS: 345678,
	}
	if err := store.completePluginStorageMigration(context.Background(), want); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(original, moved); err != nil {
		t.Fatal(err)
	}
	got, err := preparePluginStorage(context.Background(), Config{DBPath: moved, LegacyDBPath: moved})
	if err != nil {
		t.Fatal(err)
	}
	if got.Mode != want.Mode || got.SourcePath != want.SourcePath || got.CompletedAtMS != want.CompletedAtMS || got.TargetPath != moved {
		t.Fatalf("moved migration = %#v, want original audit with target %q", got, moved)
	}
	ctx, cancel := context.WithCancel(context.Background())
	service, err := StartWithConfig(ctx, Config{DBPath: moved, LegacyDBPath: moved, BatchSize: 10, PollInterval: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	service.Wait()
	if _, err = os.Stat(original); !os.IsNotExist(err) {
		t.Fatalf("service reopened stale marker target: %v", err)
	}
}

func TestPluginStorageMigrationRejectsAmbiguousDatabases(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "legacy.sqlite")
	target := filepath.Join(root, "plugin.sqlite")
	for _, path := range []string{source, target} {
		store := openStoreAt(t, path)
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := preparePluginStorage(context.Background(), Config{DBPath: target, LegacyDBPath: source}); err == nil {
		t.Fatal("preparePluginStorage() unexpectedly accepted two unowned databases")
	}
}

func TestPluginStorageMigrationTreatsSymlinkedPathsAsOneDatabase(t *testing.T) {
	root := t.TempDir()
	realPath := filepath.Join(root, "usage.sqlite")
	linkPath := filepath.Join(root, "usage-link.sqlite")
	store := openStoreAt(t, realPath)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realPath, linkPath); err != nil {
		t.Fatal(err)
	}
	migration, err := preparePluginStorage(context.Background(), Config{DBPath: linkPath, LegacyDBPath: realPath})
	if err != nil || migration.Mode != "adopted-in-place" {
		t.Fatalf("migration = %#v, %v", migration, err)
	}
}

func openStoreAt(t *testing.T, path string) *Store {
	t.Helper()
	store, err := OpenStore(path)
	if err != nil {
		t.Fatalf("OpenStore(%q) error = %v", path, err)
	}
	return store
}

func assertPluginStorageOwner(t *testing.T, store *Store, path, wantMode string) {
	t.Helper()
	var raw string
	if err := store.db.QueryRow(`select settings_json from pro_settings where namespace = ?`, pluginStorageOwnerNamespace).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if raw == "" || !strings.Contains(raw, `"mode":"`+wantMode+`"`) {
		t.Fatalf("storage migration marker = %s, want mode %q", raw, wantMode)
	}
	owned, err := pluginStorageOwned(context.Background(), path)
	if err != nil || !owned {
		t.Fatalf("pluginStorageOwned() = %v, %v", owned, err)
	}
}

func TestPluginStorageMigrationDoesNotCreateTargetOnInvalidSource(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "legacy.sqlite")
	target := filepath.Join(root, "plugin.sqlite")
	if err := os.WriteFile(source, []byte("not sqlite"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := preparePluginStorage(context.Background(), Config{DBPath: target, LegacyDBPath: source}); err == nil {
		t.Fatal("preparePluginStorage() unexpectedly accepted invalid SQLite")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("target exists after failed migration: %v", err)
	}
}

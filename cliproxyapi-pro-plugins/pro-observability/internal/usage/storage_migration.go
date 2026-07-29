package embeddedusage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	pluginStorageMigrationVersion  = 1
	pluginStorageOwnerNamespace    = "observability.storage_owner"
	pluginStorageOwnerPlugin       = "pro-observability"
	pluginStorageMigrationFileMode = 0o600
)

type PluginStorageMigration struct {
	Version       int    `json:"version"`
	Owner         string `json:"owner"`
	Mode          string `json:"mode"`
	SourcePath    string `json:"sourcePath,omitempty"`
	TargetPath    string `json:"targetPath"`
	CompletedAtMS int64  `json:"completedAtMs"`
}

func preparePluginStorage(ctx context.Context, cfg Config) (PluginStorageMigration, error) {
	target, err := normalizedDatabasePath(cfg.DBPath)
	if err != nil {
		return PluginStorageMigration{}, fmt.Errorf("target database path: %w", err)
	}
	source, err := normalizedDatabasePath(cfg.LegacyDBPath)
	if err != nil {
		return PluginStorageMigration{}, fmt.Errorf("legacy database path: %w", err)
	}
	result := PluginStorageMigration{
		Version: pluginStorageMigrationVersion, Owner: pluginStorageOwnerPlugin,
		TargetPath: target,
	}
	if err = os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return PluginStorageMigration{}, err
	}
	targetInfo, targetErr := os.Stat(target)
	sourceInfo, sourceErr := os.Stat(source)
	if targetErr != nil && !errors.Is(targetErr, os.ErrNotExist) {
		return PluginStorageMigration{}, targetErr
	}
	if sourceErr != nil && !errors.Is(sourceErr, os.ErrNotExist) {
		return PluginStorageMigration{}, sourceErr
	}
	if targetInfo != nil && targetInfo.IsDir() {
		return PluginStorageMigration{}, fmt.Errorf("target database path is a directory: %s", target)
	}
	if sourceInfo != nil && sourceInfo.IsDir() {
		return PluginStorageMigration{}, fmt.Errorf("legacy database path is a directory: %s", source)
	}
	if sameDatabasePath(source, target) || (sourceInfo != nil && targetInfo != nil && os.SameFile(sourceInfo, targetInfo)) {
		if sourceInfo == nil {
			result.Mode = "initialized"
			return result, nil
		}
		owned, checkErr := pluginStorageOwned(ctx, target)
		if checkErr != nil {
			return PluginStorageMigration{}, checkErr
		}
		if owned {
			return readPluginStorageMigration(ctx, target)
		}
		result.Mode = "adopted-in-place"
		return result, nil
	}
	if targetInfo != nil {
		owned, checkErr := pluginStorageOwned(ctx, target)
		if checkErr != nil {
			return PluginStorageMigration{}, checkErr
		}
		if owned {
			return readPluginStorageMigration(ctx, target)
		}
		if sourceInfo != nil {
			return PluginStorageMigration{}, fmt.Errorf("both unowned target and legacy usage databases exist: %s and %s", target, source)
		}
		result.Mode = "adopted-target"
		return result, nil
	}
	if sourceInfo == nil {
		result.Mode = "initialized"
		return result, nil
	}
	if err = snapshotSQLiteDatabase(ctx, source, target); err != nil {
		return PluginStorageMigration{}, err
	}
	result.Mode = "copied"
	result.SourcePath = source
	return result, nil
}

func (s *Store) completePluginStorageMigration(ctx context.Context, migration PluginStorageMigration) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("usage store is not available")
	}
	if migration.CompletedAtMS <= 0 {
		migration.CompletedAtMS = time.Now().UnixMilli()
	}
	raw, err := json.Marshal(migration)
	if err != nil {
		return err
	}
	return retrySQLiteBusy(ctx, func() error {
		_, execErr := s.db.ExecContext(ctx, `insert into pro_settings(namespace, schema_version, settings_json, updated_at_ms)
			values(?, ?, ?, ?)
			on conflict(namespace) do update set schema_version = excluded.schema_version,
			settings_json = excluded.settings_json, updated_at_ms = excluded.updated_at_ms`,
			pluginStorageOwnerNamespace, pluginStorageMigrationVersion, string(raw), migration.CompletedAtMS)
		return execErr
	})
}

func pluginStorageOwned(ctx context.Context, path string) (bool, error) {
	migration, err := readPluginStorageMigration(ctx, path)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || strings.Contains(strings.ToLower(err.Error()), "no such table") {
			return false, nil
		}
		return false, err
	}
	return migration.Owner == pluginStorageOwnerPlugin, nil
}

func readPluginStorageMigration(ctx context.Context, path string) (PluginStorageMigration, error) {
	db, err := sql.Open("sqlite3", sqliteReadOnlyDSN(path))
	if err != nil {
		return PluginStorageMigration{}, err
	}
	defer db.Close()
	var raw string
	err = db.QueryRowContext(ctx, `select settings_json from pro_settings where namespace = ?`, pluginStorageOwnerNamespace).Scan(&raw)
	if err != nil {
		return PluginStorageMigration{}, err
	}
	var migration PluginStorageMigration
	if err = json.Unmarshal([]byte(raw), &migration); err != nil {
		return PluginStorageMigration{}, fmt.Errorf("decode plugin storage migration marker: %w", err)
	}
	if migration.Owner != pluginStorageOwnerPlugin {
		return PluginStorageMigration{}, fmt.Errorf("usage database is owned by %q, not %q", migration.Owner, pluginStorageOwnerPlugin)
	}
	// The marker may travel with a moved database. Runtime ownership follows the
	// configured file that was actually inspected, while mode/source/completion
	// remain the original audit record.
	migration.TargetPath = path
	return migration, nil
}

func snapshotSQLiteDatabase(ctx context.Context, source, target string) error {
	sourceDB, err := sql.Open("sqlite3", source)
	if err != nil {
		return err
	}
	if err = sourceDB.PingContext(ctx); err != nil {
		_ = sourceDB.Close()
		return fmt.Errorf("open legacy usage database: %w", err)
	}
	if _, err = sourceDB.ExecContext(ctx, `pragma wal_checkpoint(TRUNCATE)`); err != nil {
		_ = sourceDB.Close()
		return fmt.Errorf("checkpoint legacy usage database: %w", err)
	}
	var integrity string
	if err = sourceDB.QueryRowContext(ctx, `pragma integrity_check`).Scan(&integrity); err != nil {
		_ = sourceDB.Close()
		return fmt.Errorf("check legacy usage database: %w", err)
	}
	if integrity != "ok" {
		_ = sourceDB.Close()
		return fmt.Errorf("legacy usage database integrity check failed: %s", integrity)
	}
	if err = sourceDB.Close(); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(target), ".usage-plugin-migration-*.sqlite")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	sourceFile, err := os.Open(source)
	if err != nil {
		_ = temporary.Close()
		return err
	}
	_, copyErr := io.Copy(temporary, sourceFile)
	closeSourceErr := sourceFile.Close()
	syncErr := temporary.Sync()
	closeTargetErr := temporary.Close()
	for _, candidate := range []error{copyErr, closeSourceErr, syncErr, closeTargetErr} {
		if candidate != nil {
			return candidate
		}
	}
	if err = os.Chmod(temporaryPath, pluginStorageMigrationFileMode); err != nil {
		return err
	}
	var directory *os.File
	if runtime.GOOS != "windows" {
		directory, err = os.Open(filepath.Dir(target))
		if err != nil {
			return err
		}
	}
	if err = os.Rename(temporaryPath, target); err != nil {
		if directory != nil {
			_ = directory.Close()
		}
		return err
	}
	if directory == nil {
		return nil
	}
	syncErr = directory.Sync()
	_ = directory.Close()
	if syncErr != nil {
		_ = os.Remove(target)
		return syncErr
	}
	return nil
}

func normalizedDatabasePath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	return filepath.Abs(filepath.Clean(path))
}

func sameDatabasePath(left, right string) bool {
	return filepath.Clean(left) == filepath.Clean(right)
}

func sqliteReadOnlyDSN(path string) string {
	return "file:" + filepath.ToSlash(path) + "?mode=ro&_busy_timeout=5000"
}

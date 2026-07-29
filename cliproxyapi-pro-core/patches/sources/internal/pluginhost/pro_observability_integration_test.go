package pluginhost

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/embeddedusage"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
	_ "modernc.org/sqlite"
)

func TestProObservabilityDynamicPluginStateCapabilities(t *testing.T) {
	source := os.Getenv("CLIPROXY_PRO_OBSERVABILITY_PLUGIN")
	if source == "" {
		t.Skip("CLIPROXY_PRO_OBSERVABILITY_PLUGIN is not set")
	}
	raw, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("read plugin: %v", err)
	}
	root := t.TempDir()
	archDir := filepath.Join(root, runtime.GOOS, runtime.GOARCH)
	if err = os.MkdirAll(archDir, 0o755); err != nil {
		t.Fatalf("mkdir plugin dir: %v", err)
	}
	pluginPath := filepath.Join(archDir, "pro-observability"+pluginExtension(runtime.GOOS))
	if err = os.WriteFile(pluginPath, raw, 0o755); err != nil {
		t.Fatalf("write plugin: %v", err)
	}
	dataDir := t.TempDir()
	legacyPath := filepath.Join(dataDir, "legacy-usage.sqlite")
	dbPath := filepath.Join(dataDir, "plugin", "usage.sqlite")
	legacyDB, err := sql.Open("sqlite", legacyPath)
	if err != nil {
		t.Fatalf("open legacy database: %v", err)
	}
	if _, err = legacyDB.Exec(`create table legacy_probe(value text not null); insert into legacy_probe(value) values('preserved')`); err != nil {
		t.Fatalf("seed legacy database: %v", err)
	}
	if err = legacyDB.Close(); err != nil {
		t.Fatalf("close legacy database: %v", err)
	}
	t.Setenv("USAGE_DB_PATH", legacyPath)
	t.Setenv("PRO_OBSERVABILITY_DB_PATH", dbPath)
	cfg, err := config.ParseConfigBytes([]byte(fmt.Sprintf(`plugins:
  enabled: true
  dir: %q
  configs:
    pro-observability:
      enabled: true
      db-path: %q
      legacy-db-path: %q
`, root, dbPath, legacyPath)))
	if err != nil {
		t.Fatalf("parse plugin config: %v", err)
	}
	host := New()
	t.Cleanup(host.ShutdownAll)
	host.ApplyConfig(context.Background(), cfg)
	if !host.PluginRegistered("pro-observability") {
		t.Fatal("pro-observability was not registered")
	}
	if !host.ProObservabilityReady("pro-observability") {
		t.Fatal("pro-observability did not expose the complete persistence capability set")
	}
	migratedDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open migrated database: %v", err)
	}
	defer migratedDB.Close()
	var probe string
	if err = migratedDB.QueryRow(`select value from legacy_probe`).Scan(&probe); err != nil || probe != "preserved" {
		t.Fatalf("legacy probe = %q, %v", probe, err)
	}
	host.RegisterManagementRoutes(context.Background(), nil)
	migration, handled, err := host.CallManagement(context.Background(), pluginapi.ManagementRequest{
		Method: "GET", Path: "/v0/management/pro/observability/migration/status",
	})
	if err != nil || !handled || migration.StatusCode != 200 || !strings.Contains(string(migration.Body), `"mode":"copied"`) {
		t.Fatalf("migration status = %#v handled=%v err=%v", migration, handled, err)
	}
	entry := pluginapi.QuotaCacheEntry{Provider: "xai", FileName: "x.json", Data: json.RawMessage(`{"billing":{"planType":"free"}}`)}
	if _, handled, err := host.PutQuotaCache(context.Background(), pluginapi.QuotaCachePutRequest{ContractVersion: pluginapi.QuotaCacheContractVersion, Entry: entry, Merge: true}); err != nil || !handled {
		t.Fatalf("PutQuotaCache() handled=%v err=%v", handled, err)
	}
	quota, handled, err := host.GetQuotaCache(context.Background(), pluginapi.QuotaCacheGetRequest{ContractVersion: pluginapi.QuotaCacheContractVersion, Provider: "xai", FileName: "x.json"})
	if err != nil || !handled || len(quota.Entries) != 1 {
		t.Fatalf("GetQuotaCache() = %#v handled=%v err=%v", quota, handled, err)
	}
	stats := pluginapi.AuthRuntimeStats{AuthIndex: "idx", AuthID: "auth", SelectedCount: 2, SuccessCount: 5}
	if _, handled, err = host.PutAuthRuntimeStats(context.Background(), pluginapi.AuthRuntimeStatsPutRequest{Stats: stats}); err != nil || !handled {
		t.Fatalf("PutAuthRuntimeStats() handled=%v err=%v", handled, err)
	}
	runtimeStats, handled, err := host.GetAuthRuntimeStats(context.Background(), pluginapi.AuthRuntimeStatsGetRequest{AuthIndex: "idx", AuthID: "auth"})
	if err != nil || !handled || !runtimeStats.Found || runtimeStats.Stats.SuccessCount != 5 {
		t.Fatalf("GetAuthRuntimeStats() = %#v handled=%v err=%v", runtimeStats, handled, err)
	}
	setting := pluginapi.ProSetting{Namespace: "routing.request-protection", SchemaVersion: 1, Settings: json.RawMessage(`{"enabled":true}`), UpdatedAtMS: 10}
	if _, handled, err = host.PutProSetting(context.Background(), pluginapi.ProSettingPutRequest{Setting: setting}); err != nil || !handled {
		t.Fatalf("PutProSetting() handled=%v err=%v", handled, err)
	}
	storedSetting, handled, err := host.GetProSetting(context.Background(), pluginapi.ProSettingGetRequest{Namespace: setting.Namespace})
	if err != nil || !handled || !storedSetting.Found || string(storedSetting.Setting.Settings) != string(setting.Settings) {
		t.Fatalf("GetProSetting() = %#v handled=%v err=%v", storedSetting, handled, err)
	}

	embeddedusage.SetAccountInspectionScheduleHandlers(func() ([]byte, bool, error) {
		return []byte(`{"enabled":true}`), true, nil
	}, nil)
	t.Cleanup(func() { embeddedusage.SetAccountInspectionScheduleHandlers(nil, nil) })
	exported, handled, err := host.CallManagement(context.Background(), pluginapi.ManagementRequest{
		Method: "GET", Path: "/v0/management/usage/export",
	})
	if err != nil || !handled || exported.StatusCode != 200 || !strings.Contains(string(exported.Body), `"record_type":"account_inspection_schedule"`) {
		t.Fatalf("usage export = %#v handled=%v err=%v", exported, handled, err)
	}
	var importedSchedule string
	var importedProSettings []embeddedusage.ProSetting
	embeddedusage.SetAccountInspectionScheduleHandlers(nil, func(raw []byte) error {
		importedSchedule = string(raw)
		return nil
	})
	embeddedusage.SetProSettingsImportHandler(func(items []embeddedusage.ProSetting) error {
		importedProSettings = append([]embeddedusage.ProSetting(nil), items...)
		return nil
	})
	t.Cleanup(func() { embeddedusage.SetProSettingsImportHandler(nil) })
	imported, handled, err := host.CallManagement(context.Background(), pluginapi.ManagementRequest{
		Method: "POST", Path: "/v0/management/usage/import", Body: exported.Body,
	})
	foundImportedSetting := false
	for _, item := range importedProSettings {
		if item.Namespace == setting.Namespace && string(item.Settings) == string(setting.Settings) {
			foundImportedSetting = true
			break
		}
	}
	if err != nil || !handled || imported.StatusCode != 200 || importedSchedule != `{"enabled":true}` || !foundImportedSetting {
		t.Fatalf("usage import = %#v handled=%v imported=%s err=%v", imported, handled, importedSchedule, err)
	}

	embeddedusage.SetRuntimeStatePluginBackend(host)
	embeddedusage.QueueAuthRuntimeStats(embeddedusage.AuthRuntimeStats{
		AuthIndex: "idx-shutdown", AuthID: "auth-shutdown", SelectedCount: 11, UpdatedAtMS: 100,
	})
	host.ShutdownAll()
	var selectedCount int64
	if err = migratedDB.QueryRow(`select selected_count from auth_runtime_stats where auth_index = 'idx-shutdown'`).Scan(&selectedCount); err != nil || selectedCount != 11 {
		t.Fatalf("runtime stats were not flushed before plugin unload: count=%d err=%v", selectedCount, err)
	}
}

func TestProObservabilityDynamicPluginMigrationFailurePreventsRegistration(t *testing.T) {
	source := os.Getenv("CLIPROXY_PRO_OBSERVABILITY_PLUGIN")
	if source == "" {
		t.Skip("CLIPROXY_PRO_OBSERVABILITY_PLUGIN is not set")
	}
	raw, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	archDir := filepath.Join(root, runtime.GOOS, runtime.GOARCH)
	if err = os.MkdirAll(archDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(archDir, "pro-observability"+pluginExtension(runtime.GOOS)), raw, 0o755); err != nil {
		t.Fatal(err)
	}
	legacyPath := filepath.Join(t.TempDir(), "invalid.sqlite")
	if err = os.WriteFile(legacyPath, []byte("not sqlite"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("USAGE_DB_PATH", legacyPath)
	targetPath := filepath.Join(t.TempDir(), "plugin.sqlite")
	t.Setenv("PRO_OBSERVABILITY_DB_PATH", targetPath)
	cfg, err := config.ParseConfigBytes([]byte(fmt.Sprintf(`plugins:
  enabled: true
  dir: %q
  configs:
    pro-observability:
      enabled: true
      db-path: %q
      legacy-db-path: %q
`, root, targetPath, legacyPath)))
	if err != nil {
		t.Fatal(err)
	}
	host := New()
	t.Cleanup(host.ShutdownAll)
	host.ApplyConfig(context.Background(), cfg)
	if host.PluginRegistered("pro-observability") {
		t.Fatal("plugin registered even though legacy usage migration failed")
	}
}

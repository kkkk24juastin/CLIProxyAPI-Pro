package embeddedusage

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v6/sdk/pluginapi"
)

type proSettingsBackendStub struct{ item pluginapi.ProSetting }

func (s *proSettingsBackendStub) GetProSetting(context.Context, pluginapi.ProSettingGetRequest) (pluginapi.ProSettingGetResponse, bool, error) {
	return pluginapi.ProSettingGetResponse{Found: s.item.Namespace != "", Setting: s.item}, true, nil
}

func (s *proSettingsBackendStub) PutProSetting(_ context.Context, req pluginapi.ProSettingPutRequest) (pluginapi.ProSettingPutResponse, bool, error) {
	s.item = req.Setting
	return pluginapi.ProSettingPutResponse{}, true, nil
}

func TestPluginProSettingsBackendOwnsRuntimeWithoutLegacyService(t *testing.T) {
	SetDefaultService(nil)
	stub := &proSettingsBackendStub{}
	SetProSettingsPluginBackend(stub)
	t.Cleanup(func() { SetProSettingsPluginBackend(nil) })
	want := ProSetting{Namespace: "routing.request-protection", SchemaVersion: 1, Settings: json.RawMessage(`{"enabled":true}`), UpdatedAtMS: 10}
	if err := SetProSetting(context.Background(), want); err != nil {
		t.Fatalf("SetProSetting() error = %v", err)
	}
	got, found, err := GetProSetting(context.Background(), want.Namespace)
	if err != nil || !found || got.Namespace != want.Namespace || string(got.Settings) != string(want.Settings) {
		t.Fatalf("GetProSetting() = %#v found=%v err=%v", got, found, err)
	}
}

func TestPluginProSettingsBackendFailsClosedWhenCapabilityMissing(t *testing.T) {
	SetDefaultService(nil)
	SetProSettingsPluginBackend(nil)
	if err := SetProSetting(context.Background(), ProSetting{Namespace: "routing.test", SchemaVersion: 1, Settings: json.RawMessage(`{}`)}); err == nil {
		t.Fatal("SetProSetting() error = nil, want missing plugin capability")
	}
}

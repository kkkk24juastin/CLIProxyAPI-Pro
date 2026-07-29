package pluginhost

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

type testProSettingsStore struct{ item pluginapi.ProSetting }

func (s *testProSettingsStore) GetProSetting(context.Context, pluginapi.ProSettingGetRequest) (pluginapi.ProSettingGetResponse, error) {
	return pluginapi.ProSettingGetResponse{Found: s.item.Namespace != "", Setting: s.item}, nil
}

func (s *testProSettingsStore) PutProSetting(_ context.Context, req pluginapi.ProSettingPutRequest) (pluginapi.ProSettingPutResponse, error) {
	s.item = req.Setting
	return pluginapi.ProSettingPutResponse{}, nil
}

func TestProSettingsStoreFacadeReadsAndWrites(t *testing.T) {
	store := &testProSettingsStore{}
	host := New()
	setHostSnapshotForTest(host, true, capabilityRecord{id: "observability", plugin: pluginapi.Plugin{
		Metadata:     pluginapi.Metadata{Name: "observability", Version: "1", Author: "test", GitHubRepository: "https://example.com"},
		Capabilities: pluginapi.Capabilities{ProSettingsStore: store},
	}})
	want := pluginapi.ProSetting{Namespace: "routing.test", SchemaVersion: 1, Settings: json.RawMessage(`{"enabled":true}`), UpdatedAtMS: 10}
	if _, handled, err := host.PutProSetting(context.Background(), pluginapi.ProSettingPutRequest{Setting: want}); err != nil || !handled {
		t.Fatalf("PutProSetting() handled=%v err=%v", handled, err)
	}
	got, handled, err := host.GetProSetting(context.Background(), pluginapi.ProSettingGetRequest{Namespace: want.Namespace})
	if err != nil || !handled || !got.Found || got.Setting.Namespace != want.Namespace || string(got.Setting.Settings) != string(want.Settings) {
		t.Fatalf("GetProSetting() = %#v handled=%v err=%v", got, handled, err)
	}
}

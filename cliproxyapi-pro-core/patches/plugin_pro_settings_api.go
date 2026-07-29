// ProSettingsStore owns versioned Pro settings persisted outside config.yaml.
type ProSettingsStore interface {
	GetProSetting(context.Context, ProSettingGetRequest) (ProSettingGetResponse, error)
	PutProSetting(context.Context, ProSettingPutRequest) (ProSettingPutResponse, error)
}

type ProSetting struct {
	Namespace     string          `json:"namespace"`
	SchemaVersion int             `json:"schemaVersion"`
	Settings      json.RawMessage `json:"settings"`
	UpdatedAtMS   int64           `json:"updatedAtMs"`
}

type ProSettingGetRequest struct {
	Namespace string `json:"namespace"`
}

type ProSettingGetResponse struct {
	Found   bool       `json:"found"`
	Setting ProSetting `json:"setting"`
}

type ProSettingPutRequest struct {
	Setting ProSetting `json:"setting"`
}

type ProSettingPutResponse struct{}

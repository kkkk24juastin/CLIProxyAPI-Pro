// AuthModelFilter subtracts provider-native models from one concrete auth registration.
// Filters cannot add models or mutate host model metadata.
type AuthModelFilter interface {
	FilterAuthModels(context.Context, AuthModelFilterRequest) (AuthModelFilterResponse, error)
}

// AuthModelFilterRequest carries one auth and the provider-native models that remain
// after host-level provider and per-auth exclusions have been applied.
type AuthModelFilterRequest struct {
	Plugin       Metadata          `json:"plugin"`
	AuthID       string            `json:"auth_id"`
	AuthIndex    string            `json:"auth_index,omitempty"`
	AuthProvider string            `json:"auth_provider"`
	AuthKind     string            `json:"auth_kind,omitempty"`
	StorageJSON  []byte            `json:"storage_json,omitempty"`
	Metadata     map[string]any    `json:"metadata,omitempty"`
	Attributes   map[string]string `json:"attributes,omitempty"`
	Models       []ModelInfo       `json:"models"`
	Host         HostConfigSummary `json:"host"`
	HTTPClient   HostHTTPClient    `json:"-"`
}

// AuthModelFilterResponse returns exact model IDs to subtract from the current set.
// Handled=false leaves the current model set unchanged.
type AuthModelFilterResponse struct {
	Handled          bool              `json:"handled"`
	ExcludedModelIDs []string          `json:"excluded_model_ids,omitempty"`
	Annotations      map[string]string `json:"annotations,omitempty"`
}

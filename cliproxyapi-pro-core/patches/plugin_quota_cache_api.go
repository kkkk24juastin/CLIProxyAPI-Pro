const (
	// QuotaCacheContractVersion is the host/plugin quota-cache wire contract.
	QuotaCacheContractVersion = 1
)

// QuotaCacheStore owns persisted quota snapshots and provider observations.
type QuotaCacheStore interface {
	GetQuotaCache(context.Context, QuotaCacheGetRequest) (QuotaCacheGetResponse, error)
	PutQuotaCache(context.Context, QuotaCachePutRequest) (QuotaCachePutResponse, error)
	DeleteQuotaCache(context.Context, QuotaCacheDeleteRequest) (QuotaCacheDeleteResponse, error)
	ObserveQuota(context.Context, QuotaObservationRequest) (QuotaObservationResponse, error)
}

// QuotaCacheEntry is one provider/auth quota snapshot.
type QuotaCacheEntry struct {
	ID                  string          `json:"id"`
	Provider            string          `json:"provider"`
	FileName            string          `json:"file_name"`
	AuthIndex           string          `json:"auth_index,omitempty"`
	IdentityFingerprint string          `json:"identity_fingerprint,omitempty"`
	Data                json.RawMessage `json:"data"`
	CachedAt            int64           `json:"cached_at"`
	AccessedAt          int64           `json:"accessed_at"`
	ObservedAt          int64           `json:"observed_at"`
	StoredAt            int64           `json:"stored_at"`
	Version             int             `json:"version"`
	Revision            int64           `json:"revision"`
}

type QuotaCacheGetRequest struct {
	ContractVersion int    `json:"contract_version"`
	Provider        string `json:"provider,omitempty"`
	FileName        string `json:"file_name,omitempty"`
}

type QuotaCacheGetResponse struct {
	ContractVersion int               `json:"contract_version"`
	Entries         []QuotaCacheEntry `json:"entries"`
}

type QuotaCachePutRequest struct {
	ContractVersion int             `json:"contract_version"`
	Entry           QuotaCacheEntry `json:"entry"`
	Merge           bool            `json:"merge,omitempty"`
}

type QuotaCachePutResponse struct {
	ContractVersion int `json:"contract_version"`
}

type QuotaCacheDeleteRequest struct {
	ContractVersion int    `json:"contract_version"`
	Provider        string `json:"provider,omitempty"`
	FileName        string `json:"file_name,omitempty"`
}

type QuotaCacheDeleteResponse struct {
	ContractVersion int `json:"contract_version"`
}

// QuotaObservation carries provider response evidence to the plugin-owned parser.
type QuotaObservation struct {
	Provider   string      `json:"provider"`
	FileName   string      `json:"file_name"`
	AuthIndex  string      `json:"auth_index,omitempty"`
	Email      string      `json:"email,omitempty"`
	Label      string      `json:"label,omitempty"`
	Model      string      `json:"model,omitempty"`
	Status     int         `json:"status"`
	Headers    http.Header `json:"headers,omitempty"`
	Body       []byte      `json:"body,omitempty"`
	ObservedAt time.Time   `json:"observed_at"`
}

type QuotaObservationRequest struct {
	ContractVersion int              `json:"contract_version"`
	Observation     QuotaObservation `json:"observation"`
}

type QuotaObservationResponse struct {
	ContractVersion int `json:"contract_version"`
}

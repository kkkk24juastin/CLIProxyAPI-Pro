// HostAuthInspectionListResponse contains the non-secret credential projection
// available to the account-inspection plugin.
type HostAuthInspectionListResponse struct {
	Auths []HostAuthFileEntry `json:"auths"`
}

// HostAuthRefreshRequest asks the host to refresh one concrete credential.
type HostAuthRefreshRequest struct {
	AuthIndex string `json:"auth_index"`
	Force     bool   `json:"force,omitempty"`
}

// HostAuthRefreshResponse reports the latest runtime credential after refresh.
type HostAuthRefreshResponse struct {
	Triggered bool              `json:"triggered"`
	Refreshed bool              `json:"refreshed"`
	Auth      HostAuthFileEntry `json:"auth"`
}

// HostAuthHTTPRequest asks the host to execute an HTTP request with one pinned
// credential through its registered provider executor.
type HostAuthHTTPRequest struct {
	AuthIndex        string      `json:"auth_index"`
	Method           string      `json:"method"`
	URL              string      `json:"url"`
	Headers          http.Header `json:"headers,omitempty"`
	Body             []byte      `json:"body,omitempty"`
	TimeoutMS        int         `json:"timeout_ms,omitempty"`
	MaxResponseBytes int64       `json:"max_response_bytes,omitempty"`
}

// HostAuthHTTPResponse is the bounded response returned by HostAuthHTTPRequest.
type HostAuthHTTPResponse struct {
	StatusCode int         `json:"status_code"`
	Headers    http.Header `json:"headers,omitempty"`
	Body       []byte      `json:"body,omitempty"`
	Truncated  bool        `json:"truncated,omitempty"`
}

// HostAuthHealthError is host-managed credential health state attributed to a
// caller. Plugins never replace the complete credential JSON for health writes.
type HostAuthHealthError struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	HTTPStatus int    `json:"http_status,omitempty"`
	Retryable  bool   `json:"retryable,omitempty"`
}

// HostAuthHealthPatchRequest atomically applies inspection-owned health state.
type HostAuthHealthPatchRequest struct {
	AuthIndex       string               `json:"auth_index"`
	ExpectedRevision int64               `json:"expected_revision,omitempty"`
	Disabled        *bool                `json:"disabled,omitempty"`
	Error           *HostAuthHealthError `json:"error,omitempty"`
	ClearError      bool                 `json:"clear_error,omitempty"`
}

type HostAuthHealthPatchResponse struct {
	Auth HostAuthFileEntry `json:"auth"`
}

type HostAuthDeleteRequest struct {
	AuthIndex        string `json:"auth_index"`
	ExpectedRevision int64  `json:"expected_revision,omitempty"`
}

type HostAuthDeleteResponse struct {
	Name string `json:"name"`
}

type HostAuthQuotaFetchRequest struct {
	AuthIndex string `json:"auth_index"`
}

type HostAuthQuotaFetchResponse struct {
	Handled        bool          `json:"handled"`
	Snapshot       QuotaSnapshot `json:"snapshot"`
	UpstreamStatus int           `json:"upstream_status,omitempty"`
	Error          string        `json:"error,omitempty"`
}

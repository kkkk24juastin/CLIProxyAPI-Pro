// RuntimeStateStore owns persisted per-auth scheduler statistics.
type RuntimeStateStore interface {
	GetAuthRuntimeStats(context.Context, AuthRuntimeStatsGetRequest) (AuthRuntimeStatsGetResponse, error)
	PutAuthRuntimeStats(context.Context, AuthRuntimeStatsPutRequest) (AuthRuntimeStatsPutResponse, error)
	DeleteAuthRuntimeState(context.Context, AuthRuntimeStateDeleteRequest) (AuthRuntimeStateDeleteResponse, error)
}

type RuntimeRequestBucket struct {
	BucketID int64 `json:"bucket_id"`
	Success  int64 `json:"success"`
	Failed   int64 `json:"failed"`
}

type AuthRuntimeStats struct {
	AuthIndex           string                 `json:"auth_index"`
	AuthID              string                 `json:"auth_id"`
	FileName            string                 `json:"file_name,omitempty"`
	IdentityFingerprint string                 `json:"identity_fingerprint,omitempty"`
	SelectedCount       int64                  `json:"selected_count"`
	SuccessCount        int64                  `json:"success_count"`
	FailureCount        int64                  `json:"failure_count"`
	RecentBuckets       []RuntimeRequestBucket `json:"recent_buckets"`
	UpdatedAtMS         int64                  `json:"updated_at_ms"`
}

type AuthRuntimeStatsGetRequest struct {
	AuthIndex string `json:"auth_index,omitempty"`
	AuthID    string `json:"auth_id,omitempty"`
}

type AuthRuntimeStatsGetResponse struct {
	Found bool             `json:"found"`
	Stats AuthRuntimeStats `json:"stats"`
}

type AuthRuntimeStatsPutRequest struct {
	Stats AuthRuntimeStats `json:"stats"`
}

type AuthRuntimeStatsPutResponse struct{}

type AuthRuntimeStateDeleteRequest struct {
	AuthID    string `json:"auth_id,omitempty"`
	AuthIndex string `json:"auth_index,omitempty"`
	FileName  string `json:"file_name,omitempty"`
}

type AuthRuntimeStateDeleteResponse struct{}

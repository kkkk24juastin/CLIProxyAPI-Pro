// Package state contains stable Pro runtime-state contracts shared by routing,
// backup orchestration and the SQLite adapter.
package state

type RoutingCursor struct {
	CursorKey   string `json:"cursorKey"`
	LastAuthID  string `json:"lastAuthId"`
	UpdatedAtMS int64  `json:"updatedAtMs"`
}

type RequestBucket struct {
	BucketID int64 `json:"bucketId"`
	Success  int64 `json:"success"`
	Failed   int64 `json:"failed"`
}

type AuthRuntimeStats struct {
	AuthIndex           string          `json:"authIndex"`
	AuthID              string          `json:"authId"`
	FileName            string          `json:"fileName,omitempty"`
	IdentityFingerprint string          `json:"identityFingerprint,omitempty"`
	SelectedCount       int64           `json:"selectedCount"`
	SuccessCount        int64           `json:"successCount"`
	FailureCount        int64           `json:"failureCount"`
	RecentBuckets       []RequestBucket `json:"recentBuckets"`
	Generation          int64           `json:"generation"`
	UpdatedAtMS         int64           `json:"updatedAtMs"`
}

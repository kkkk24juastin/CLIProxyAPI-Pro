package inspection

import (
	"encoding/json"
	"testing"
	"time"
)

func TestDecodeResultSnapshotNormalizesDerivedState(t *testing.T) {
	raw, err := json.Marshal(ResultSnapshot{
		Version:        ResultSnapshotVersion,
		State:          RunStateRunning,
		LastStartedAt:  200,
		LastFinishedAt: 100,
		Settings:       Settings{UsedPercentThreshold: -1},
		HealthCounts:   HealthCounts{Total: 99},
		Results: []Result{
			{Key: "b", Provider: "xai", IsQuota: true},
			{Key: "a", Provider: "codex"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := DecodeResultSnapshot(raw, time.Unix(1000, 0))
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.State != RunStateCompleted || snapshot.LastStartedAt != snapshot.LastFinishedAt {
		t.Fatalf("state/times = %q/%d/%d", snapshot.State, snapshot.LastStartedAt, snapshot.LastFinishedAt)
	}
	if len(snapshot.Results) != 2 || snapshot.Results[0].Key != "a" {
		t.Fatalf("sorted results = %#v", snapshot.Results)
	}
	if snapshot.HealthCounts.Total != 2 || snapshot.HealthCounts.QuotaExhausted != 1 {
		t.Fatalf("health counts = %#v", snapshot.HealthCounts)
	}
}

func TestDecodeResultSnapshotRejectsInvalidEnvelope(t *testing.T) {
	if _, err := DecodeResultSnapshot([]byte(`{"version":99,"lastFinishedAt":1}`), time.Now()); err == nil {
		t.Fatal("unsupported snapshot version accepted")
	}
	if _, err := DecodeResultSnapshot([]byte(`{"version":1}`), time.Now()); err == nil {
		t.Fatal("snapshot without completion time accepted")
	}
}

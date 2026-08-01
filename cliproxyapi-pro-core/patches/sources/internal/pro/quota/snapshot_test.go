package quota

import (
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func TestSnapshotMaxUsedPercent(t *testing.T) {
	remaining := 0.25
	used := 82.0
	snapshot := pluginapi.QuotaSnapshot{Items: []pluginapi.QuotaItem{
		{RemainingFraction: &remaining},
		{UsedPercent: &used},
	}}
	maximum, hasQuota := SnapshotMaxUsedPercent(snapshot)
	if !hasQuota || maximum == nil || *maximum != 82 {
		t.Fatalf("maximum=%v hasQuota=%v", maximum, hasQuota)
	}
	if maximum, hasQuota = SnapshotMaxUsedPercent(pluginapi.QuotaSnapshot{Items: []pluginapi.QuotaItem{{}}}); !hasQuota || maximum != nil {
		t.Fatalf("empty item maximum=%v hasQuota=%v", maximum, hasQuota)
	}
}

func TestNormalizeSnapshotBoundsValuesAndRetainsUnavailablePlan(t *testing.T) {
	remaining := 2.0
	used := -10.0
	previous := &pluginapi.QuotaSnapshot{Plan: &pluginapi.QuotaPlan{
		ID: "paid", Metadata: map[string]any{"tier": "pro"},
	}}
	got := NormalizeSnapshot(pluginapi.QuotaSnapshot{
		ObservedAtMS: time.Now().Add(24 * time.Hour).UnixMilli(),
		Items:        []pluginapi.QuotaItem{{ID: " item ", Label: " label ", RemainingFraction: &remaining, UsedPercent: &used}},
	}, "xai", previous, true, " unavailable ")
	if got.Provider != "xai" || got.SchemaVersion != pluginapi.QuotaSnapshotSchemaVersion {
		t.Fatalf("identity = %q/%d", got.Provider, got.SchemaVersion)
	}
	if *got.Items[0].RemainingFraction != 1 || *got.Items[0].UsedPercent != 0 {
		t.Fatalf("bounded values = %v/%v", *got.Items[0].RemainingFraction, *got.Items[0].UsedPercent)
	}
	if got.Plan == nil || !got.Plan.Stale || got.Plan.Error != "unavailable" {
		t.Fatalf("retained plan = %+v", got.Plan)
	}
	if len(got.Warnings) != 1 || got.Warnings[0].Code != "plan_unavailable" {
		t.Fatalf("warnings = %+v", got.Warnings)
	}
}

func TestCloneSnapshotDoesNotShareNestedCollections(t *testing.T) {
	source := &pluginapi.QuotaSnapshot{
		Items:    []pluginapi.QuotaItem{{ModelIDs: []string{"a"}, Metadata: map[string]any{"item": "source"}}},
		Metadata: map[string]any{"snapshot": "source"},
		Plan:     &pluginapi.QuotaPlan{Metadata: map[string]any{"plan": "source"}},
	}
	clone := CloneSnapshot(source)
	clone.Items[0].ModelIDs[0] = "changed"
	clone.Items[0].Metadata["item"] = "changed"
	clone.Metadata["snapshot"] = "changed"
	clone.Plan.Metadata["plan"] = "changed"
	if source.Items[0].ModelIDs[0] != "a" || source.Items[0].Metadata["item"] != "source" || source.Metadata["snapshot"] != "source" || source.Plan.Metadata["plan"] != "source" {
		t.Fatalf("clone mutated source: %+v", source)
	}
}

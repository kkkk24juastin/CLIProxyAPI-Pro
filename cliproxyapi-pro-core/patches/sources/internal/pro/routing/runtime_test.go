package routing

import "testing"

func TestCursorAfterIDSurvivesCredentialSetChanges(t *testing.T) {
	if got := CursorAfterID([]string{"a", "c", "d"}, "b"); got != 1 {
		t.Fatalf("CursorAfterID() = %d, want insertion point 1", got)
	}
	if got := CursorAfterID([]string{"a", "b", "c"}, "b"); got != 2 {
		t.Fatalf("CursorAfterID() = %d, want next index 2", got)
	}
}

func TestInspectionTakesOwnershipFromRequestProtection(t *testing.T) {
	metadata := map[string]any{ProtectionMetadataKey: map[string]any{"owner": ProtectionOwner}}
	if !ProtectionOwned(metadata) {
		t.Fatal("request protection ownership not detected")
	}
	InspectionOwnsStatus(metadata)
	if ProtectionOwned(metadata) {
		t.Fatal("inspection must have precedence over request protection")
	}
}

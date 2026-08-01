// Package routing owns durable selection cursor semantics and request-
// protection ownership policy. Host Auth objects are adapted at the SDK and
// management boundaries rather than leaking into this module.
package routing

import "strings"

const (
	LegacyRoundRobinCursorPrefix = "legacy|single|"
	ProtectionMetadataKey        = "request_protection"
	ProtectionOwner              = "request-protection"
)

func LegacyRoundRobinCursorKey(provider, canonicalModel string) string {
	return LegacyRoundRobinCursorPrefix + strings.ToLower(strings.TrimSpace(provider)) + "|" + strings.TrimSpace(canonicalModel)
}

// CursorAfterID converts a durable last-picked identity into the next index
// for an already sorted list. Insertions/removals therefore preserve the
// historical round-robin behavior without persisting fragile array offsets.
func CursorAfterID(sortedIDs []string, lastID string) int {
	lastID = strings.TrimSpace(lastID)
	if lastID == "" || len(sortedIDs) == 0 {
		return 0
	}
	for index, id := range sortedIDs {
		if id == lastID {
			return index + 1
		}
		if id > lastID {
			return index
		}
	}
	return 0
}

func ProtectionMetadata(metadata map[string]any) map[string]any {
	if metadata == nil {
		return nil
	}
	value, _ := metadata[ProtectionMetadataKey].(map[string]any)
	return value
}

func ProtectionOwned(metadata map[string]any) bool {
	value := ProtectionMetadata(metadata)
	return value != nil && strings.EqualFold(strings.TrimSpace(stringValue(value["owner"])), ProtectionOwner)
}

func ClearProtectionOwnership(metadata map[string]any) {
	if metadata != nil {
		delete(metadata, ProtectionMetadataKey)
	}
}

// InspectionOwnsStatus is the explicit precedence rule: once inspection
// applies a status decision, request protection can no longer auto-release it.
func InspectionOwnsStatus(metadata map[string]any) {
	ClearProtectionOwnership(metadata)
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}

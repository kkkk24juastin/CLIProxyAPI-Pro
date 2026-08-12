package redisqueue

import (
	"context"
	"encoding/json"
	"testing"

	coreusage "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/usage"
)

func TestUsageQueuePluginIncludesRequestedAndResponseSpeed(t *testing.T) {
	withEnabledQueue(t, func() {
		ctx := coreusage.WithSpeed(context.Background(), "fast")
		(&usageQueuePlugin{}).HandleUsage(ctx, coreusage.Record{
			Provider:      "claude",
			Model:         "claude-opus-test",
			ResponseSpeed: "standard",
			Detail:        coreusage.Detail{InputTokens: 1, TotalTokens: 1},
		})
		items := PopOldest(1)
		if len(items) != 1 {
			t.Fatalf("PopOldest() items = %d, want 1", len(items))
		}
		var payload map[string]json.RawMessage
		if err := json.Unmarshal(items[0], &payload); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}
		requireStringField(t, payload, "speed", "fast")
		requireStringField(t, payload, "response_speed", "standard")
	})
}

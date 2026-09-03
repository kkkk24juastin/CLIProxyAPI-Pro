package auth

import (
	"context"
	"testing"

	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	coreusage "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/usage"
)

func TestContextWithRequestedModelAliasIncludesSpeed(t *testing.T) {
	opts := cliproxyexecutor.Options{Metadata: map[string]any{cliproxyexecutor.SpeedMetadataKey: "fast"}}
	ctx := contextWithRequestedModelAlias(context.Background(), opts, "claude-opus-test")
	if got := coreusage.SpeedFromContext(ctx); got != "fast" {
		t.Fatalf("SpeedFromContext() = %q, want fast", got)
	}
}

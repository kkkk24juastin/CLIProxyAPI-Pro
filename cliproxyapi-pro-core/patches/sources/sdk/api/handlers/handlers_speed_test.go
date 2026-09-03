package handlers

import (
	"testing"

	coreexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
)

func TestSetServiceTierMetadataAlsoExtractsSpeed(t *testing.T) {
	metadata := map[string]any{}
	setServiceTierMetadata(metadata, []byte(`{"speed":" fast "}`))
	if got := metadata[coreexecutor.SpeedMetadataKey]; got != "fast" {
		t.Fatalf("SpeedMetadataKey = %v, want fast", got)
	}
}

package pluginhost

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/embeddedusage"
)

const proObservabilityPluginID = "pro-observability"

type proBackupImportRequest struct {
	Kind           string                             `json:"kind"`
	RoutingCursors []embeddedusage.RoutingCursorState `json:"routingCursors,omitempty"`
	RuntimeStats   []embeddedusage.AuthRuntimeStats   `json:"runtimeStats,omitempty"`
	ProSettings    []embeddedusage.ProSetting         `json:"proSettings,omitempty"`
}

func requireProBackupCallbackCaller(ctx context.Context) error {
	if hostCallbackPluginIDFromContext(ctx) != proObservabilityPluginID {
		return fmt.Errorf("Pro backup host callback is restricted to %s", proObservabilityPluginID)
	}
	return nil
}

func (h *Host) callHostProBackupImport(ctx context.Context, raw []byte) ([]byte, error) {
	if err := requireProBackupCallbackCaller(ctx); err != nil {
		return nil, err
	}
	var request proBackupImportRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		return nil, fmt.Errorf("decode Pro backup import request: %w", err)
	}
	var err error
	switch request.Kind {
	case "runtime-state":
		err = embeddedusage.ApplyImportedAuthRuntimeState(request.RoutingCursors, request.RuntimeStats)
	case "pro-settings":
		err = embeddedusage.ApplyImportedProSettings(request.ProSettings)
	default:
		return nil, fmt.Errorf("unsupported Pro backup import kind %q", request.Kind)
	}
	if err != nil {
		return nil, err
	}
	return marshalRPCResult(struct{}{})
}

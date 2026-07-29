package pluginhost

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/embeddedusage"
)

const proObservabilityPluginID = "pro-observability"

type proBackupExportRequest struct {
	Kind string `json:"kind"`
}

type proBackupExportResponse struct {
	Found bool            `json:"found"`
	Data  json.RawMessage `json:"data,omitempty"`
}

type proBackupImportRequest struct {
	Kind           string                             `json:"kind"`
	Data           json.RawMessage                    `json:"data,omitempty"`
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

func (h *Host) callHostProBackupExport(ctx context.Context, raw []byte) ([]byte, error) {
	if err := requireProBackupCallbackCaller(ctx); err != nil {
		return nil, err
	}
	var request proBackupExportRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		return nil, fmt.Errorf("decode Pro backup export request: %w", err)
	}
	var data []byte
	var found bool
	var err error
	switch strings.TrimSpace(request.Kind) {
	case "account-inspection-schedule":
		data, found, err = embeddedusage.ExportAccountInspectionSchedule()
	case "account-inspection-snapshot":
		data, found, err = embeddedusage.ExportAccountInspectionSnapshot()
	default:
		return nil, fmt.Errorf("unsupported Pro backup export kind %q", request.Kind)
	}
	if err != nil {
		return nil, err
	}
	return marshalRPCResult(proBackupExportResponse{Found: found, Data: append(json.RawMessage(nil), data...)})
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
	switch strings.TrimSpace(request.Kind) {
	case "runtime-state":
		err = embeddedusage.ApplyImportedAuthRuntimeState(request.RoutingCursors, request.RuntimeStats)
	case "pro-settings":
		err = embeddedusage.ApplyImportedProSettings(request.ProSettings)
	case "account-inspection-schedule":
		err = embeddedusage.ApplyImportedAccountInspectionSchedule(request.Data)
	case "account-inspection-snapshot":
		err = embeddedusage.ApplyImportedAccountInspectionSnapshot(request.Data)
	default:
		return nil, fmt.Errorf("unsupported Pro backup import kind %q", request.Kind)
	}
	if err != nil {
		return nil, err
	}
	return marshalRPCResult(struct{}{})
}

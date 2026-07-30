package main

import (
	"context"
	"encoding/json"
	"fmt"

	inspection "github.com/ssfun/CLIProxyAPI-Pro/cliproxyapi-pro-plugins/pro-observability/internal/inspection"
)

const (
	methodHostAuthInspectionList = "host.auth.inspection_list"
	methodHostAuthRefresh        = "host.auth.refresh"
	methodHostAuthHTTPDo         = "host.auth.http.do"
	methodHostAuthHealthPatch    = "host.auth.health.patch"
	methodHostAuthDelete         = "host.auth.delete"
	methodHostAuthQuotaFetch     = "host.auth.quota.fetch"
)

type rpcInspectionGateway struct{}

func (rpcInspectionGateway) List(context.Context) ([]inspection.HostAuthEntry, error) {
	raw, err := callHost(methodHostAuthInspectionList, struct{}{})
	if err != nil {
		return nil, err
	}
	var response struct {
		Auths []inspection.HostAuthEntry `json:"auths"`
	}
	if err = json.Unmarshal(raw, &response); err != nil {
		return nil, fmt.Errorf("decode host inspection list: %w", err)
	}
	return response.Auths, nil
}

func (rpcInspectionGateway) Refresh(_ context.Context, authIndex string, force bool) (inspection.HostAuthRefreshResponse, error) {
	raw, err := callHost(methodHostAuthRefresh, map[string]any{"auth_index": authIndex, "force": force})
	if err != nil {
		return inspection.HostAuthRefreshResponse{}, err
	}
	var response inspection.HostAuthRefreshResponse
	if err = json.Unmarshal(raw, &response); err != nil {
		return response, fmt.Errorf("decode host auth refresh: %w", err)
	}
	return response, nil
}

func (rpcInspectionGateway) HTTPDo(_ context.Context, request inspection.HostHTTPRequest) (inspection.HostHTTPResponse, error) {
	raw, err := callHost(methodHostAuthHTTPDo, request)
	if err != nil {
		return inspection.HostHTTPResponse{}, err
	}
	var response inspection.HostHTTPResponse
	if err = json.Unmarshal(raw, &response); err != nil {
		return response, fmt.Errorf("decode host auth HTTP response: %w", err)
	}
	return response, nil
}

func (rpcInspectionGateway) PatchHealth(_ context.Context, request inspection.HostHealthPatch) (inspection.HostAuthEntry, error) {
	raw, err := callHost(methodHostAuthHealthPatch, request)
	if err != nil {
		return inspection.HostAuthEntry{}, err
	}
	var response struct {
		Auth inspection.HostAuthEntry `json:"auth"`
	}
	if err = json.Unmarshal(raw, &response); err != nil {
		return response.Auth, fmt.Errorf("decode host auth health patch: %w", err)
	}
	return response.Auth, nil
}

func (rpcInspectionGateway) Delete(_ context.Context, authIndex string, revision int64) (string, error) {
	raw, err := callHost(methodHostAuthDelete, map[string]any{"auth_index": authIndex, "expected_revision": revision})
	if err != nil {
		return "", err
	}
	var response struct {
		Name string `json:"name"`
	}
	if err = json.Unmarshal(raw, &response); err != nil {
		return "", fmt.Errorf("decode host auth delete: %w", err)
	}
	return response.Name, nil
}

func (rpcInspectionGateway) FetchQuota(_ context.Context, authIndex string) (inspection.HostQuotaResponse, error) {
	raw, err := callHost(methodHostAuthQuotaFetch, map[string]any{"auth_index": authIndex})
	if err != nil {
		return inspection.HostQuotaResponse{}, err
	}
	var response inspection.HostQuotaResponse
	if err = json.Unmarshal(raw, &response); err != nil {
		return response, fmt.Errorf("decode host auth quota: %w", err)
	}
	return response, nil
}

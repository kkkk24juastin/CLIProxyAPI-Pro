package embeddedusage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

type AccountInspectionState struct {
	Key           string `json:"key"`
	SchemaVersion int    `json:"schemaVersion"`
	Generation    int64  `json:"generation"`
	Payload       []byte `json:"payload"`
	UpdatedAtMS   int64  `json:"updatedAtMs"`
}

func (s *Store) GetAccountInspectionState(ctx context.Context, key string) (AccountInspectionState, bool, error) {
	if s == nil || s.db == nil {
		return AccountInspectionState{}, false, fmt.Errorf("usage store is unavailable")
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return AccountInspectionState{}, false, fmt.Errorf("inspection state key is required")
	}
	var state AccountInspectionState
	err := s.db.QueryRowContext(ctx, `select state_key, schema_version, generation, payload_json, updated_at_ms from account_inspection_state where state_key = ?`, key).Scan(
		&state.Key, &state.SchemaVersion, &state.Generation, &state.Payload, &state.UpdatedAtMS,
	)
	if err != nil {
		if isSQLNoRows(err) {
			return AccountInspectionState{}, false, nil
		}
		return AccountInspectionState{}, false, err
	}
	state.Payload = append([]byte(nil), state.Payload...)
	return state, true, nil
}

func (s *Store) SetAccountInspectionState(ctx context.Context, key string, schemaVersion int, payload []byte) (AccountInspectionState, error) {
	if s == nil || s.db == nil {
		return AccountInspectionState{}, fmt.Errorf("usage store is unavailable")
	}
	key = strings.TrimSpace(key)
	if key == "" || schemaVersion <= 0 || len(payload) == 0 {
		return AccountInspectionState{}, fmt.Errorf("valid inspection state key, schema and payload are required")
	}
	now := time.Now().UnixMilli()
	_, err := s.db.ExecContext(ctx, `insert into account_inspection_state(state_key, schema_version, generation, payload_json, updated_at_ms)
		values(?, ?, 1, ?, ?)
		on conflict(state_key) do update set
			schema_version = excluded.schema_version,
			generation = account_inspection_state.generation + 1,
			payload_json = excluded.payload_json,
			updated_at_ms = excluded.updated_at_ms`, key, schemaVersion, append([]byte(nil), payload...), now)
	if err != nil {
		return AccountInspectionState{}, err
	}
	state, found, err := s.GetAccountInspectionState(ctx, key)
	if err != nil || !found {
		return AccountInspectionState{}, err
	}
	return state, nil
}

func isSQLNoRows(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}

func (s *Service) AccountInspectionState(ctx context.Context, key string) (AccountInspectionState, bool, error) {
	if s == nil || s.store == nil {
		return AccountInspectionState{}, false, fmt.Errorf("usage service is unavailable")
	}
	return s.store.GetAccountInspectionState(ctx, key)
}

func (s *Service) SetAccountInspectionState(ctx context.Context, key string, schemaVersion int, payload []byte) (AccountInspectionState, error) {
	if s == nil || s.store == nil {
		return AccountInspectionState{}, fmt.Errorf("usage service is unavailable")
	}
	return s.store.SetAccountInspectionState(ctx, key, schemaVersion, payload)
}

func GetAccountInspectionState(ctx context.Context, key string) (AccountInspectionState, bool, error) {
	globalStateMu.RLock()
	service := globalService
	globalStateMu.RUnlock()
	if service == nil {
		return AccountInspectionState{}, false, fmt.Errorf("usage service is unavailable")
	}
	return service.AccountInspectionState(ctx, key)
}

func SetAccountInspectionState(ctx context.Context, key string, schemaVersion int, payload []byte) (AccountInspectionState, error) {
	globalStateMu.RLock()
	service := globalService
	globalStateMu.RUnlock()
	if service == nil {
		return AccountInspectionState{}, fmt.Errorf("usage service is unavailable")
	}
	return service.SetAccountInspectionState(ctx, key, schemaVersion, payload)
}

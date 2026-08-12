package inspection

import (
	"encoding/json"
	"fmt"
	"time"
)

const ResultSnapshotVersion = 1

type RunState string

const (
	RunStateIdle      RunState = "idle"
	RunStateRunning   RunState = "running"
	RunStatePaused    RunState = "paused"
	RunStateStopping  RunState = "stopping"
	RunStateStopped   RunState = "stopped"
	RunStateCompleted RunState = "completed"
	RunStatePartial   RunState = "partial"
	RunStateFailed    RunState = "failed"
)

type ResultSnapshot struct {
	Version        int               `json:"version"`
	State          RunState          `json:"state"`
	LastStartedAt  int64             `json:"lastStartedAt"`
	LastFinishedAt int64             `json:"lastFinishedAt"`
	LastError      string            `json:"lastError,omitempty"`
	Settings       Settings          `json:"settings"`
	Summary        Summary           `json:"summary"`
	HealthCounts   HealthCounts      `json:"healthCounts"`
	Results        []Result          `json:"results"`
	Confirmations  ConfirmationState `json:"confirmations,omitempty"`
}

func NormalizeSnapshotState(state RunState) RunState {
	switch state {
	case RunStateStopped, RunStateCompleted, RunStatePartial, RunStateFailed:
		return state
	default:
		return RunStateCompleted
	}
}

func DecodeResultSnapshot(raw []byte, now time.Time) (ResultSnapshot, error) {
	var snapshot ResultSnapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return ResultSnapshot{}, err
	}
	if snapshot.Version != ResultSnapshotVersion {
		return ResultSnapshot{}, fmt.Errorf("unsupported account inspection snapshot version %d", snapshot.Version)
	}
	if snapshot.LastFinishedAt <= 0 {
		return ResultSnapshot{}, fmt.Errorf("account inspection snapshot is missing completion time")
	}
	if snapshot.LastStartedAt <= 0 || snapshot.LastStartedAt > snapshot.LastFinishedAt {
		snapshot.LastStartedAt = snapshot.LastFinishedAt
	}
	snapshot.State = NormalizeSnapshotState(snapshot.State)
	snapshot.Settings = NormalizeSchedule(Schedule{Settings: snapshot.Settings}, now).Settings
	for index := range snapshot.Results {
		snapshot.Results[index] = NormalizeResultSemantics(snapshot.Results[index])
	}
	snapshot.Results = SortResults(snapshot.Results)
	snapshot.HealthCounts = ResultHealthCounts(snapshot.Results)
	return snapshot, nil
}

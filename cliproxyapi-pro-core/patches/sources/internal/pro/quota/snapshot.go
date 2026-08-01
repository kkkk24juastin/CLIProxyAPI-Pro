package quota

import (
	"math"
	"reflect"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

const observationFutureSkew = 5 * time.Minute

func SnapshotMaxUsedPercent(snapshot pluginapi.QuotaSnapshot) (*float64, bool) {
	usedValues := make([]float64, 0, len(snapshot.Items))
	for _, item := range snapshot.Items {
		if item.UsedPercent != nil {
			usedValues = append(usedValues, math.Max(0, math.Min(100, *item.UsedPercent)))
			continue
		}
		if item.RemainingFraction != nil {
			remaining := math.Max(0, math.Min(1, *item.RemainingFraction))
			usedValues = append(usedValues, math.Max(0, math.Min(100, (1-remaining)*100)))
		}
	}
	if len(usedValues) == 0 {
		return nil, len(snapshot.Items) > 0
	}
	maximum := usedValues[0]
	for _, value := range usedValues[1:] {
		if value > maximum {
			maximum = value
		}
	}
	return &maximum, len(snapshot.Items) > 0
}

// NormalizeSnapshot applies the host contract shared by dynamic and built-in
// quota providers without coupling that policy to pluginhost lifecycle code.
func NormalizeSnapshot(snapshot pluginapi.QuotaSnapshot, provider string, previous *pluginapi.QuotaSnapshot, planUnavailable bool, planError string) pluginapi.QuotaSnapshot {
	if snapshot.SchemaVersion <= 0 {
		snapshot.SchemaVersion = pluginapi.QuotaSnapshotSchemaVersion
	}
	snapshot.Provider = provider
	now := time.Now().UnixMilli()
	if snapshot.ObservedAtMS <= 0 || snapshot.ObservedAtMS > now+observationFutureSkew.Milliseconds() {
		snapshot.ObservedAtMS = now
	}
	if snapshot.Items == nil {
		snapshot.Items = []pluginapi.QuotaItem{}
	}
	for index := range snapshot.Items {
		item := &snapshot.Items[index]
		item.ID = strings.TrimSpace(item.ID)
		item.Label = strings.TrimSpace(item.Label)
		item.RemainingFraction = clampFloatPointer(item.RemainingFraction, 0, 1)
		item.UsedPercent = clampFloatPointer(item.UsedPercent, 0, 100)
	}
	planError = strings.TrimSpace(planError)
	if planUnavailable && snapshot.Plan == nil && previous != nil && previous.Plan != nil {
		retained := *previous.Plan
		retained.Metadata = cloneSnapshotMap(previous.Plan.Metadata)
		retained.Stale = true
		retained.Error = planError
		snapshot.Plan = &retained
	}
	if planUnavailable && planError != "" {
		snapshot.Warnings = append(snapshot.Warnings, pluginapi.QuotaWarning{Code: "plan_unavailable", Message: planError, Retryable: true})
	}
	if snapshot.Plan != nil && snapshot.Plan.ObservedAtMS > now+observationFutureSkew.Milliseconds() {
		snapshot.Plan.ObservedAtMS = snapshot.ObservedAtMS
	}
	return snapshot
}

// CloneSnapshot protects provider callbacks from mutating the host's cached
// snapshot while retaining every nested metadata collection.
func CloneSnapshot(snapshot *pluginapi.QuotaSnapshot) *pluginapi.QuotaSnapshot {
	if snapshot == nil {
		return nil
	}
	clone := *snapshot
	clone.Items = append([]pluginapi.QuotaItem(nil), snapshot.Items...)
	for index := range clone.Items {
		clone.Items[index].ModelIDs = append([]string(nil), snapshot.Items[index].ModelIDs...)
		clone.Items[index].Metadata = cloneSnapshotMap(snapshot.Items[index].Metadata)
		clone.Items[index].RemainingFraction = cloneFloatPointer(snapshot.Items[index].RemainingFraction)
		clone.Items[index].UsedPercent = cloneFloatPointer(snapshot.Items[index].UsedPercent)
		clone.Items[index].RemainingAmount = cloneFloatPointer(snapshot.Items[index].RemainingAmount)
		clone.Items[index].Limit = cloneFloatPointer(snapshot.Items[index].Limit)
	}
	clone.Warnings = append([]pluginapi.QuotaWarning(nil), snapshot.Warnings...)
	clone.Metadata = cloneSnapshotMap(snapshot.Metadata)
	if snapshot.Plan != nil {
		plan := *snapshot.Plan
		plan.Metadata = cloneSnapshotMap(snapshot.Plan.Metadata)
		plan.CreditBalance = cloneFloatPointer(snapshot.Plan.CreditBalance)
		clone.Plan = &plan
	}
	return &clone
}

func cloneSnapshotMap(source map[string]any) map[string]any {
	if len(source) == 0 {
		return nil
	}
	clone := make(map[string]any, len(source))
	for key, value := range source {
		clone[key] = cloneSnapshotValue(value)
	}
	return clone
}

func cloneSnapshotValue(value any) any {
	clone := cloneSnapshotReflect(reflect.ValueOf(value))
	if !clone.IsValid() {
		return nil
	}
	return clone.Interface()
}

func cloneSnapshotReflect(value reflect.Value) reflect.Value {
	if !value.IsValid() {
		return value
	}
	switch value.Kind() {
	case reflect.Interface:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		clone := cloneSnapshotReflect(value.Elem())
		wrapped := reflect.New(value.Type()).Elem()
		wrapped.Set(clone)
		return wrapped
	case reflect.Map:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		clone := reflect.MakeMapWithSize(value.Type(), value.Len())
		iterator := value.MapRange()
		for iterator.Next() {
			clone.SetMapIndex(iterator.Key(), cloneSnapshotReflect(iterator.Value()))
		}
		return clone
	case reflect.Slice:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		clone := reflect.MakeSlice(value.Type(), value.Len(), value.Len())
		for index := 0; index < value.Len(); index++ {
			clone.Index(index).Set(cloneSnapshotReflect(value.Index(index)))
		}
		return clone
	case reflect.Array:
		clone := reflect.New(value.Type()).Elem()
		for index := 0; index < value.Len(); index++ {
			clone.Index(index).Set(cloneSnapshotReflect(value.Index(index)))
		}
		return clone
	case reflect.Pointer:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		clone := reflect.New(value.Type().Elem())
		clone.Elem().Set(cloneSnapshotReflect(value.Elem()))
		return clone
	default:
		return value
	}
}

func cloneFloatPointer(value *float64) *float64 {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func clampFloatPointer(value *float64, minValue, maxValue float64) *float64 {
	if value == nil {
		return nil
	}
	clamped := *value
	if clamped < minValue {
		clamped = minValue
	}
	if clamped > maxValue {
		clamped = maxValue
	}
	return &clamped
}

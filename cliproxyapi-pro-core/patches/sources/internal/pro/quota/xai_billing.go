package quota

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

func billingString(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		return ""
	}
}

func billingFirstAny(data map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := data[key]; ok {
			return value
		}
	}
	return nil
}
func billingFirstMap(data map[string]any, keys ...string) map[string]any {
	for _, key := range keys {
		if value, ok := data[key].(map[string]any); ok {
			return value
		}
	}
	return nil
}
func billingFirstString(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
func billingSlice(value any) []any {
	switch items := value.(type) {
	case []any:
		return items
	case []map[string]any:
		out := make([]any, 0, len(items))
		for _, item := range items {
			out = append(out, item)
		}
		return out
	default:
		return nil
	}
}
func billingFloat(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case json.Number:
		parsed, err := typed.Float64()
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}
func billingBool(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(typed, "true") || typed == "1"
	case float64:
		return typed != 0
	default:
		return false
	}
}
func billingFloatPtrAny(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}
func billingMax(values []float64) *float64 {
	if len(values) == 0 {
		return nil
	}
	maximum := values[0]
	for _, value := range values[1:] {
		if value > maximum {
			maximum = value
		}
	}
	return &maximum
}

func xaiPeriodInstants(periodStart string, periodEnd string) (any, any) {
	parse := func(value string) (time.Time, bool) {
		value = strings.TrimSpace(value)
		if value == "" {
			return time.Time{}, false
		}
		parsed, err := time.Parse(time.RFC3339Nano, value)
		return parsed, err == nil
	}
	end, hasEnd := parse(periodEnd)
	if !hasEnd {
		return nil, nil
	}
	var periodHours any
	if start, hasStart := parse(periodStart); hasStart && end.After(start) {
		periodHours = end.Sub(start).Hours()
	}
	return end.UnixMilli(), periodHours
}

func BuildXAIBillingSummary(body string) (map[string]any, *float64, error) {
	var payload map[string]any
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		return nil, nil, err
	}
	config := billingFirstMap(payload, "config")
	if config == nil {
		return nil, nil, fmt.Errorf("empty xai billing config")
	}
	return BuildXAIBillingSummaryFromConfig(config)
}

func BuildXAIBillingSummaryFromConfig(config map[string]any) (map[string]any, *float64, error) {
	monthlyLimitCents, hasMonthlyLimit := xaiCentValue(billingFirstAny(config, "monthlyLimit", "monthly_limit"))
	usedCents, hasUsed := xaiCentValue(config["used"])
	onDemandCapCents, hasOnDemandCap := xaiCentValue(billingFirstAny(config, "onDemandCap", "on_demand_cap"))
	explicitOnDemandUsedCents, hasExplicitOnDemandUsed := xaiCentValue(billingFirstAny(config, "onDemandUsed", "on_demand_used"))
	currentPeriod := billingFirstMap(config, "currentPeriod", "current_period")
	periodType := xaiPeriodType(currentPeriod)
	creditUsagePercent, hasCreditUsagePercent := billingFloat(billingFirstAny(config, "creditUsagePercent", "credit_usage_percent"))
	productUsage := xaiProductUsage(billingFirstAny(config, "productUsage", "product_usage"))
	billingPeriodStart := billingFirstString(billingString(billingFirstAny(config, "billingPeriodStart", "billing_period_start")))
	billingPeriodEnd := billingFirstString(billingString(billingFirstAny(config, "billingPeriodEnd", "billing_period_end")))
	periodStart := billingFirstString(billingString(currentPeriod["start"]), billingPeriodStart)
	periodEnd := billingFirstString(billingString(currentPeriod["end"]), billingPeriodEnd)
	hasWeeklyData := hasCreditUsagePercent || periodType == "weekly" || len(productUsage) > 0
	hasMonthlyData := hasMonthlyLimit || hasUsed || (!hasWeeklyData && (hasOnDemandCap || billingPeriodEnd != ""))
	if !hasWeeklyData && !hasMonthlyData {
		return nil, nil, fmt.Errorf("empty xai billing config")
	}
	summary := map[string]any{
		"periodType":          "unknown",
		"usagePercent":        nil,
		"productUsage":        productUsage,
		"monthlyLimitCents":   nil,
		"usedCents":           nil,
		"includedUsedCents":   nil,
		"onDemandCapCents":    nil,
		"onDemandUsedCents":   nil,
		"onDemandUsedPercent": nil,
		"usedPercent":         nil,
	}
	var includedUsedCents *float64
	if hasUsed {
		value := usedCents
		if hasMonthlyLimit && monthlyLimitCents > 0 {
			value = math.Min(usedCents, monthlyLimitCents)
		}
		includedUsedCents = &value
	}
	var onDemandUsedCents *float64
	if hasExplicitOnDemandUsed {
		value := explicitOnDemandUsedCents
		onDemandUsedCents = &value
	} else if hasUsed && hasMonthlyLimit {
		value := math.Max(0, usedCents-monthlyLimitCents)
		onDemandUsedCents = &value
	}
	var usedPercent *float64
	if hasMonthlyLimit && monthlyLimitCents > 0 && includedUsedCents != nil {
		value := (*includedUsedCents / monthlyLimitCents) * 100
		usedPercent = &value
	}
	var onDemandUsedPercent *float64
	if hasOnDemandCap && onDemandCapCents > 0 && onDemandUsedCents != nil {
		value := (*onDemandUsedCents / onDemandCapCents) * 100
		onDemandUsedPercent = &value
	}
	if hasWeeklyData {
		if periodType == "unknown" {
			summary["periodType"] = "weekly"
		} else {
			summary["periodType"] = periodType
		}
		if hasCreditUsagePercent {
			summary["usagePercent"] = creditUsagePercent
		}
		if periodStart != "" {
			summary["periodStart"] = periodStart
		}
		if periodEnd != "" {
			summary["periodEnd"] = periodEnd
		}
	} else {
		summary["periodType"] = "monthly"
		summary["usagePercent"] = billingFloatPtrAny(usedPercent)
		if billingPeriodStart != "" {
			summary["periodStart"] = billingPeriodStart
		}
		if billingPeriodEnd != "" {
			summary["periodEnd"] = billingPeriodEnd
		}
	}
	if hasMonthlyLimit {
		summary["monthlyLimitCents"] = monthlyLimitCents
	}
	if hasUsed {
		summary["usedCents"] = usedCents
	}
	if includedUsedCents != nil {
		summary["includedUsedCents"] = *includedUsedCents
	}
	if hasOnDemandCap {
		summary["onDemandCapCents"] = onDemandCapCents
	}
	if onDemandUsedCents != nil {
		summary["onDemandUsedCents"] = *onDemandUsedCents
	}
	if onDemandUsedPercent != nil {
		summary["onDemandUsedPercent"] = *onDemandUsedPercent
	}
	if usedPercent != nil {
		summary["usedPercent"] = *usedPercent
	}
	if hasMonthlyData && billingPeriodStart != "" {
		summary["billingPeriodStart"] = billingPeriodStart
	}
	if hasMonthlyData && billingPeriodEnd != "" {
		summary["billingPeriodEnd"] = billingPeriodEnd
	}
	resetAtMS, periodHours := xaiPeriodInstants(billingString(summary["periodStart"]), billingString(summary["periodEnd"]))
	summary["resetAtMs"] = resetAtMS
	summary["periodHours"] = periodHours
	return summary, XAISummaryUsedPercent(summary), nil
}

func xaiCentValue(value any) (float64, bool) {
	if mapped, ok := value.(map[string]any); ok {
		return billingFloat(mapped["val"])
	}
	return billingFloat(value)
}

func xaiPeriodType(period map[string]any) string {
	raw := strings.ToLower(billingString(period["type"]))
	switch {
	case strings.Contains(raw, "weekly"):
		return "weekly"
	case strings.Contains(raw, "monthly"):
		return "monthly"
	default:
		return "unknown"
	}
}

func xaiProductUsage(value any) []map[string]any {
	items := make([]map[string]any, 0)
	for i, raw := range billingSlice(value) {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		product := billingFirstString(billingString(item["product"]), fmt.Sprintf("Product %d", i+1))
		usagePercent, hasUsagePercent := billingFloat(billingFirstAny(item, "usagePercent", "usage_percent"))
		row := map[string]any{"product": product, "usagePercent": nil}
		if hasUsagePercent {
			row["usagePercent"] = usagePercent
		}
		items = append(items, row)
	}
	return items
}

func MergeXAIBillingSummaries(primary map[string]any, fallback map[string]any) map[string]any {
	if primary == nil {
		return fallback
	}
	if fallback == nil {
		return primary
	}
	periodSummary := primary
	if billingString(primary["periodType"]) == "" || billingString(primary["periodType"]) == "unknown" {
		if fallbackType := billingString(fallback["periodType"]); fallbackType != "" && fallbackType != "unknown" {
			periodSummary = fallback
		}
	}
	periodStart := billingString(periodSummary["periodStart"])
	periodEnd := billingString(periodSummary["periodEnd"])
	resetAtMS, periodHours := xaiPeriodInstants(periodStart, periodEnd)
	merged := map[string]any{
		"periodType":          firstKnownXaiPeriodType(primary["periodType"], fallback["periodType"]),
		"usagePercent":        firstNonNilAny(primary["usagePercent"], fallback["usagePercent"]),
		"productUsage":        firstNonEmptyXaiProductUsage(primary["productUsage"], fallback["productUsage"]),
		"monthlyLimitCents":   firstNonNilAny(primary["monthlyLimitCents"], fallback["monthlyLimitCents"]),
		"usedCents":           firstNonNilAny(primary["usedCents"], fallback["usedCents"]),
		"includedUsedCents":   firstNonNilAny(primary["includedUsedCents"], fallback["includedUsedCents"]),
		"onDemandCapCents":    firstNonNilAny(primary["onDemandCapCents"], fallback["onDemandCapCents"]),
		"onDemandUsedCents":   firstNonNilAny(primary["onDemandUsedCents"], fallback["onDemandUsedCents"]),
		"onDemandUsedPercent": firstNonNilAny(primary["onDemandUsedPercent"], fallback["onDemandUsedPercent"]),
		"billingPeriodStart":  firstNonNilAny(primary["billingPeriodStart"], fallback["billingPeriodStart"]),
		"billingPeriodEnd":    firstNonNilAny(primary["billingPeriodEnd"], fallback["billingPeriodEnd"]),
		"usedPercent":         firstNonNilAny(primary["usedPercent"], fallback["usedPercent"]),
		"resetAtMs":           resetAtMS,
		"periodHours":         periodHours,
	}
	if periodStart != "" {
		merged["periodStart"] = periodStart
	}
	if periodEnd != "" {
		merged["periodEnd"] = periodEnd
	}
	return merged
}

func firstKnownXaiPeriodType(primary any, fallback any) any {
	if value := billingString(primary); value != "" && value != "unknown" {
		return value
	}
	if value := billingString(fallback); value != "" {
		return value
	}
	return "unknown"
}

func firstNonNilAny(primary any, fallback any) any {
	if primary != nil {
		return primary
	}
	return fallback
}

func firstNonEmptyXaiProductUsage(primary any, fallback any) any {
	if len(billingSlice(primary)) > 0 {
		return primary
	}
	if len(billingSlice(fallback)) > 0 {
		return fallback
	}
	return []map[string]any{}
}

func XAISummaryUsedPercent(summary map[string]any) *float64 {
	if strings.EqualFold(strings.TrimSpace(billingString(summary["planType"])), "free") {
		freeQuota := billingFirstMap(summary, "freeQuota", "free_quota")
		if billingBool(freeQuota["exhausted"]) {
			value := 100.0
			return &value
		}
		if used, okUsed := billingFloat(billingFirstAny(freeQuota, "usedTokens", "used_tokens")); okUsed {
			if limit, okLimit := billingFloat(billingFirstAny(freeQuota, "limitTokens", "limit_tokens")); okLimit && limit > 0 {
				value := math.Max(0, math.Min(100, (used/limit)*100))
				return &value
			}
		}
	}
	for _, key := range []string{"usagePercent", "usage_percent", "usedPercent", "used_percent", "onDemandUsedPercent", "on_demand_used_percent"} {
		if value, ok := billingFloat(summary[key]); ok {
			return &value
		}
	}
	values := make([]float64, 0)
	for _, raw := range billingSlice(billingFirstAny(summary, "productUsage", "product_usage")) {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if value, ok := billingFloat(billingFirstAny(item, "usagePercent", "usage_percent")); ok {
			values = append(values, value)
		}
	}
	return billingMax(values)
}

const (
	xaiSuperGrokLimitCents      = 15_000
	xaiXPremiumPlusLimitCents   = 20_000
	xaiSuperGrokHeavyLimitCents = 150_000
)

func EmptyXAIBillingSummary() map[string]any {
	return map[string]any{
		"periodType":          "unknown",
		"usagePercent":        nil,
		"productUsage":        []map[string]any{},
		"monthlyLimitCents":   nil,
		"usedCents":           nil,
		"includedUsedCents":   nil,
		"onDemandCapCents":    nil,
		"onDemandUsedCents":   nil,
		"onDemandUsedPercent": nil,
		"usedPercent":         nil,
	}
}

func XAIPaidHealthSummary() map[string]any {
	summary := EmptyXAIBillingSummary()
	summary["mode"] = "paid-health"
	summary["source"] = "api.x.ai"
	summary["planType"] = "paid"
	summary["healthStatus"] = "chat-ok"
	return summary
}

func XAIPlanTypeFromBillingBody(status int, body string) (string, bool) {
	if status < 200 || status >= 300 {
		return "", false
	}
	var payload map[string]any
	if json.Unmarshal([]byte(body), &payload) != nil {
		return "", false
	}
	config := billingFirstMap(payload, "config")
	if config == nil {
		return "", false
	}
	limit, hasLimit := xaiCentValue(billingFirstAny(config, "monthlyLimit", "monthly_limit"))
	return XAIPlanTypeFromMonthlyLimit(limit, hasLimit)
}

func XAIPlanTypeFromMonthlyLimit(limit float64, hasLimit bool) (string, bool) {
	if !hasLimit || math.Round(limit) == 0 {
		return "free", true
	}
	switch int64(math.Round(limit)) {
	case xaiSuperGrokLimitCents:
		return "supergrok", true
	case xaiXPremiumPlusLimitCents:
		return "x-premium-plus", true
	case xaiSuperGrokHeavyLimitCents:
		return "supergrok-heavy", true
	default:
		return "paid-unknown", true
	}
}

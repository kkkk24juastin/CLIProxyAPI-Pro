package quota

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

const geminiCLIGoogleOneAI = "GOOGLE_ONE_AI"

func GeminiCLIFirstProjectID(value string) string {
	for _, part := range strings.Split(value, ",") {
		if projectID := strings.TrimSpace(part); projectID != "" {
			return projectID
		}
	}
	return ""
}

type geminiCLIBucket struct {
	modelID, tokenType, resetAt        string
	remainingFraction, remainingAmount *float64
}

type geminiCLIGroup struct {
	id, label string
}

func geminiCLIGroupForModel(modelID string) geminiCLIGroup {
	normalized := strings.ToLower(strings.TrimSpace(modelID))
	if strings.HasPrefix(normalized, "gemini-") {
		switch {
		case strings.Contains(normalized, "-flash-lite"):
			return geminiCLIGroup{id: "gemini-flash-lite-series", label: "Gemini Flash Lite Series"}
		case strings.Contains(normalized, "-flash"):
			return geminiCLIGroup{id: "gemini-flash-series", label: "Gemini Flash Series"}
		case strings.Contains(normalized, "-pro"):
			return geminiCLIGroup{id: "gemini-pro-series", label: "Gemini Pro Series"}
		}
	}
	return geminiCLIGroup{id: modelID, label: modelID}
}

func GeminiCLIQuotaItems(payload map[string]any) []pluginapi.QuotaItem {
	order := map[string]int{
		"gemini-flash-lite-series": 0,
		"gemini-flash-series":      1,
		"gemini-pro-series":        2,
	}
	type aggregate struct {
		group   geminiCLIGroup
		token   string
		buckets []geminiCLIBucket
	}
	aggregates := make(map[string]*aggregate)
	for _, bucket := range geminiCLIParseBuckets(payload["buckets"]) {
		if bucket.modelID == "gemini-2.0-flash" || strings.HasPrefix(bucket.modelID, "gemini-2.0-flash-") {
			continue
		}
		group := geminiCLIGroupForModel(bucket.modelID)
		key := group.id + "\x00" + bucket.tokenType
		if aggregates[key] == nil {
			aggregates[key] = &aggregate{group: group, token: bucket.tokenType}
		}
		aggregates[key].buckets = append(aggregates[key].buckets, bucket)
	}

	items := make([]pluginapi.QuotaItem, 0, len(aggregates))
	for _, aggregate := range aggregates {
		chosen := aggregate.buckets[0]
		for _, bucket := range aggregate.buckets[1:] {
			if geminiCLILessNullable(bucket.remainingFraction, chosen.remainingFraction) {
				chosen = bucket
			}
		}
		modelIDs := make([]string, 0, len(aggregate.buckets))
		for _, bucket := range aggregate.buckets {
			modelIDs = append(modelIDs, bucket.modelID)
		}
		sort.Strings(modelIDs)
		id := aggregate.group.id
		if aggregate.token != "" {
			id += "-" + aggregate.token
		}
		items = append(items, pluginapi.QuotaItem{
			ID: id, Label: aggregate.group.label, Kind: "model",
			RemainingFraction: chosen.remainingFraction, RemainingAmount: chosen.remainingAmount,
			ResetAt: chosen.resetAt, ModelIDs: modelIDs,
			Metadata: map[string]any{"token_type": aggregate.token},
		})
	}
	sort.SliceStable(items, func(i, j int) bool {
		leftID := strings.TrimSuffix(items[i].ID, "-"+geminiCLIString(items[i].Metadata["token_type"]))
		rightID := strings.TrimSuffix(items[j].ID, "-"+geminiCLIString(items[j].Metadata["token_type"]))
		left, leftKnown := order[leftID]
		right, rightKnown := order[rightID]
		if leftKnown != rightKnown {
			return leftKnown
		}
		if leftKnown && left != right {
			return left < right
		}
		return items[i].ID < items[j].ID
	})
	return items
}

func geminiCLIParseBuckets(value any) []geminiCLIBucket {
	rows, _ := value.([]any)
	out := make([]geminiCLIBucket, 0, len(rows))
	for _, value := range rows {
		row, okRow := value.(map[string]any)
		if !okRow {
			continue
		}
		modelID := strings.TrimSuffix(geminiCLIFirstString(row, "modelId", "model_id"), "_vertex")
		if modelID == "" {
			continue
		}
		remainingFraction := geminiCLIFirstNumber(row, "remainingFraction", "remaining_fraction")
		remainingAmount := geminiCLIFirstNumber(row, "remainingAmount", "remaining_amount")
		resetAt := geminiCLIFirstString(row, "resetTime", "reset_time")
		if remainingFraction == nil && ((remainingAmount != nil && *remainingAmount <= 0) || resetAt != "") {
			zero := 0.0
			remainingFraction = &zero
		}
		out = append(out, geminiCLIBucket{
			modelID: modelID, tokenType: geminiCLIFirstString(row, "tokenType", "token_type"),
			resetAt: resetAt, remainingFraction: remainingFraction, remainingAmount: remainingAmount,
		})
	}
	return out
}

func GeminiCLIQuotaPlan(payload map[string]any, observedAt int64) *pluginapi.QuotaPlan {
	payload = geminiCLIUnwrapPayload(payload)
	tier := geminiCLIFirstRecord(payload, "paidTier", "paid_tier")
	if strings.TrimSpace(geminiCLIString(tier["id"])) == "" {
		tier = geminiCLIFirstRecord(payload, "currentTier", "current_tier")
	}
	if strings.TrimSpace(geminiCLIString(tier["id"])) == "" {
		tier = geminiCLIDefaultAllowedTier(payload)
	}
	if tier == nil {
		return nil
	}
	id := strings.TrimSpace(geminiCLIString(tier["id"]))
	label := strings.TrimSpace(geminiCLIString(tier["name"]))
	if label == "" {
		label = id
	}
	plan := &pluginapi.QuotaPlan{ID: id, Label: label, Kind: geminiCLIPlanKind(id), ObservedAtMS: observedAt}
	if credits, okCredits := geminiCLIFirstValue(tier, "availableCredits", "available_credits").([]any); okCredits {
		for _, value := range credits {
			row, _ := value.(map[string]any)
			if geminiCLIFirstString(row, "creditType", "credit_type") == geminiCLIGoogleOneAI {
				plan.CreditBalance = geminiCLIFirstNumber(row, "creditAmount", "credit_amount")
				break
			}
		}
	}
	return plan
}

func geminiCLIUnwrapPayload(payload map[string]any) map[string]any {
	if unwrapped := geminiCLIFindTierPayload(payload); unwrapped != nil {
		return unwrapped
	}
	return payload
}

func geminiCLIFindTierPayload(payload map[string]any) map[string]any {
	if payload == nil {
		return nil
	}
	for _, keys := range [][]string{{"paidTier", "paid_tier"}, {"currentTier", "current_tier"}} {
		tier := geminiCLIFirstRecord(payload, keys...)
		if strings.TrimSpace(geminiCLIString(tier["id"])) != "" {
			return payload
		}
	}
	if geminiCLIDefaultAllowedTier(payload) != nil {
		return payload
	}
	for _, key := range []string{"body", "bodyText", "data", "response", "result"} {
		switch nested := payload[key].(type) {
		case map[string]any:
			if unwrapped := geminiCLIFindTierPayload(nested); unwrapped != nil {
				return unwrapped
			}
		case string:
			var decoded map[string]any
			if json.Unmarshal([]byte(strings.TrimSpace(nested)), &decoded) == nil {
				if unwrapped := geminiCLIFindTierPayload(decoded); unwrapped != nil {
					return unwrapped
				}
			}
		}
	}
	return nil
}

func geminiCLIDefaultAllowedTier(payload map[string]any) map[string]any {
	tiers, _ := geminiCLIFirstValue(payload, "allowedTiers", "allowed_tiers").([]any)
	for _, value := range tiers {
		tier, _ := value.(map[string]any)
		isDefault, _ := geminiCLIFirstValue(tier, "isDefault", "is_default").(bool)
		if isDefault && strings.TrimSpace(geminiCLIString(tier["id"])) != "" {
			return tier
		}
	}
	return nil
}

func geminiCLIPlanKind(id string) string {
	return map[string]string{"free-tier": "free", "legacy-tier": "legacy", "standard-tier": "standard", "g1-pro-tier": "pro", "g1-ultra-tier": "ultra"}[strings.ToLower(id)]
}
func geminiCLIString(value any) string { text, _ := value.(string); return text }
func geminiCLIFirstString(row map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(geminiCLIString(row[key])); value != "" {
			return value
		}
	}
	return ""
}
func geminiCLIFirstNumber(row map[string]any, keys ...string) *float64 {
	for _, key := range keys {
		if value, okValue := row[key].(float64); okValue {
			return &value
		}
	}
	return nil
}
func geminiCLIFirstValue(row map[string]any, keys ...string) any {
	for _, key := range keys {
		if value := row[key]; value != nil {
			return value
		}
	}
	return nil
}
func geminiCLIFirstRecord(row map[string]any, keys ...string) map[string]any {
	value, _ := geminiCLIFirstValue(row, keys...).(map[string]any)
	return value
}
func geminiCLILessNullable(left, right *float64) bool {
	return left != nil && (right == nil || *left < *right)
}

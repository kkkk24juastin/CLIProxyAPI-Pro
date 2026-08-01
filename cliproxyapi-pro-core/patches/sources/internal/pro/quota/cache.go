package quota

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

func SuccessCacheState(parserVersion int, values map[string]any) map[string]any {
	state := map[string]any{
		"status":        "success",
		"schemaVersion": 2,
		"parserVersion": parserVersion,
		"cachedAt":      time.Now().UnixMilli(),
	}
	for key, value := range values {
		state[key] = value
	}
	return state
}

func JSONShapeHash(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	var payload any
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		return ""
	}
	shape, err := json.Marshal(jsonShape(payload))
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(shape)
	return hex.EncodeToString(sum[:])
}

func JSONShapeHashForBodies(bodies map[string]string) string {
	shape := make(map[string]any, len(bodies))
	for key, body := range bodies {
		body = strings.TrimSpace(body)
		if body == "" {
			continue
		}
		var payload any
		if err := json.Unmarshal([]byte(body), &payload); err != nil {
			continue
		}
		shape[key] = jsonShape(payload)
	}
	if len(shape) == 0 {
		return ""
	}
	encoded, err := json.Marshal(shape)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func jsonShape(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		out := make(map[string]any, len(keys))
		for _, key := range keys {
			out[key] = jsonShape(typed[key])
		}
		return out
	case []any:
		if len(typed) == 0 {
			return []any{}
		}
		return []any{jsonShape(typed[0])}
	case string:
		return "string"
	case float64:
		return "number"
	case bool:
		return "bool"
	case nil:
		return "null"
	default:
		return fmt.Sprintf("%T", value)
	}
}

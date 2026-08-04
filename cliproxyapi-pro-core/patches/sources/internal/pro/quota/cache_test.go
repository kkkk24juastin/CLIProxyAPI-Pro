package quota

import "testing"

func TestSuccessCacheState(t *testing.T) {
	state := SuccessCacheState(7, map[string]any{"rows": []any{"value"}})
	if state["status"] != "success" || state["schemaVersion"] != 2 || state["parserVersion"] != 7 {
		t.Fatalf("state = %#v", state)
	}
	if cachedAt, ok := state["cachedAt"].(int64); !ok || cachedAt <= 0 {
		t.Fatalf("cachedAt = %#v", state["cachedAt"])
	}
}

func TestJSONShapeHashIgnoresValuesAndObjectOrder(t *testing.T) {
	left := JSONShapeHash(`{"name":"first","items":[{"used":1,"active":true}]}`)
	right := JSONShapeHash(`{"items":[{"active":false,"used":99}],"name":"second"}`)
	if left == "" || left != right {
		t.Fatalf("shape hashes = %q and %q", left, right)
	}
	if got := JSONShapeHash(`{"items":[]}`); got == left {
		t.Fatalf("different shapes have same hash %q", got)
	}
}

func TestJSONShapeHashForBodiesIsStable(t *testing.T) {
	left := JSONShapeHashForBodies(map[string]string{"weekly": `{"used":1}`, "monthly": `{"limit":2}`})
	right := JSONShapeHashForBodies(map[string]string{"monthly": `{"limit":9}`, "weekly": `{"used":4}`})
	if left == "" || left != right {
		t.Fatalf("shape hashes = %q and %q", left, right)
	}
}

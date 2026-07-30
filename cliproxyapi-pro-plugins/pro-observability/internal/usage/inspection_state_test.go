package embeddedusage

import (
	"context"
	"testing"
)

func TestAccountInspectionStatePersistsAndIncrementsGeneration(t *testing.T) {
	store, err := OpenStore(t.TempDir() + "/usage.sqlite")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	first, err := store.SetAccountInspectionState(context.Background(), "schedule", 1, []byte(`{"enabled":true}`))
	if err != nil {
		t.Fatalf("set first state: %v", err)
	}
	second, err := store.SetAccountInspectionState(context.Background(), "schedule", 1, []byte(`{"enabled":false}`))
	if err != nil {
		t.Fatalf("set second state: %v", err)
	}
	if first.Generation != 1 || second.Generation != 2 || string(second.Payload) != `{"enabled":false}` {
		t.Fatalf("states = first:%#v second:%#v", first, second)
	}
	loaded, found, err := store.GetAccountInspectionState(context.Background(), "schedule")
	if err != nil || !found || loaded.Generation != 2 {
		t.Fatalf("loaded = %#v found=%v err=%v", loaded, found, err)
	}
}

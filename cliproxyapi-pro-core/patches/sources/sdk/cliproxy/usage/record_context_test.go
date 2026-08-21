package usage

import (
	"context"
	"net/http"
	"testing"
)

func TestRecordSnapshotFromContextIsIsolated(t *testing.T) {
	generate := false
	attempt := int64(2)
	source := Record{
		Provider:        "codex",
		Generate:        &generate,
		AttemptIndex:    &attempt,
		ResponseHeaders: http.Header{"X-Test": []string{"before"}},
	}
	ctx := WithRecordSnapshot(context.Background(), source)

	source.Provider = "changed"
	*source.Generate = true
	*source.AttemptIndex = 9
	source.ResponseHeaders.Set("X-Test", "changed")

	first, ok := RecordFromContext(ctx)
	if !ok {
		t.Fatal("RecordFromContext() = false")
	}
	if first.Provider != "codex" || first.Generate == nil || *first.Generate || first.AttemptIndex == nil || *first.AttemptIndex != 2 || first.ResponseHeaders.Get("X-Test") != "before" {
		t.Fatalf("snapshot = %#v", first)
	}

	first.Provider = "mutated"
	*first.Generate = true
	*first.AttemptIndex = 7
	first.ResponseHeaders.Set("X-Test", "mutated")
	second, ok := RecordFromContext(ctx)
	if !ok || second.Provider != "codex" || second.Generate == nil || *second.Generate || second.AttemptIndex == nil || *second.AttemptIndex != 2 || second.ResponseHeaders.Get("X-Test") != "before" {
		t.Fatalf("second snapshot = %#v ok=%t", second, ok)
	}
}

type recordContextPlugin struct {
	record Record
	ok     bool
}

func (p *recordContextPlugin) HandleUsage(ctx context.Context, _ Record) {
	p.record, p.ok = RecordFromContext(ctx)
}

func TestManagerDispatchAttachesRecordSnapshot(t *testing.T) {
	manager := NewManager(1)
	plugin := &recordContextPlugin{}
	manager.Register(plugin)
	manager.dispatch(queueItem{
		ctx:    context.Background(),
		record: Record{Provider: "codex", ResponseHeaders: http.Header{"X-Test": []string{"value"}}},
	})
	if !plugin.ok || plugin.record.Provider != "codex" || plugin.record.ResponseHeaders.Get("X-Test") != "value" {
		t.Fatalf("dispatched snapshot = %#v ok=%t", plugin.record, plugin.ok)
	}
}

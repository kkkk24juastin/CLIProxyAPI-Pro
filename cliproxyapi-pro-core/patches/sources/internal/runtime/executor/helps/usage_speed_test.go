package helps

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/usage"
)

type captureUsageSpeedPlugin struct {
	records chan usage.Record
}

func (p *captureUsageSpeedPlugin) HandleUsage(_ context.Context, record usage.Record) {
	select {
	case p.records <- record:
	default:
	}
}

func TestParseClaudeUsageIncludesResponseSpeed(t *testing.T) {
	detail := ParseClaudeUsage([]byte(`{"usage":{"input_tokens":10,"output_tokens":2,"speed":"fast"}}`))
	if detail.ResponseSpeed != "fast" {
		t.Fatalf("ResponseSpeed = %q, want fast", detail.ResponseSpeed)
	}
}

func TestParseClaudeStreamUsageIncludesResponseSpeed(t *testing.T) {
	detail, ok := ParseClaudeStreamUsage([]byte(`data: {"type":"message_start","message":{"usage":{"input_tokens":10,"speed":"standard"}}}`))
	if !ok || detail.ResponseSpeed != "standard" {
		t.Fatalf("ParseClaudeStreamUsage() = (%+v, %v), want standard speed", detail, ok)
	}
}

func TestUsageReporterBuildRecordIncludesSpeed(t *testing.T) {
	ctx := usage.WithSpeed(context.Background(), "fast")
	reporter := NewUsageReporter(ctx, "claude", "claude-opus-test", nil)
	record := reporter.buildRecord(usage.Detail{TotalTokens: 3, ResponseSpeed: "standard"}, false)
	if record.Speed != "fast" || record.ResponseSpeed != "standard" {
		t.Fatalf("record speeds = %q/%q, want fast/standard", record.Speed, record.ResponseSpeed)
	}
}

func TestStreamUsageBufferPreservesResponseSpeedFromMessageStart(t *testing.T) {
	var buffer StreamUsageBuffer
	start, startOK := ParseClaudeStreamUsage([]byte(`data: {"type":"message_start","message":{"usage":{"input_tokens":10,"speed":"fast"}}}`))
	final, finalOK := ParseClaudeStreamUsage([]byte(`data: {"type":"message_delta","usage":{"output_tokens":2}}`))
	buffer.ObserveClaude(start, startOK)
	buffer.ObserveClaude(final, finalOK)
	detail, ok := buffer.Detail()
	if !ok || detail.InputTokens != 10 || detail.OutputTokens != 2 || detail.TotalTokens != 12 || detail.ResponseSpeed != "fast" {
		t.Fatalf("buffer detail = (%+v, %v), want merged input/output tokens with preserved fast speed", detail, ok)
	}
}

func TestStreamUsageBufferMergesClaudeUsageWithoutResponseSpeed(t *testing.T) {
	var buffer StreamUsageBuffer
	start, startOK := ParseClaudeStreamUsage([]byte(`data: {"type":"message_start","message":{"usage":{"input_tokens":10,"cache_read_input_tokens":3,"cache_creation_input_tokens":4}}}`))
	final, finalOK := ParseClaudeStreamUsage([]byte(`data: {"type":"message_delta","usage":{"output_tokens":2}}`))
	buffer.ObserveClaude(start, startOK)
	buffer.ObserveClaude(final, finalOK)
	detail, ok := buffer.Detail()
	if !ok || detail.InputTokens != 10 || detail.OutputTokens != 2 || detail.CacheReadTokens != 3 ||
		detail.CacheCreationTokens != 4 || detail.CachedTokens != 3 || detail.TotalTokens != 19 {
		t.Fatalf("buffer detail = (%+v, %v), want merged input/cache/output tokens without response speed", detail, ok)
	}
}

func TestStreamUsageBufferMergesClaudeCacheCreationWithoutCacheRead(t *testing.T) {
	var buffer StreamUsageBuffer
	start, startOK := ParseClaudeStreamUsage([]byte(`data: {"type":"message_start","message":{"usage":{"input_tokens":10,"cache_creation_input_tokens":4}}}`))
	final, finalOK := ParseClaudeStreamUsage([]byte(`data: {"type":"message_delta","usage":{"output_tokens":2}}`))
	buffer.ObserveClaude(start, startOK)
	buffer.ObserveClaude(final, finalOK)
	detail, ok := buffer.Detail()
	if !ok || detail.CacheCreationTokens != 4 || detail.CachedTokens != 4 || detail.TotalTokens != 16 {
		t.Fatalf("buffer detail = (%+v, %v), want preserved cache creation tokens", detail, ok)
	}
}

func TestStreamUsageBufferGenericObserveKeepsLatestUsageAuthoritative(t *testing.T) {
	var buffer StreamUsageBuffer
	buffer.Observe(usage.Detail{InputTokens: 10, TotalTokens: 10}, true)
	buffer.Observe(usage.Detail{OutputTokens: 2, TotalTokens: 2}, true)
	detail, ok := buffer.Detail()
	if !ok || detail.InputTokens != 0 || detail.OutputTokens != 2 || detail.TotalTokens != 2 {
		t.Fatalf("buffer detail = (%+v, %v), want final generic usage to remain authoritative", detail, ok)
	}
}

func TestStreamUsageBufferPublishFailurePreservesObservedUsage(t *testing.T) {
	plugin := &captureUsageSpeedPlugin{records: make(chan usage.Record, 4)}
	pluginName := "usage-speed-stream-failure"
	usage.RegisterNamedPlugin(pluginName, plugin)
	defer usage.UnregisterNamedPlugin(pluginName, plugin)

	ctx := context.Background()
	reporter := NewUsageReporter(ctx, "claude", "claude-stream-failure-test", nil)
	var buffer StreamUsageBuffer
	if buffer.PublishFailure(ctx, reporter, errors.New("before usage")) {
		t.Fatal("PublishFailure() = true before usage was observed")
	}
	buffer.ObserveClaude(usage.Detail{
		InputTokens:     10,
		CacheReadTokens: 3,
		TotalTokens:     13,
		ResponseSpeed:   "fast",
	}, true)
	if !buffer.PublishFailure(ctx, reporter, errors.New("stream canceled")) {
		t.Fatal("PublishFailure() = false after usage was observed")
	}

	deadline := time.After(2 * time.Second)
	var record usage.Record
	for record.Model != "claude-stream-failure-test" {
		select {
		case record = <-plugin.records:
		case <-deadline:
			t.Fatal("timed out waiting for failed usage record")
		}
	}
	if !record.Failed || record.Fail.Body != "stream canceled" {
		t.Fatalf("record failure = (%v, %q), want failed stream cancellation", record.Failed, record.Fail.Body)
	}
	if record.Detail.InputTokens != 10 || record.Detail.CacheReadTokens != 3 || record.ResponseSpeed != "fast" || record.Detail.ResponseSpeed != "fast" {
		t.Fatalf("record detail = %+v, response speed = %q; want preserved input/cache usage and fast speed", record.Detail, record.ResponseSpeed)
	}
	select {
	case duplicate := <-plugin.records:
		if duplicate.Model == "claude-stream-failure-test" {
			t.Fatalf("received duplicate usage record: %+v", duplicate)
		}
	case <-time.After(100 * time.Millisecond):
	}
}

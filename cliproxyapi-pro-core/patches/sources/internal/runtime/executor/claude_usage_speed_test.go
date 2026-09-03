package executor

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/usage"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v7/sdk/translator"
)

type captureClaudeUsageSpeedPlugin struct {
	records chan usage.Record
}

func (p *captureClaudeUsageSpeedPlugin) HandleUsage(_ context.Context, record usage.Record) {
	select {
	case p.records <- record:
	default:
	}
}

func TestClaudeExecutorStreamCancellationPreservesObservedUsageAndSpeed(t *testing.T) {
	testClaudeExecutorStreamCancellationPreservesObservedUsageAndSpeed(
		t,
		"oauth-stream-observed-usage-cancellation",
		"sk-ant-oat-stream-observed-usage",
		claudeOAuthCancellationTestMetadata(),
	)
}

func TestClaudeExecutorAPIStreamCancellationPreservesObservedUsageAndSpeed(t *testing.T) {
	testClaudeExecutorStreamCancellationPreservesObservedUsageAndSpeed(
		t,
		"api-stream-observed-usage-cancellation",
		"anthropic-api-key-stream-observed-usage",
		nil,
	)
}

func testClaudeExecutorStreamCancellationPreservesObservedUsageAndSpeed(t *testing.T, authID, apiKey string, metadata map[string]any) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "event: message_start\n"+
			`data: {"type":"message_start","message":{"id":"msg_usage_speed","model":"claude-opus-5","usage":{"input_tokens":10,"cache_read_input_tokens":3,"speed":"fast"}}}`+"\n\n")
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		<-r.Context().Done()
	}))
	defer server.Close()

	plugin := &captureClaudeUsageSpeedPlugin{records: make(chan usage.Record, 16)}
	pluginName := "claude-stream-observed-usage-cancellation-" + authID
	usage.RegisterNamedPlugin(pluginName, plugin)
	defer usage.UnregisterNamedPlugin(pluginName, plugin)

	auth := &cliproxyauth.Auth{
		ID: authID,
		Attributes: map[string]string{
			"api_key":  apiKey,
			"base_url": server.URL,
		},
		Metadata: metadata,
	}
	ctx, cancel := context.WithCancel(context.Background())
	result, errStream := NewClaudeExecutor(&config.Config{}).ExecuteStream(ctx, auth, cliproxyexecutor.Request{
		Model:   "claude-opus-5",
		Payload: []byte(`{"model":"claude-opus-5","messages":[{"role":"user","content":"hello"}],"stream":true}`),
	}, cliproxyexecutor.Options{
		Stream:         true,
		SourceFormat:   sdktranslator.FormatClaude,
		ResponseFormat: sdktranslator.FormatClaude,
	})
	if errStream != nil {
		cancel()
		t.Fatalf("ExecuteStream() error = %v", errStream)
	}

	select {
	case chunk, ok := <-result.Chunks:
		if !ok {
			cancel()
			t.Fatal("stream closed before message_start was forwarded")
		}
		if chunk.Err != nil {
			cancel()
			t.Fatalf("message_start chunk error = %v", chunk.Err)
		}
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("timed out waiting for message_start chunk")
	}
	cancel()
	for range result.Chunks {
	}

	record := waitForClaudeUsageSpeedRecord(t, plugin.records, authID)
	if !record.Failed {
		t.Fatalf("record.Failed = false, want true: %+v", record)
	}
	if record.Detail.InputTokens != 10 || record.Detail.CacheReadTokens != 3 {
		t.Fatalf("record usage = input %d/cache read %d, want 10/3", record.Detail.InputTokens, record.Detail.CacheReadTokens)
	}
	if record.ResponseSpeed != "fast" || record.Detail.ResponseSpeed != "fast" {
		t.Fatalf("record speeds = %q/%q, want fast/fast", record.ResponseSpeed, record.Detail.ResponseSpeed)
	}
	assertNoDuplicateClaudeUsageSpeedRecord(t, plugin.records, authID)
}

func waitForClaudeUsageSpeedRecord(t *testing.T, records <-chan usage.Record, authID string) usage.Record {
	t.Helper()
	timeout := time.After(2 * time.Second)
	for {
		select {
		case record := <-records:
			if record.AuthID == authID {
				return record
			}
		case <-timeout:
			t.Fatal("timed out waiting for Claude usage record")
		}
	}
}

func assertNoDuplicateClaudeUsageSpeedRecord(t *testing.T, records <-chan usage.Record, authID string) {
	t.Helper()
	timeout := time.After(100 * time.Millisecond)
	for {
		select {
		case record := <-records:
			if record.AuthID == authID {
				t.Fatalf("received duplicate Claude usage record: %+v", record)
			}
		case <-timeout:
			return
		}
	}
}

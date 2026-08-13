package pluginhost

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	coreexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	coreusage "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/usage"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v7/sdk/translator"
)

type pluginExecutorUsageRecorder struct {
	records chan coreusage.Record
}

func (r *pluginExecutorUsageRecorder) HandleUsage(_ context.Context, record coreusage.Record) {
	if record.Provider == "plugin-provider" {
		r.records <- record
	}
}

type pluginExecutorStatusError struct {
	status int
}

func (e pluginExecutorStatusError) Error() string   { return "plugin upstream failed" }
func (e pluginExecutorStatusError) StatusCode() int { return e.status }

func TestPluginExecutorPublishesNonStreamUsage(t *testing.T) {
	recorder := &pluginExecutorUsageRecorder{records: make(chan coreusage.Record, 4)}
	coreusage.RegisterNamedPlugin("plugin-executor-usage-test", recorder)
	defer coreusage.UnregisterNamedPlugin("plugin-executor-usage-test", recorder)
	host := newHostWithRecords(normalizeTestCapabilityRecord(capabilityRecord{id: "usage-executor"}))
	executeErr := pluginExecutorStatusError{status: http.StatusTooManyRequests}
	executor := &fakeExecutor{
		identifier: "plugin-provider",
		execute: func(context.Context, pluginapi.ExecutorRequest) (pluginapi.ExecutorResponse, error) {
			return pluginapi.ExecutorResponse{}, executeErr
		},
		countTokens: func(context.Context, pluginapi.ExecutorRequest) (pluginapi.ExecutorResponse, error) {
			return pluginapi.ExecutorResponse{Payload: []byte(`{"total_tokens":1}`)}, nil
		},
	}
	adapter := newCurrentExecutorAdapterForTest(
		host,
		"usage-executor",
		executor,
		[]sdktranslator.Format{sdktranslator.FormatOpenAI},
		[]sdktranslator.Format{sdktranslator.FormatOpenAI},
	)
	auth := &coreauth.Auth{ID: "plugin-auth", Index: "plugin-index", Provider: "plugin-provider"}
	ctx := coreusage.NextAttemptContext(context.Background())
	_, err := adapter.Execute(ctx, auth, coreexecutor.Request{Model: "plugin-model"}, coreexecutor.Options{})
	if !errors.Is(err, executeErr) {
		t.Fatalf("Execute() error = %v, want %v", err, executeErr)
	}
	record := waitForPluginExecutorUsage(t, recorder.records)
	if !record.Failed || record.Fail.StatusCode != http.StatusTooManyRequests || record.Fail.Body != executeErr.Error() {
		t.Fatalf("failure = %#v", record.Fail)
	}
	if record.AuthID != auth.ID || record.AuthIndex != auth.Index || record.Model != "plugin-model" {
		t.Fatalf("identity = %#v", record)
	}
	if record.AttemptIndex == nil || *record.AttemptIndex != 0 {
		t.Fatalf("attempt index = %#v", record.AttemptIndex)
	}
	if _, err = adapter.CountTokens(ctx, auth, coreexecutor.Request{Model: "plugin-model"}, coreexecutor.Options{}); err != nil {
		t.Fatal(err)
	}
	assertNoPluginExecutorUsage(t, recorder.records)
}

func TestPluginExecutorTranslationPanicPublishesFailure(t *testing.T) {
	recorder := &pluginExecutorUsageRecorder{records: make(chan coreusage.Record, 2)}
	coreusage.RegisterNamedPlugin("plugin-executor-translation-panic-test", recorder)
	defer coreusage.UnregisterNamedPlugin("plugin-executor-translation-panic-test", recorder)
	customOutput := sdktranslator.Format("plugin-panic-output")
	sdktranslator.Register(sdktranslator.FormatOpenAI, customOutput, nil, sdktranslator.ResponseTransform{
		NonStream: func(context.Context, string, []byte, []byte, []byte, *any) []byte {
			panic("forced response translation panic")
		},
	})
	host := newHostWithRecords(normalizeTestCapabilityRecord(capabilityRecord{id: "translation-panic-executor"}))
	adapter := newCurrentExecutorAdapterForTest(
		host,
		"translation-panic-executor",
		&fakeExecutor{
			identifier: "plugin-provider",
			execute: func(context.Context, pluginapi.ExecutorRequest) (pluginapi.ExecutorResponse, error) {
				return pluginapi.ExecutorResponse{Payload: []byte(`{"ok":true}`)}, nil
			},
		},
		[]sdktranslator.Format{sdktranslator.FormatOpenAI},
		[]sdktranslator.Format{customOutput},
	)
	_, err := adapter.Execute(context.Background(), &coreauth.Auth{ID: "panic-auth"}, coreexecutor.Request{
		Model: "panic-model", Format: sdktranslator.FormatOpenAI, Payload: []byte(`{"model":"panic-model"}`),
	}, coreexecutor.Options{SourceFormat: sdktranslator.FormatOpenAI, ResponseFormat: sdktranslator.FormatOpenAI})
	if err == nil {
		t.Fatal("Execute() error = nil, want recovered translation panic")
	}
	record := waitForPluginExecutorUsage(t, recorder.records)
	if !record.Failed || record.Fail.Body == "" {
		t.Fatalf("translation panic failure = %#v", record.Fail)
	}
}

func TestPluginExecutorStreamPublishesTerminalOutcome(t *testing.T) {
	for _, test := range []struct {
		name       string
		terminal   error
		wantFailed bool
	}{
		{name: "normal close"},
		{name: "terminal error", terminal: pluginExecutorStatusError{status: http.StatusServiceUnavailable}, wantFailed: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := &pluginExecutorUsageRecorder{records: make(chan coreusage.Record, 2)}
			coreusage.RegisterNamedPlugin("plugin-executor-stream-usage-test", recorder)
			defer coreusage.UnregisterNamedPlugin("plugin-executor-stream-usage-test", recorder)
			chunks := make(chan pluginapi.ExecutorStreamChunk, 2)
			chunks <- pluginapi.ExecutorStreamChunk{Payload: []byte("data")}
			if test.terminal != nil {
				chunks <- pluginapi.ExecutorStreamChunk{Err: test.terminal}
			}
			close(chunks)
			host := newHostWithRecords(normalizeTestCapabilityRecord(capabilityRecord{id: "stream-usage-executor"}))
			adapter := newCurrentExecutorAdapterForTest(
				host,
				"stream-usage-executor",
				&fakeExecutor{
					identifier: "plugin-provider",
					executeStream: func(context.Context, pluginapi.ExecutorRequest) (pluginapi.ExecutorStreamResponse, error) {
						return pluginapi.ExecutorStreamResponse{Headers: http.Header{"Retry-After": []string{"60"}}, Chunks: chunks}, nil
					},
				},
				[]sdktranslator.Format{sdktranslator.FormatOpenAI},
				[]sdktranslator.Format{sdktranslator.FormatOpenAI},
			)
			ctx := context.Background()
			result, err := adapter.ExecuteStream(ctx, &coreauth.Auth{ID: "stream-auth", Index: "stream-index"}, coreexecutor.Request{Model: "stream-model"}, coreexecutor.Options{})
			if err != nil {
				t.Fatal(err)
			}
			for range result.Chunks {
			}
			record := waitForPluginExecutorUsage(t, recorder.records)
			if record.Failed != test.wantFailed {
				t.Fatalf("Failed = %v, want %v", record.Failed, test.wantFailed)
			}
			if record.ResponseHeaders.Get("Retry-After") != "60" {
				t.Fatalf("response headers = %#v", record.ResponseHeaders)
			}
			if test.terminal != nil && record.Fail.StatusCode != http.StatusServiceUnavailable {
				t.Fatalf("failure = %#v", record.Fail)
			}
			assertNoPluginExecutorUsage(t, recorder.records)
		})
	}
}

func TestPluginExecutorEmptyStreamPublishesFailure(t *testing.T) {
	recorder := &pluginExecutorUsageRecorder{records: make(chan coreusage.Record, 2)}
	coreusage.RegisterNamedPlugin("plugin-executor-empty-stream-test", recorder)
	defer coreusage.UnregisterNamedPlugin("plugin-executor-empty-stream-test", recorder)
	chunks := make(chan pluginapi.ExecutorStreamChunk)
	close(chunks)
	host := newHostWithRecords(normalizeTestCapabilityRecord(capabilityRecord{id: "empty-stream-executor"}))
	adapter := newCurrentExecutorAdapterForTest(
		host,
		"empty-stream-executor",
		&fakeExecutor{
			identifier: "plugin-provider",
			executeStream: func(context.Context, pluginapi.ExecutorRequest) (pluginapi.ExecutorStreamResponse, error) {
				return pluginapi.ExecutorStreamResponse{Chunks: chunks}, nil
			},
		},
		[]sdktranslator.Format{sdktranslator.FormatOpenAI},
		[]sdktranslator.Format{sdktranslator.FormatOpenAI},
	)
	result, err := adapter.ExecuteStream(context.Background(), &coreauth.Auth{ID: "empty-stream-auth"}, coreexecutor.Request{Model: "empty-stream-model"}, coreexecutor.Options{})
	if err != nil {
		t.Fatal(err)
	}
	for range result.Chunks {
	}
	record := waitForPluginExecutorUsage(t, recorder.records)
	if !record.Failed || record.Fail.StatusCode != http.StatusBadGateway {
		t.Fatalf("empty stream failure = %#v", record.Fail)
	}
}

func waitForPluginExecutorUsage(t *testing.T, records <-chan coreusage.Record) coreusage.Record {
	t.Helper()
	select {
	case record := <-records:
		return record
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for plugin executor usage")
		return coreusage.Record{}
	}
}

func assertNoPluginExecutorUsage(t *testing.T, records <-chan coreusage.Record) {
	t.Helper()
	select {
	case record := <-records:
		t.Fatalf("unexpected usage record: %#v", record)
	case <-time.After(25 * time.Millisecond):
	}
}

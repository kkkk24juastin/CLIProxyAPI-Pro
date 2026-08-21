package pluginhost

import (
	"bytes"
	"context"
	"net/http"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/logging"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/runtime/executor/helps"
	coreexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	coreusage "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/usage"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v7/sdk/translator"
)

func pluginExecutorUsageContext(ctx context.Context, headers http.Header) context.Context {
	logging.SetResponseHeaders(ctx, headers)
	if len(headers) == 0 || len(logging.GetResponseHeaders(ctx)) > 0 {
		return ctx
	}
	ctx = logging.WithResponseHeadersHolder(ctx)
	logging.SetResponseHeaders(ctx, headers)
	return ctx
}

func trackPluginExecutorStreamUsage(ctx context.Context, in <-chan coreexecutor.StreamChunk, reporter *helps.UsageReporter, format sdktranslator.Format) <-chan coreexecutor.StreamChunk {
	if ctx == nil {
		ctx = context.Background()
	}
	out := make(chan coreexecutor.StreamChunk)
	go func() {
		defer close(out)
		sawPayload := false
		var usageBuffer helps.StreamUsageBuffer
		var terminalErr error
		for {
			select {
			case <-ctx.Done():
				if !usageBuffer.PublishFailure(ctx, reporter, ctx.Err()) {
					reporter.PublishFailure(ctx, ctx.Err())
				}
				return
			case chunk, ok := <-in:
				if !ok {
					if terminalErr != nil {
						if !usageBuffer.PublishFailure(ctx, reporter, terminalErr) {
							reporter.PublishFailure(ctx, terminalErr)
						}
					} else if sawPayload {
						if !usageBuffer.Publish(ctx, reporter) {
							reporter.EnsurePublished(ctx)
						}
					} else {
						reporter.PublishFailure(ctx, pluginExecutorEmptyStreamError{})
					}
					return
				}
				if chunk.Err != nil {
					terminalErr = chunk.Err
				} else if len(chunk.Payload) > 0 {
					sawPayload = true
					observePluginExecutorUsage(&usageBuffer, format, chunk.Payload, true)
				}
				select {
				case <-ctx.Done():
					if !usageBuffer.PublishFailure(ctx, reporter, ctx.Err()) {
						reporter.PublishFailure(ctx, ctx.Err())
					}
					return
				case out <- chunk:
				}
			}
		}
	}()
	return out
}

func publishPluginExecutorUsage(ctx context.Context, reporter *helps.UsageReporter, format sdktranslator.Format, payload []byte, stream bool) bool {
	var buffer helps.StreamUsageBuffer
	observePluginExecutorUsage(&buffer, format, payload, stream)
	return buffer.Publish(ctx, reporter)
}

func pluginExecutorUsageFormat(prepared preparedExecutorCall) sdktranslator.Format {
	if prepared.requestedFormat != "" {
		return prepared.requestedFormat
	}
	return prepared.outputFormat
}

func observePluginExecutorUsage(buffer *helps.StreamUsageBuffer, format sdktranslator.Format, payload []byte, stream bool) {
	if buffer == nil || len(payload) == 0 {
		return
	}
	payload = pluginExecutorJSONPayload(payload)
	if len(payload) == 0 {
		return
	}
	var detail coreusage.Detail
	var ok bool
	switch format {
	case sdktranslator.FormatCodex, sdktranslator.FormatOpenAIResponse:
		detail, ok = helps.ParseCodexUsage(payload)
	case sdktranslator.FormatClaude:
		if stream {
			detail, ok = helps.ParseClaudeStreamUsage(payload)
			buffer.ObserveClaude(detail, ok)
			return
		}
		detail = helps.ParseClaudeUsage(payload)
		ok = pluginExecutorUsageDetailPresent(detail)
	case sdktranslator.FormatGemini, sdktranslator.FormatAntigravity:
		if stream {
			detail, ok = helps.ParseGeminiStreamUsage(payload)
		} else {
			detail = helps.ParseGeminiUsage(payload)
			ok = pluginExecutorUsageDetailPresent(detail)
		}
	case sdktranslator.FormatInteractions:
		if stream {
			detail, ok = helps.ParseInteractionsStreamUsage(payload)
		} else {
			detail = helps.ParseInteractionsUsage(payload)
			ok = pluginExecutorUsageDetailPresent(detail)
		}
	default:
		if stream {
			detail, ok = helps.ParseOpenAIStreamUsage(payload)
		} else {
			detail = helps.ParseOpenAIUsage(payload)
			ok = pluginExecutorUsageDetailPresent(detail)
		}
	}
	buffer.Observe(detail, ok)
}

func pluginExecutorJSONPayload(payload []byte) []byte {
	if parsed := helps.JSONPayload(payload); len(parsed) > 0 {
		return parsed
	}
	for _, line := range bytes.Split(payload, []byte("\n")) {
		if parsed := helps.JSONPayload(line); len(parsed) > 0 {
			return parsed
		}
	}
	return nil
}

func pluginExecutorUsageDetailPresent(detail coreusage.Detail) bool {
	return detail.InputTokens != 0 || detail.OutputTokens != 0 || detail.ReasoningTokens != 0 ||
		detail.CachedTokens != 0 || detail.CacheReadTokens != 0 || detail.CacheCreationTokens != 0 ||
		detail.TotalTokens != 0 || detail.TokenBreakdown.TotalTokens != 0 ||
		strings.TrimSpace(detail.ResponseServiceTier) != "" || strings.TrimSpace(detail.ResponseSpeed) != ""
}

type pluginExecutorEmptyStreamError struct{}

func (pluginExecutorEmptyStreamError) Error() string {
	return "plugin executor stream closed before first payload"
}
func (pluginExecutorEmptyStreamError) StatusCode() int { return http.StatusBadGateway }

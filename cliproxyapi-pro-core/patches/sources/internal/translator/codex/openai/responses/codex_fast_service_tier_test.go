package responses

import (
	"testing"

	"github.com/tidwall/gjson"
)

func TestConvertOpenAIResponsesRequestToCodexNormalizesFastServiceTier(t *testing.T) {
	input := []byte(`{"model":"gpt-5.6","service_tier":"fast","input":"hello"}`)
	output := ConvertOpenAIResponsesRequestToCodex("gpt-5.6", input, true)

	if got := gjson.GetBytes(output, "service_tier").String(); got != "priority" {
		t.Fatalf("service_tier = %q, want priority: %s", got, output)
	}
}

package requestmeta

import (
	"context"
	"testing"
)

func TestClientRequestMetadataRoundTrip(t *testing.T) {
	want := ClientRequestMetadata{
		ClientIP:      "192.0.2.10",
		XForwardedFor: "203.0.113.5, 198.51.100.8",
		UserAgent:     "test-client/1.0",
	}
	ctx := WithClientRequestMetadata(context.Background(), want)
	if got := GetClientRequestMetadata(ctx); got != want {
		t.Fatalf("GetClientRequestMetadata() = %#v, want %#v", got, want)
	}
}

func TestClientRequestMetadataNilContext(t *testing.T) {
	ctx := WithClientRequestMetadata(nil, ClientRequestMetadata{ClientIP: "192.0.2.10"})
	if got := GetClientRequestMetadata(ctx).ClientIP; got != "192.0.2.10" {
		t.Fatalf("ClientIP = %q", got)
	}
	if got := GetClientRequestMetadata(nil); got != (ClientRequestMetadata{}) {
		t.Fatalf("nil context metadata = %#v", got)
	}
}

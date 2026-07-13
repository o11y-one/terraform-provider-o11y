package client

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/metadata"
)

func TestContextUsesPlatformAPIKeyMetadata(t *testing.T) {
	client := &Client{token: "o11y_plat.selector.secret", tenantID: "tenant-1", orgID: "org-1", requestTimeout: time.Second}
	ctx, cancel := client.Context(context.Background())
	defer cancel()

	values, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		t.Fatal("outgoing gRPC metadata is missing")
	}
	if got := values.Get("x-o11y-key"); len(got) != 1 || got[0] != client.token {
		t.Fatalf("x-o11y-key = %v, want the configured platform token", got)
	}
	if got := values.Get("authorization"); len(got) != 0 {
		t.Fatalf("platform API token must not be sent as Authorization metadata: %v", got)
	}
	if got := values.Get("x-o11y-tenant-id"); len(got) != 1 || got[0] != client.tenantID {
		t.Fatalf("tenant metadata = %v", got)
	}
	if got := values.Get("x-o11y-org-id"); len(got) != 1 || got[0] != client.orgID {
		t.Fatalf("organization metadata = %v", got)
	}
}

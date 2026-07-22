package validation

import "testing"

func TestProviderAndResourceValidation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"https endpoint", Endpoint("https://api.example.test:4317", false), false},
		{"http rejected", Endpoint("http://api.example.test:4317", false), true},
		{"http explicit", Endpoint("http://127.0.0.1:4317", true), false},
		{"endpoint path", Endpoint("https://api.example.test/grpc", false), true},
		{"opaque refs", OpaqueSecretRefsJSON(`{"authorization":"secret:webhook-auth"}`), false},
		{"raw ref", OpaqueSecretRefsJSON(`{"authorization":"Bearer raw"}`), true},
		{"inline token key", RejectSecretLikeConfig(`{"token":"raw"}`), true},
		{"ordinary config", RejectSecretLikeConfig(`{"url":"https://hooks.example.test"}`), false},
		{"Observe allowed", NotifyActivation(false), false},
		{"notify rejected", NotifyActivation(true), true},
		{"uuid", UUID("019f430f-90d4-74c3-95b7-9120db366252"), false},
		{"bad uuid", UUID("tenant-a"), true},
		{"empty", NonEmpty("  "), true},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err != nil; got != tt.want {
				t.Fatalf("error=%v, want error=%v", tt.err, tt.want)
			}
		})
	}
}

package idempotency

import "testing"

func TestKeyIsDeterministicAndScopeSensitive(t *testing.T) {
	t.Parallel()
	a := Key("tenant", "org", "destination", "create", "primary")
	if a != Key("tenant", "org", "destination", "create", "primary") {
		t.Fatal("same mutation did not produce the same key")
	}
	if a == Key("other-tenant", "org", "destination", "create", "primary") {
		t.Fatal("tenant scope did not affect key")
	}
	if len(a) != 67 || a[:3] != "tf-" {
		t.Fatalf("unexpected key format: %q", a)
	}
}

func TestKeyCanonicalizesPayload(t *testing.T) {
	a := Key("tenant", "org", "policy", "update", "id", `{"routes":[{"b":2,"a":1}]}`)
	b := Key("tenant", "org", "policy", "update", "id", " { \"routes\" : [ { \"a\" : 1, \"b\" : 2 } ] } ")
	if a != b {
		t.Fatal("JSON whitespace or map order changed key")
	}
	if a == Key("tenant", "org", "policy", "update", "id", `{"routes":[{"a":1,"b":3}]}`) {
		t.Fatal("material payload change did not change key")
	}
}

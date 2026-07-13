package idempotency

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

// Key returns a stable mutation key for the same provider scope, operation, and identity.
func Key(tenantID, orgID, resourceType, operation, identity string, payload ...any) string {
	parts := []string{"terraform", tenantID, orgID, resourceType, operation, identity}
	if len(payload) > 0 {
		canonical, err := json.Marshal(normalize(payload[0]))
		if err != nil {
			panic("idempotency payload is not JSON encodable: " + err.Error())
		}
		parts = append(parts, string(canonical))
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "tf-" + hex.EncodeToString(sum[:])
}

func normalize(value any) any {
	switch typed := value.(type) {
	case string:
		var decoded any
		if json.Unmarshal([]byte(typed), &decoded) == nil {
			return normalize(decoded)
		}
		return typed
	case map[string]any:
		for key, nested := range typed {
			typed[key] = normalize(nested)
		}
		return typed
	case []any:
		for index, nested := range typed {
			typed[index] = normalize(nested)
		}
		return typed
	default:
		return typed
	}
}

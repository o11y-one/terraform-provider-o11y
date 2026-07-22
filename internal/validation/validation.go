package validation

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-8][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)

func NonEmpty(value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("must not be empty")
	}
	return nil
}
func UUID(value string) error {
	if !uuidPattern.MatchString(value) {
		return fmt.Errorf("must be a UUID")
	}
	return nil
}

func JSONDocument(value string) error {
	var document any
	if err := json.Unmarshal([]byte(value), &document); err != nil {
		return fmt.Errorf("must be valid JSON: %w", err)
	}
	if _, ok := document.(map[string]any); !ok {
		return fmt.Errorf("must be a JSON object")
	}
	return nil
}

func Endpoint(value string, allowInsecure bool) error {
	u, err := url.Parse(value)
	if err != nil || u.Host == "" || u.Path != "" && u.Path != "/" {
		return fmt.Errorf("endpoint must be an absolute origin without a path")
	}
	if u.Scheme != "https" && !(allowInsecure && u.Scheme == "http") {
		return fmt.Errorf("endpoint must use https unless insecure TLS is explicitly enabled")
	}
	return nil
}

func OpaqueSecretRefsJSON(value string) error {
	if err := JSONDocument(value); err != nil {
		return err
	}
	var refs map[string]any
	_ = json.Unmarshal([]byte(value), &refs)
	for name, raw := range refs {
		ref, ok := raw.(string)
		if !ok || !strings.HasPrefix(ref, "secret:") || strings.TrimPrefix(ref, "secret:") == "" {
			return fmt.Errorf("secret_refs.%s must be an opaque secret: reference", name)
		}
	}
	return nil
}

func RejectSecretLikeConfig(value string) error {
	var config map[string]any
	if err := json.Unmarshal([]byte(value), &config); err != nil {
		return fmt.Errorf("must be valid JSON: %w", err)
	}
	return rejectSecretLikeValues(config, "config")
}

func rejectSecretLikeValues(value any, location string) error {
	object, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	for key, nested := range object {
		lower := strings.ToLower(key)
		for _, marker := range []string{"authorization", "password", "secret", "token", "api_key", "apikey", "private_key"} {
			if strings.Contains(lower, marker) {
				return fmt.Errorf("%s key %q may contain secret material; use secret_refs", location, key)
			}
		}
		if err := rejectSecretLikeValues(nested, location+"."+key); err != nil {
			return err
		}
	}
	return nil
}

func NotifyActivation(notify bool) error {
	if notify {
		return fmt.Errorf("Terraform manages Observe alerts only; notify activation requires explicit confirmation and an audit reason")
	}
	return nil
}

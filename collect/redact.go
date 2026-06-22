package collect

import "regexp"

// secretPatterns matches common secret values in log lines.
// Each entry is (compiled regex, replacement string using $1 capture).
var secretPatterns = []struct {
	re  *regexp.Regexp
	sub string
}{
	// HTTP auth headers
	{regexp.MustCompile(`(?i)(Authorization:\s*(?:Bearer|Token|Basic)\s+)\S+`), `${1}***`},
	// Stripe / payment keys
	{regexp.MustCompile(`(?i)((?:sk|pk|rk)_(?:live|test)_)\w+`), `${1}***`},
	{regexp.MustCompile(`(?i)((?:sk-proj-|whsec_))[A-Za-z0-9_-]+`), `${1}***`},
	// OpenAI / generic API keys (common env-var assignment forms)
	{regexp.MustCompile(`(?i)(\b(?:OPENAI_API_KEY|ANTHROPIC_API_KEY|STRIPE_SECRET|DATABASE_URL|DB_PASSWORD|REDIS_URL)\s*[=:])\s*\S+`), `${1} ***`},
	// Prefixed environment secrets such as SDK_API_KEY or QUARK_INTERNAL_KEY.
	{regexp.MustCompile(`(?i)(\b[A-Z0-9_]*(?:API_KEY|SECRET_KEY|WEBHOOK_KEY|INTERNAL_KEY|ACCESS_TOKEN|REFRESH_TOKEN|PASSWORD|PRIVATE_KEY)\s*[=:])\s*\S+`), `${1} ***`},
	// Generic low-specificity secrets (env-var form: KEY=value or KEY: value)
	{regexp.MustCompile(`(?i)(\b(?:password|passwd|secret|api_key|jwt_secret|access_token|refresh_token|private_key)\s*[=:])\s*\S+`), `${1} ***`},
	// JSON field form: "password":"value" or "token": "value"
	{regexp.MustCompile(`(?i)("(?:password|passwd|secret|token|api_key)"\s*:\s*")[^"]+"`), `${1}***"`},
}

// Redact replaces recognisable secret values in s with ***.
// It is intentionally conservative: it only matches patterns that are
// very unlikely to produce false positives in normal log output.
func Redact(s string) string {
	for _, p := range secretPatterns {
		s = p.re.ReplaceAllString(s, p.sub)
	}
	return s
}

package collect

import (
	"strings"
	"testing"
)

func TestRedactRemovesCommonProductionSecretShapes(t *testing.T) {
	input := strings.Join([]string{
		"OPENAI_API_KEY=sk-proj-real_secret",
		"STRIPE_WEBHOOK_KEY=whsec_real_secret",
		"SDK_API_KEY=abcdef123456",
		`{"token":"server-token-value"}`,
	}, "\n")

	output := Redact(input)
	for _, leaked := range []string{"real_secret", "abcdef123456", "server-token-value"} {
		if strings.Contains(output, leaked) {
			t.Fatalf("redaction leaked %q in %q", leaked, output)
		}
	}
}

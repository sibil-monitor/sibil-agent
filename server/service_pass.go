package server

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const servicePassPubKeyHex = "d23c7e142453af33f2552485a37c581f14ce2fb791058c3cbaa92df2d021363b"

// ServicePassClaims holds the verified fields from a backend-signed service pass.
type ServicePassClaims struct {
	Ver     int      `json:"ver"`
	Typ     string   `json:"typ"`
	Kid     string   `json:"kid"`
	Sub     string   `json:"sub"`
	OrderID string   `json:"order_id"`
	Offer   string   `json:"offer"`
	AgentID string   `json:"agent_id"`
	Scopes  []string `json:"scopes"`
	MaxRuns int      `json:"max_runs"`
	JTI     string   `json:"jti"`
	Iat     int64    `json:"iat"`
	Nbf     int64    `json:"nbf"`
	Exp     int64    `json:"exp"`
}

// VerifyServicePass parses and verifies a service pass JWT.
// Returns an error if the token is invalid, expired, or bound to a different agent.
func VerifyServicePass(token, agentID string) (*ServicePassClaims, error) {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("malformed service pass")
	}
	payloadB64, sigB64 := parts[0], parts[1]

	if len(payloadB64) > 4096 {
		return nil, fmt.Errorf("service pass too large")
	}

	pubKeyBytes, err := hexDecode(servicePassPubKeyHex)
	if err != nil || len(pubKeyBytes) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("invalid embedded public key")
	}
	pubKey := ed25519.PublicKey(pubKeyBytes)

	sig, err := base64.RawURLEncoding.DecodeString(sigB64)
	if err != nil {
		return nil, fmt.Errorf("invalid signature encoding")
	}
	if !ed25519.Verify(pubKey, []byte(payloadB64), sig) {
		return nil, fmt.Errorf("invalid signature")
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(payloadB64)
	if err != nil {
		return nil, fmt.Errorf("invalid payload encoding")
	}

	var claims ServicePassClaims
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, fmt.Errorf("malformed payload")
	}

	if claims.Typ != "sibil_service_pass" {
		return nil, fmt.Errorf("wrong token type: %s", claims.Typ)
	}

	now := time.Now().Unix()
	if claims.Nbf > 0 && now < claims.Nbf {
		return nil, fmt.Errorf("service pass not yet active")
	}
	if now > claims.Exp {
		return nil, fmt.Errorf("service pass expired")
	}

	if agentID != "" && claims.AgentID != agentID {
		return nil, fmt.Errorf("service pass bound to a different server")
	}

	return &claims, nil
}

// HasScope returns true if the claims include the given scope.
func (c *ServicePassClaims) HasScope(scope string) bool {
	for _, s := range c.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}

package server

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// entitlementPublicKeys maps kid → raw 32-byte Ed25519 public key (hex).
// Add new entries here when rotating keys; keep old ones during the transition window.
// Remove an old entry only once all tokens signed with it have expired (TTL: 24h + 72h grace = 96h).
var entitlementPublicKeys = map[string]string{
	"sibil-entitlement-v1": "791049eedcc6ae3e960468982f101c1a3edd328251d94541d656981e02337168",
	// "sibil-entitlement-v2": "newkeyheX...",
}

const (
	gracePeriod = 72 * time.Hour
	expectedIss = "monitor.cordee.ovh"
	expectedAud = "sibil-cli"
)

// EntitlementClaims holds the verified plan limits from a backend-signed entitlement token.
type EntitlementClaims struct {
	Ver                int     `json:"ver"`
	Kid                string  `json:"kid"`
	Iss                string  `json:"iss"`
	Aud                string  `json:"aud"`
	Sub                string  `json:"sub"`
	Plan               string  `json:"plan"`
	MaxModules         *int    `json:"max_modules"` // nil = unlimited
	RefreshIntervalS   float64 `json:"refresh_interval_s"`
	CanViewSystem      bool    `json:"can_view_system"`
	CanViewLogs        bool    `json:"can_view_logs"`
	CanControlServices bool    `json:"can_control_services"`
	Iat                int64   `json:"iat"`
	Nbf                int64   `json:"nbf"`
	Exp                int64   `json:"exp"`

	IsGrace bool `json:"-"` // token expired but within 72h grace window
}

// degradedClaims is returned when the key map is populated but no valid token is present.
// One module, 30s refresh, no actions — restrictive but not a crash.
func degradedClaims() *EntitlementClaims {
	n := 1
	return &EntitlementClaims{
		Plan:               "free",
		MaxModules:         &n,
		RefreshIntervalS:   30,
		CanViewSystem:      false,
		CanViewLogs:        false,
		CanControlServices: false,
	}
}

// parseEntitlement returns:
//   - nil            → no keys embedded (legacy build), no limits applied
//   - degradedClaims → keys present but token absent, malformed, unknown kid,
//     invalid signature, failed field validation, or expired beyond grace
//   - claims         → fully valid (IsGrace=false) or within 72h grace window (IsGrace=true)
func parseEntitlement(token string) *EntitlementClaims {
	// No keys configured: preserve unlimited legacy behaviour.
	if len(entitlementPublicKeys) == 0 {
		return nil
	}

	if token == "" {
		return degradedClaims()
	}

	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return degradedClaims()
	}
	payloadB64, sigB64 := parts[0], parts[1]

	// Guard against absurdly large tokens before any decoding.
	if len(payloadB64) > 4096 {
		logEntitlementRejection("payload too large")
		return degradedClaims()
	}

	// Decode payload first to read kid — untrusted until the signature check
	// below passes, but kid is only used to select a public key.
	payloadBytes, err := base64.RawURLEncoding.DecodeString(payloadB64)
	if err != nil {
		return degradedClaims()
	}
	var kidPeek struct {
		Kid string `json:"kid"`
	}
	_ = json.Unmarshal(payloadBytes, &kidPeek)

	pubKeyHex, ok := entitlementPublicKeys[kidPeek.Kid]
	if !ok {
		logEntitlementRejection("unknown kid")
		return degradedClaims()
	}

	pubKeyBytes, err := hexDecode(pubKeyHex)
	if err != nil || len(pubKeyBytes) != ed25519.PublicKeySize {
		return degradedClaims()
	}
	pubKey := ed25519.PublicKey(pubKeyBytes)

	sig, err := base64.RawURLEncoding.DecodeString(sigB64)
	if err != nil {
		return degradedClaims()
	}

	// The signature covers the exact payloadB64 bytes — never re-serialise
	// the decoded JSON before verifying, as field order / spacing may differ.
	if !ed25519.Verify(pubKey, []byte(payloadB64), sig) {
		logEntitlementRejection("invalid signature")
		return degradedClaims()
	}

	// Signature valid — now trust the payload.
	var claims EntitlementClaims
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return degradedClaims()
	}

	now := time.Now().Unix()

	// Validate issuer and audience to prevent token reuse across services.
	if claims.Iss != expectedIss || claims.Aud != expectedAud {
		logEntitlementRejection("invalid iss/aud")
		return degradedClaims()
	}

	// not-before: reject tokens used before their activation time.
	if claims.Nbf > 0 && now < claims.Nbf {
		logEntitlementRejection("not yet valid (nbf)")
		return degradedClaims()
	}

	// Version 1 tokens predate explicit read capabilities; honor their signed plan
	// during the TTL/grace transition so an app upgrade does not abruptly hide data.
	if claims.Ver < 2 {
		switch claims.Plan {
		case "trial", "standard", "unlimited":
			claims.CanViewLogs = true
		}
		switch claims.Plan {
		case "trial", "starter", "standard", "unlimited":
			claims.CanViewSystem = true
		}
	}

	// Expiry: accept within grace window, reject beyond it.
	if now > claims.Exp {
		if now > claims.Exp+int64(gracePeriod.Seconds()) {
			logEntitlementRejection("expired beyond grace")
			return degradedClaims()
		}
		fmt.Println("[sibil] entitlement degraded: expired, grace window active")
		claims.IsGrace = true
	}

	return &claims
}

func logEntitlementRejection(reason string) {
	fmt.Printf("[sibil] entitlement rejected: %s\n", reason)
}

// hexDecode converts a hex string to bytes without importing encoding/hex.
func hexDecode(s string) ([]byte, error) {
	if len(s)%2 != 0 {
		return nil, nil
	}
	b := make([]byte, len(s)/2)
	for i := range b {
		hi := hexVal(s[i*2])
		lo := hexVal(s[i*2+1])
		if hi == 255 || lo == 255 {
			return nil, nil
		}
		b[i] = hi<<4 | lo
	}
	return b, nil
}

func hexVal(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	default:
		return 255
	}
}

package server

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"
)

func TestDegradedEntitlementCannotReadPaidSurfacesOrControlServices(t *testing.T) {
	claims := degradedClaims()
	if claims.CanViewSystem || claims.CanViewLogs || claims.CanControlServices {
		t.Fatalf("degraded claims grant paid features: %#v", claims)
	}
}

func TestV1TrialEntitlementKeepsReadCapabilitiesDuringUpgrade(t *testing.T) {
	pub, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	previousKeys := entitlementPublicKeys
	entitlementPublicKeys = map[string]string{"test-v1": hex.EncodeToString(pub)}
	t.Cleanup(func() { entitlementPublicKeys = previousKeys })

	payload, err := json.Marshal(EntitlementClaims{
		Ver:              1,
		Kid:              "test-v1",
		Iss:              expectedIss,
		Aud:              expectedAud,
		Plan:             "trial",
		RefreshIntervalS: 3,
		Nbf:              time.Now().Add(-time.Minute).Unix(),
		Exp:              time.Now().Add(time.Hour).Unix(),
	})
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	payloadB64 := base64.RawURLEncoding.EncodeToString(payload)
	signature := ed25519.Sign(privateKey, []byte(payloadB64))
	token := payloadB64 + "." + base64.RawURLEncoding.EncodeToString(signature)

	claims := parseEntitlement(token)
	if claims == nil || !claims.CanViewSystem || !claims.CanViewLogs {
		t.Fatalf("v1 trial token lost read capabilities: %#v", claims)
	}
	if claims.CanControlServices {
		t.Fatal("v1 trial token unexpectedly grants service control")
	}
}

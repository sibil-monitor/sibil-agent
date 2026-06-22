package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateTokenProducesHex32Bytes(t *testing.T) {
	token, err := GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}
	if len(token) != 64 {
		t.Fatalf("expected 64-char hex token, got %d chars", len(token))
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "sibil.json")

	previous := os.Getenv("SIBIL_CONFIG")
	if err := os.Setenv("SIBIL_CONFIG", configPath); err != nil {
		t.Fatalf("cannot set SIBIL_CONFIG: %v", err)
	}
	defer func() {
		if previous == "" {
			_ = os.Unsetenv("SIBIL_CONFIG")
		} else {
			_ = os.Setenv("SIBIL_CONFIG", previous)
		}
	}()

	cfg := &Config{
		Token:         "demo-token",
		Host:          "127.0.0.1",
		Port:          9876,
		Detector:      "pm2",
		AllowActions:  true,
		TunnelEnabled: true,
		TunnelBackend: DefaultTunnelBackend,
	}

	if err := Save(cfg); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if loaded.Token != cfg.Token || loaded.Detector != cfg.Detector || loaded.Port != cfg.Port {
		t.Fatalf("loaded config does not match saved config: %#v", loaded)
	}
	if !loaded.AllowActions || !loaded.TunnelEnabled {
		t.Fatalf("expected boolean flags to survive round-trip: %#v", loaded)
	}
}

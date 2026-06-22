package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
)

const defaultPort = 9876

const DefaultTunnelBackend = "wss://monitor.cordee.ovh/ws"

// Config is stored in ./sibil.json (or $SIBIL_CONFIG).
type Config struct {
	Token         string `json:"token"`
	Host          string `json:"host"` // bind address, default 127.0.0.1
	Port          int    `json:"port"`
	Detector      string `json:"detector"`       // auto | pm2 | systemd
	AllowActions  bool   `json:"allow_actions"`  // restart/stop/start; disabled by default
	TunnelEnabled bool   `json:"tunnel_enabled"` // relay via monitor.cordee.ovh WebSocket
	TunnelBackend string `json:"tunnel_backend"` // wss:// URL, defaults to DefaultTunnelBackend
}

func Path() string {
	if p := os.Getenv("SIBIL_CONFIG"); p != "" {
		return p
	}
	return "sibil.json"
}

func Load() (*Config, error) {
	data, err := os.ReadFile(Path())
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if cfg.Host == "" {
		cfg.Host = "127.0.0.1"
	}
	if cfg.Port == 0 {
		cfg.Port = defaultPort
	}
	if cfg.Detector == "" {
		cfg.Detector = "auto"
	}
	if cfg.TunnelBackend == "" {
		cfg.TunnelBackend = DefaultTunnelBackend
	}
	return &cfg, nil
}

func Save(cfg *Config) error {
	if err := os.MkdirAll(filepath.Dir(Path()), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(Path(), data, 0o600)
}

func GenerateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const manifestSubdir = ".sibil"
const manifestFilename = "install_manifest.json"

// InstallManifest is written at sibil init so sibil uninstall can locate
// every component without guessing. Stored in ~/.sibil/install_manifest.json.
type InstallManifest struct {
	Binary      string `json:"binary"`
	Config      string `json:"config"`
	PassesDir   string `json:"passes_dir"`
	Runtime     string `json:"runtime"`      // "pm2" | "systemd" | "manual" | "unknown"
	RuntimeName string `json:"runtime_name"` // PM2 process name or systemd unit
	InstalledAt string `json:"installed_at"`
	InstallMode string `json:"install_mode"` // always "persistent_activation"
}

func ManifestPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, manifestSubdir, manifestFilename), nil
}

func ManifestDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, manifestSubdir), nil
}

func WriteManifest(m *InstallManifest) error {
	dir, err := ManifestDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(dir, manifestFilename)
	return os.WriteFile(path, data, 0o600)
}

func ReadManifest() (*InstallManifest, error) {
	path, err := ManifestPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m InstallManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func RemoveManifest() error {
	path, err := ManifestPath()
	if err != nil {
		return err
	}
	os.Remove(path)
	// Remove the ~/.sibil dir only if it is now empty
	if dir, err := ManifestDir(); err == nil {
		entries, readErr := os.ReadDir(dir)
		if readErr == nil && len(entries) == 0 {
			os.Remove(dir)
		}
	}
	return nil
}

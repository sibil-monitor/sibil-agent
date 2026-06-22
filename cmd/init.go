package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sibil-monitor/sibil-agent/config"
	"github.com/spf13/cobra"
)

var (
	initHostFlag         string
	initPortFlag         int
	initDetectorFlag     string
	initTunnelFlag       bool
	initAllowActionsFlag bool
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Generate a sibil.json config file with a fresh token",
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := os.Stat(config.Path()); err == nil {
			fmt.Printf("Config already exists at %s\nDelete it first to reinitialise.\n", config.Path())
			return nil
		}

		token, err := config.GenerateToken()
		if err != nil {
			return fmt.Errorf("cannot generate token: %w", err)
		}

		cfg := &config.Config{
			Token:         token,
			Host:          normalizeHost(initHostFlag),
			Port:          normalizePort(initPortFlag),
			Detector:      normalizeDetector(initDetectorFlag),
			AllowActions:  initAllowActionsFlag,
			TunnelEnabled: initTunnelFlag,
			TunnelBackend: config.DefaultTunnelBackend,
		}

		if err := config.Save(cfg); err != nil {
			return fmt.Errorf("cannot write config: %w", err)
		}

		// Write install manifest so sibil uninstall can find all components
		// without guessing. Non-fatal: uninstall falls back to path scanning.
		writeInstallManifest(cfg)

		fmt.Printf("✓ Config created: %s\n\n", config.Path())
		fmt.Printf("  Token         : %s\n", token)
		fmt.Printf("  Bind          : %s:%d\n", cfg.Host, cfg.Port)
		fmt.Printf("  Detector      : %s\n", cfg.Detector)
		fmt.Printf("  Tunnel        : %t\n", cfg.TunnelEnabled)
		fmt.Printf("  Allow actions : %t", cfg.AllowActions)
		if !cfg.AllowActions {
			fmt.Printf("  (run: sibil actions enable to activate)")
		}
		fmt.Println()
		fmt.Println("Security notice:")
		fmt.Println("  sibil binds to 127.0.0.1 by default.")
		fmt.Println("  To expose it remotely, use a HTTPS reverse proxy or Tailscale.")
		fmt.Println("  Never bind 0.0.0.0 directly on a public interface.")
		fmt.Println()
		if cfg.TunnelEnabled {
			fmt.Println("Tunnel mode has been pre-enabled.")
		}
		fmt.Println("Copy this token into the Sibil Monitor app, then run: sibil doctor && sibil start")
		return nil
	},
}

func init() {
	initCmd.Flags().StringVar(&initHostFlag, "host", "127.0.0.1", "bind address to store in sibil.json")
	initCmd.Flags().IntVar(&initPortFlag, "port", 9876, "port to store in sibil.json")
	initCmd.Flags().StringVar(&initDetectorFlag, "detector", "auto", "service detector to use: auto | pm2 | systemd")
	initCmd.Flags().BoolVar(&initTunnelFlag, "tunnel", false, "pre-enable WebSocket tunnel mode")
	initCmd.Flags().BoolVar(&initAllowActionsFlag, "allow-actions", false, "pre-enable destructive service actions")
}

func writeInstallManifest(cfg *config.Config) {
	binaryPath, _ := os.Executable()
	if resolved, err := filepath.EvalSymlinks(binaryPath); err == nil {
		binaryPath = resolved
	}

	absConfig, _ := filepath.Abs(config.Path())
	wd, _ := os.Getwd()
	passesDir := filepath.Join(wd, ".sibil-passes")
	if absConfig != "" {
		passesDir = filepath.Join(filepath.Dir(absConfig), ".sibil-passes")
	}

	runtime := "unknown"
	if cfg.Detector == "pm2" {
		runtime = "pm2"
	} else if cfg.Detector == "systemd" {
		runtime = "systemd"
	}

	m := &config.InstallManifest{
		Binary:      binaryPath,
		Config:      absConfig,
		PassesDir:   passesDir,
		Runtime:     runtime,
		RuntimeName: "sibil",
		InstalledAt: time.Now().UTC().Format(time.RFC3339),
		InstallMode: "persistent_activation",
	}
	config.WriteManifest(m) //nolint:errcheck
}

func normalizeHost(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return "127.0.0.1"
	}
	return host
}

func normalizePort(port int) int {
	if port <= 0 {
		return 9876
	}
	return port
}

func normalizeDetector(detector string) string {
	switch strings.ToLower(strings.TrimSpace(detector)) {
	case "pm2", "systemd":
		return strings.ToLower(strings.TrimSpace(detector))
	default:
		return "auto"
	}
}

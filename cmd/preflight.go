package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const preflightProbeEndpoint = "https://monitor.cordee.ovh/api/v1/preflight/probe"
const preflightPingEndpoint = "https://monitor.cordee.ovh/health"

var preflightCmd = &cobra.Command{
	Use:   "preflight",
	Short: "Run an ephemeral compatibility check — nothing is installed",
	Long: `sibil preflight checks whether Sibil can run on this VPS.

No config file is created. No service passes are stored. No daemon is started.
Temporary files (if any) are removed on exit.

If a --connect code is provided, a minimal probe is sent to confirm tunnel
reachability. No infrastructure inventory is uploaded.`,
	RunE: runPreflight,
}

func init() {
	preflightCmd.Flags().String("connect", "", "connect code (SIB-CONNECT-XXXX-XXXX-XXXX) — sends a minimal probe to the backend")
}

func runPreflight(cmd *cobra.Command, args []string) error {
	connectCode, _ := cmd.Flags().GetString("connect")
	if connectCode != "" {
		connectCode = strings.ToUpper(strings.TrimSpace(connectCode))
		if !strings.HasPrefix(connectCode, "SIB-CONNECT-") {
			return fmt.Errorf("invalid connect code format (expected SIB-CONNECT-XXXX-XXXX-XXXX)")
		}
	}

	fmt.Println("sibil preflight — temporary compatibility check")
	fmt.Println("  Nothing will be installed. No config or passes are created.")
	fmt.Println()

	os_ := runtime.GOOS
	arch := runtime.GOARCH

	// ── OS / arch ────────────────────────────────────────────────────────────
	supported := (os_ == "linux") && (arch == "amd64" || arch == "arm64")
	if supported {
		fmt.Printf("  ✓ OS: %s/%s\n", os_, arch)
	} else {
		fmt.Printf("  ✗ OS: %s/%s — not supported\n", os_, arch)
	}

	// ── PM2 ──────────────────────────────────────────────────────────────────
	pm2Found := false
	if _, err := exec.LookPath("pm2"); err == nil {
		pm2Found = true
		fmt.Println("  ✓ PM2: detected")
	} else {
		fmt.Println("  · PM2: not detected")
	}

	// ── systemd ───────────────────────────────────────────────────────────────
	systemdFound := false
	if _, err := exec.LookPath("systemctl"); err == nil {
		systemdFound = true
		fmt.Println("  ✓ systemd: detected")
	} else {
		fmt.Println("  · systemd: not detected")
	}

	// ── Docker (non-blocking) ─────────────────────────────────────────────────
	dockerStatus := "not_detected"
	if out, err := exec.Command("docker", "info").CombinedOutput(); err == nil {
		dockerStatus = "detected"
		fmt.Println("  ✓ Docker: detected")
	} else if strings.Contains(string(out), "permission denied") {
		dockerStatus = "permission_denied"
		fmt.Println("  · Docker: permission denied (non-blocking)")
	} else {
		fmt.Println("  · Docker: not detected (non-blocking)")
	}

	// ── Outbound tunnel ───────────────────────────────────────────────────────
	tunnelOK := false
	httpClient := &http.Client{Timeout: 8 * time.Second}
	if resp, err := httpClient.Get(preflightPingEndpoint); err == nil {
		resp.Body.Close()
		tunnelOK = true
		fmt.Println("  ✓ Tunnel: outbound connection to monitor.cordee.ovh reachable")
	} else {
		fmt.Println("  ✗ Tunnel: cannot reach monitor.cordee.ovh")
	}

	// ── Backend probe (only if connect code provided + tunnel works) ──────────
	if connectCode != "" && tunnelOK {
		fmt.Println()
		fmt.Printf("  Sending compatibility probe (%s...)...\n", connectCode[:min(16, len(connectCode))])
		if err := sendPreflightProbe(connectCode, os_, arch, pm2Found, systemdFound, dockerStatus, tunnelOK); err != nil {
			fmt.Printf("  · Probe not recorded by backend: %v\n", err)
		} else {
			fmt.Println("  ✓ Backend handshake confirmed")
		}
	}

	// ── Summary ───────────────────────────────────────────────────────────────
	fmt.Println()
	fmt.Println("──────────────────────────────────────────────────────")

	blockers := []string{}
	warnings := []string{}

	if !supported {
		blockers = append(blockers, fmt.Sprintf("Architecture %s/%s is not supported", os_, arch))
	}
	if !tunnelOK {
		blockers = append(blockers, "Outbound connection to monitor.cordee.ovh is blocked")
	}
	if !pm2Found && !systemdFound {
		warnings = append(warnings, "Neither PM2 nor systemd found — Sibil will need to be started manually")
	} else if !pm2Found {
		warnings = append(warnings, "PM2 not found — systemd will be used as process manager")
	}

	switch {
	case len(blockers) > 0:
		fmt.Println("✗ Not compatible")
		fmt.Println()
		fmt.Println("  Sibil cannot run safely on this VPS:")
		for _, b := range blockers {
			fmt.Printf("    · %s\n", b)
		}
	case len(warnings) > 0:
		fmt.Println("⚠  Partially compatible")
		fmt.Println()
		fmt.Printf("  Sibil can run on this VPS (%s/%s), with limitations:\n", os_, arch)
		for _, w := range warnings {
			fmt.Printf("    · %s\n", w)
		}
	default:
		fmt.Println("✓ Compatible")
		fmt.Println()
		fmt.Printf("  Sibil can run on this VPS (%s/%s).\n", os_, arch)
		if pm2Found {
			fmt.Println("  PM2 is available — recommended process manager.")
		}
	}

	fmt.Println()
	fmt.Println("  No persistent install was made.")
	fmt.Println("  Temporary probe removed.")

	if len(blockers) == 0 {
		fmt.Println()
		fmt.Println("  Next:")
		fmt.Println("    · Start trial      → https://monitor.cordee.ovh/#trial")
		fmt.Println("    · Book Sibil Setup → https://monitor.cordee.ovh/#setup")
		fmt.Println("    · Request Audit    → https://monitor.cordee.ovh/#audit")
	}

	return nil
}

func sendPreflightProbe(connectCode, os_, arch string, pm2, systemd bool, docker string, tunnel bool) error {
	payload := map[string]any{
		"connect_code":       connectCode,
		"os":                 os_,
		"arch":               arch,
		"can_run":            tunnel,
		"tunnel_reachable":   tunnel,
		"pm2_detected":       pm2,
		"systemd_detected":   systemd,
		"docker_detected":    docker,
		"persistent_install": false,
	}
	body, _ := json.Marshal(payload)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(preflightProbeEndpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		var result map[string]any
		json.NewDecoder(resp.Body).Decode(&result) //nolint:errcheck
		errMsg, _ := result["error"].(string)
		return fmt.Errorf("backend returned %d: %s", resp.StatusCode, errMsg)
	}
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

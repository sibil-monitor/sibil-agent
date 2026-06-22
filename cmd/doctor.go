package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/sibil-monitor/sibil-agent/config"
	"github.com/spf13/cobra"
)

// doctor is intentionally read-only. It observes and reports; it never repairs.
// This mirrors Sibil's core doctrine: observe first, act deliberately.
// Do not add --fix. If needed in the future, add --explain instead.
var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check your Sibil setup and report any issues",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("sibil doctor — checking your setup...")
		fmt.Println()

		ok := true

		// ── 1. Config ──────────────────────────────────────────────────────────
		cfg, err := config.Load()
		if err != nil {
			fail("Config: not found — run: sibil init")
			return nil
		}
		pass("Config: %s", config.Path())

		// ── 2. Token ───────────────────────────────────────────────────────────
		if cfg.Token == "" {
			fail("Token: empty — run: sibil init")
			ok = false
		} else {
			pass("Token: set (%d chars)", len(cfg.Token))
		}

		// ── 3. Bind address ────────────────────────────────────────────────────
		if cfg.Host == "0.0.0.0" {
			warn("Host: 0.0.0.0 (public exposure risk)")
			hint("Recommended: use 127.0.0.1 + Tailscale/VPN or HTTPS reverse proxy.")
		} else {
			pass("Host: %s (safe)", cfg.Host)
		}
		pass("Port: %d", cfg.Port)

		// ── 4. Actions ─────────────────────────────────────────────────────────
		if cfg.AllowActions {
			warn("Actions: enabled")
			hint("Restart/stop/start endpoints are active. Use only on trusted networks.")
		} else {
			pass("Actions: disabled (read-only mode)")
		}

		// ── 5. Service detector ────────────────────────────────────────────────
		pm2Count, pm2Err := detectPM2()
		sdCount, sdErr := detectSystemd()

		switch cfg.Detector {
		case "pm2":
			if pm2Err != nil {
				fail("Detector: pm2 — pm2 not found (%v)", pm2Err)
				ok = false
			} else {
				pass("Detector: pm2 — %d process(es) detected", pm2Count)
			}
		case "systemd":
			if sdErr != nil {
				fail("Detector: systemd — systemctl not found (%v)", sdErr)
				ok = false
			} else {
				pass("Detector: systemd — %d service(s) detected", sdCount)
			}
		default: // auto
			if pm2Err == nil {
				pass("Detector: auto → pm2 — %d process(es) detected", pm2Count)
			} else if sdErr == nil {
				pass("Detector: auto → systemd — %d service(s) detected", sdCount)
			} else {
				fail("Detector: auto — neither pm2 nor systemctl found")
				ok = false
			}
		}

		// ── 6. Running server ──────────────────────────────────────────────────
		addr := fmt.Sprintf("http://%s:%d", cfg.Host, cfg.Port)
		health, runErr := checkHealth(addr)
		if runErr != nil {
			note("Server: not running — start with: sibil start")
		} else {
			pass("Server: running at %s (version: %s)", addr, health["version"])
			if kids, ok2 := health["supported_entitlement_kids"]; ok2 {
				pass("Entitlement kids: %v", kids)
			}
		}

		// ── Summary ────────────────────────────────────────────────────────────
		fmt.Println()
		if ok {
			fmt.Println("✓ Setup looks good.")
			if cfg.TunnelEnabled {
				if runErr != nil {
					fmt.Println("\nNext steps:")
					fmt.Println("  sibil start")
				}
				fmt.Println("\n→ In the Sibil Monitor app, choose tunnel mode and enter your token.")
			} else if runErr != nil {
				fmt.Println("\nNext steps:")
				fmt.Println("  sibil start")
				fmt.Printf("  → Then add %s to the Sibil Monitor app.\n", addr)
			} else {
				fmt.Printf("\n→ Add %s and your token to the Sibil Monitor app.\n", addr)
			}
		} else {
			fmt.Println("✗ Some checks failed — see above.")
		}
		return nil
	},
}

// ── helpers ───────────────────────────────────────────────────────────────────

func pass(format string, a ...any) {
	fmt.Printf("  ✓ "+format+"\n", a...)
}

func fail(format string, a ...any) {
	fmt.Printf("  ✗ "+format+"\n", a...)
}

func warn(format string, a ...any) {
	fmt.Printf("  ⚠ "+format+"\n", a...)
}

func note(format string, a ...any) {
	fmt.Printf("  · "+format+"\n", a...)
}

func hint(text string) {
	fmt.Printf("    → %s\n", text)
}

func detectPM2() (int, error) {
	out, err := exec.Command("pm2", "jlist").Output()
	if err != nil {
		return 0, err
	}
	var procs []json.RawMessage
	if err := json.Unmarshal(out, &procs); err != nil {
		return 0, err
	}
	return len(procs), nil
}

func detectSystemd() (int, error) {
	out, err := exec.Command(
		"systemctl", "list-units", "--type=service",
		"--state=loaded", "--no-pager", "--no-legend", "--plain",
	).Output()
	if err != nil {
		return 0, err
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	count := 0
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			count++
		}
	}
	return count, nil
}

func checkHealth(addr string) (map[string]any, error) {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(addr + "/health")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	return body, nil
}

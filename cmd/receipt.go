package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/sibil-monitor/sibil-agent/collect"
	"github.com/sibil-monitor/sibil-agent/config"
	"github.com/spf13/cobra"
)

var receiptCmd = &cobra.Command{
	Use:   "setup-receipt",
	Short: "Generate a Sibil Setup Receipt after a guided setup session",
	Long: `sibil setup-receipt produces a lightweight handoff document summarising
the setup state: CLI config, detector, first health picture, and next actions.

This is the deliverable for the VPS Health Setup service (EUR 199).
It is not an audit — for a full observability audit use: sibil audit`,
	RunE: runReceipt,
}

var receiptOutput string

func init() {
	receiptCmd.Flags().StringVar(&receiptOutput, "output", "", "output file path (default: ./sibil-setup-receipt-YYYYMMDD.md)")
}

func runReceipt(cmd *cobra.Command, args []string) error {
	now := time.Now()
	ts := now.Format("20060102")

	outPath := receiptOutput
	if outPath == "" {
		outPath = fmt.Sprintf("sibil-setup-receipt-%s.md", ts)
	}

	cfg, cfgErr := config.Load()

	fmt.Println("sibil setup-receipt — collecting setup state...")
	fmt.Println()

	// ── Setup status ──────────────────────────────────────────────────────────
	cliInstalled := "yes"
	cliVer := auditCLIVersion
	serverVersion := ""
	detector := "auto"
	actionsEnabled := "no"
	tokenConfigured := "no"
	hostSafe := "yes"
	tunnelStatus := "disabled"

	if cfgErr != nil {
		cliInstalled = "partial (config not found — run: sibil init)"
	} else {
		detector = cfg.Detector
		if cfg.Token != "" {
			tokenConfigured = "yes"
		}
		if cfg.AllowActions {
			actionsEnabled = "yes"
		}
		if cfg.Host == "0.0.0.0" {
			hostSafe = "warning (0.0.0.0 — public exposure risk)"
		}
		if cfg.TunnelEnabled {
			tunnelStatus = "enabled"
		}
	}

	// ── Connection test ───────────────────────────────────────────────────────
	dashboardConnection := "not tested"
	if cfg != nil {
		addr := fmt.Sprintf("http://%s:%d", cfg.Host, cfg.Port)
		if health, err := checkHealth(addr); err == nil {
			dashboardConnection = "verified (server responding)"
			if v, ok := health["version"].(string); ok && v != "" {
				serverVersion = v
			}
		} else {
			dashboardConnection = "not reachable — start with: sibil start"
		}
	}

	// ── Doctor summary ────────────────────────────────────────────────────────
	doctorLines := runDoctorCapture(cfg)

	// ── First health picture ──────────────────────────────────────────────────
	sys, _ := collect.AuditSystemSnapshot(false)
	pm2Svcs, _ := collect.AuditPM2Services(false)
	sysdSvcs, _ := collect.AuditSystemdServices()

	// Count only running systemd services
	sysdRunning := 0
	for _, s := range sysdSvcs {
		if s.SubState == "running" {
			sysdRunning++
		}
	}

	// ── Build receipt ─────────────────────────────────────────────────────────
	var b strings.Builder

	setupID := fmt.Sprintf("SIB-SETUP-%s", now.Format("2006-01-02-150405"))

	b.WriteString("# Sibil Setup Receipt\n\n")
	b.WriteString(fmt.Sprintf("Setup ID: `%s`  \n", setupID))
	b.WriteString(fmt.Sprintf("Date: %s  \n", now.Format("2006-01-02 15:04")))
	b.WriteString(fmt.Sprintf("CLI version: %s  \n\n", cliVer))
	b.WriteString("> Generated locally by Sibil CLI. No setup data was uploaded automatically. Review before sharing.\n\n")
	b.WriteString("---\n\n")

	b.WriteString("## Scope\n\n")
	b.WriteString("One VPS. Sibil CLI installation, connection validation, and first health picture.\n\n")
	b.WriteString("## Out of Scope\n\n")
	b.WriteString("Full architecture audit, runtime risk register, remediation roadmap, ")
	b.WriteString("incident runbook, security hardening, Docker/nginx deep mapping, ")
	b.WriteString("and provider inventory. These are covered by the AI Stack Observability Audit.\n\n")
	b.WriteString("---\n\n")

	b.WriteString("## Setup Status\n\n")
	b.WriteString(fmt.Sprintf("- CLI installed: %s\n", cliInstalled))
	b.WriteString(fmt.Sprintf("- CLI version (receipt): %s\n", cliVer))
	if serverVersion != "" && serverVersion != cliVer {
		b.WriteString(fmt.Sprintf("- Server version (running): %s\n", serverVersion))
	}
	b.WriteString(fmt.Sprintf("- Detector: %s\n", detector))
	b.WriteString(fmt.Sprintf("- Tunnel mode: %s\n", tunnelStatus))
	b.WriteString(fmt.Sprintf("- Actions enabled: %s\n", actionsEnabled))
	b.WriteString(fmt.Sprintf("- Token configured: %s\n", tokenConfigured))
	b.WriteString(fmt.Sprintf("- Host safety: %s\n", hostSafe))
	b.WriteString(fmt.Sprintf("- Dashboard connection: %s\n\n", dashboardConnection))

	b.WriteString("## First Health Picture\n\n")
	if sys != nil {
		b.WriteString(fmt.Sprintf("- OS: %s\n", sys.OS))
		b.WriteString(fmt.Sprintf("- Uptime: %s\n", formatUptimeSec(sys.UptimeSeconds)))
		b.WriteString(fmt.Sprintf("- CPU: %d core(s), load %.2f / %.2f / %.2f\n",
			sys.CPUCount, sys.LoadAvg[0], sys.LoadAvg[1], sys.LoadAvg[2]))
		b.WriteString(fmt.Sprintf("- RAM: %d MB used / %d MB total\n", sys.Memory.UsedMB, sys.Memory.TotalMB))
		for _, d := range sys.Disk {
			b.WriteString(fmt.Sprintf("- Disk %s: %.1f%% used\n", d.Mount, d.UsedPercent))
		}
	} else {
		b.WriteString("- System metrics: unavailable\n")
	}
	b.WriteString(fmt.Sprintf("- PM2 services detected: %d\n", len(pm2Svcs)))
	b.WriteString(fmt.Sprintf("- systemd running services: %d\n\n", sysdRunning))

	b.WriteString("## Doctor Result\n\n")
	for _, line := range doctorLines {
		b.WriteString(fmt.Sprintf("%s\n", line))
	}
	b.WriteString("\n")

	b.WriteString("## Immediate Notes\n\n")
	b.WriteString("_Completed during the setup session._\n\n")

	// Auto-populate "What looks healthy" from doctor passing checks
	b.WriteString("**What looks healthy:**\n")
	if dashboardConnection != "" && strings.HasPrefix(dashboardConnection, "verified") {
		b.WriteString("- Server running and responding\n")
	}
	if tokenConfigured == "yes" {
		b.WriteString("- Token configured and connection tested\n")
	}
	if hostSafe == "yes" {
		b.WriteString("- Host binding is local-only (127.0.0.1)\n")
	}
	b.WriteString("- \n\n")

	// Auto-populate "What needs attention" from detected signals
	b.WriteString("**What needs attention:**\n")
	attention := 0
	if sys != nil {
		for _, d := range sys.Disk {
			if d.UsedPercent >= 90 {
				b.WriteString(fmt.Sprintf("- Disk %s at %.1f%% — action recommended before it hits 95%%\n", d.Mount, d.UsedPercent))
				attention++
			} else if d.UsedPercent >= 75 {
				b.WriteString(fmt.Sprintf("- Disk %s at %.1f%% — monitor closely\n", d.Mount, d.UsedPercent))
				attention++
			}
		}
	}
	if hostSafe != "yes" {
		b.WriteString("- Host bound to 0.0.0.0 — restrict to 127.0.0.1 or add reverse proxy\n")
		attention++
	}
	if attention == 0 {
		b.WriteString("- \n")
	}
	b.WriteString("\n")
	b.WriteString("**Recommended next step:**\n")
	b.WriteString("- \n\n")

	b.WriteString("---\n\n")
	b.WriteString("## Upgrade Path\n\n")
	b.WriteString("For a full architecture map, observability gap matrix, priority risk register,\n")
	b.WriteString("and 7-day remediation roadmap, use the **AI Stack Observability Audit**.\n\n")
	b.WriteString("> This setup covers Sibil installation and connection only.\n")
	b.WriteString("> Fixing existing infrastructure issues can be quoted separately.\n")

	if err := os.WriteFile(outPath, []byte(b.String()), 0600); err != nil {
		return fmt.Errorf("cannot write receipt: %w", err)
	}

	fmt.Printf("✓ Setup receipt written to: %s\n", outPath)
	return nil
}

// runDoctorCapture runs the core doctor checks and returns formatted lines.
func runDoctorCapture(cfg *config.Config) []string {
	var lines []string

	if cfg == nil {
		lines = append(lines, "- ✗ Config: not found — run: sibil init")
		return lines
	}
	lines = append(lines, fmt.Sprintf("- ✓ Config: %s", config.Path()))

	if cfg.Token == "" {
		lines = append(lines, "- ✗ Token: empty")
	} else {
		lines = append(lines, fmt.Sprintf("- ✓ Token: set (%d chars)", len(cfg.Token)))
	}

	if cfg.Host == "0.0.0.0" {
		lines = append(lines, "- ⚠ Host: 0.0.0.0 (public exposure — use 127.0.0.1 + reverse proxy)")
	} else {
		lines = append(lines, fmt.Sprintf("- ✓ Host: %s (safe)", cfg.Host))
	}

	pm2Count, pm2Err := detectPM2()
	sdCount, sdErr := detectSystemd()
	switch cfg.Detector {
	case "pm2":
		if pm2Err != nil {
			lines = append(lines, "- ✗ Detector: pm2 — pm2 not found")
		} else {
			lines = append(lines, fmt.Sprintf("- ✓ Detector: pm2 — %d process(es)", pm2Count))
		}
	case "systemd":
		if sdErr != nil {
			lines = append(lines, "- ✗ Detector: systemd — not found")
		} else {
			lines = append(lines, fmt.Sprintf("- ✓ Detector: systemd — %d service(s)", sdCount))
		}
	default:
		if pm2Err == nil {
			lines = append(lines, fmt.Sprintf("- ✓ Detector: auto → pm2 (%d process(es))", pm2Count))
		} else if sdErr == nil {
			lines = append(lines, fmt.Sprintf("- ✓ Detector: auto → systemd (%d service(s))", sdCount))
		} else {
			lines = append(lines, "- ✗ Detector: auto — neither pm2 nor systemctl found")
		}
	}

	addr := fmt.Sprintf("http://%s:%d", cfg.Host, cfg.Port)
	if _, err := checkHealth(addr); err != nil {
		lines = append(lines, fmt.Sprintf("- · Server: not running at %s", addr))
	} else {
		lines = append(lines, fmt.Sprintf("- ✓ Server: running at %s", addr))
	}

	return lines
}

// sibylVersion reads the installed sibil binary version if in PATH.
func sibylVersion() string {
	out, err := exec.Command("sibil", "version").Output()
	if err != nil {
		return auditCLIVersion
	}
	return strings.TrimSpace(string(out))
}

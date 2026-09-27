package cmd

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/sibil-monitor/sibil-agent/collect"
	"github.com/sibil-monitor/sibil-agent/config"
	"github.com/spf13/cobra"
)

// ── Types ─────────────────────────────────────────────────────────────────────

type auditReport struct {
	GeneratedAt   string               `json:"generated_at"`
	CLIVersion    string               `json:"cli_version"`
	SafeMode      bool                 `json:"safe_mode"`
	System        *collect.AuditSystem `json:"system"`
	Services      auditServices        `json:"services"`
	Network       auditNetwork         `json:"network"`
	Observability []obsSignal          `json:"observability"`
	RiskHints     []riskHint           `json:"risk_hints"`
}

type auditServices struct {
	PM2     []collect.AuditPM2Service     `json:"pm2"`
	Systemd []collect.AuditSystemdService `json:"systemd"`
	Docker  collect.DockerResult          `json:"docker"`
}

type auditNetwork struct {
	Status    string                    `json:"status"` // ok | error
	Error     string                    `json:"error,omitempty"`
	Listeners []collect.NetworkListener `json:"listeners"`
}

type obsSignal struct {
	Service            string   `json:"service"`
	ManagedBy          string   `json:"managed_by"`
	StackGuess         string   `json:"stack_guess"`
	LogsAccessible     bool     `json:"logs_accessible"`
	RecentRestarts     int      `json:"recent_restarts"`
	PortDetected       *int     `json:"port_detected,omitempty"`
	HasHealthEndpoint  string   `json:"has_health_endpoint"` // unknown | yes | no
	ObservabilityScore float64  `json:"observability_score"`
	Gaps               []string `json:"gaps"`
}

type riskHint struct {
	Level               string  `json:"level"` // high | medium | low
	Title               string  `json:"title"`
	Evidence            string  `json:"evidence"`
	Confidence          float64 `json:"confidence"`
	RequiresHumanReview bool    `json:"requires_human_review"`
}

type redactionEvent struct {
	Field   string `json:"field"`
	Pattern string `json:"pattern"`
	Note    string `json:"note"`
}

type redactionReport struct {
	GeneratedAt string           `json:"generated_at"`
	SafeMode    bool             `json:"safe_mode"`
	Events      []redactionEvent `json:"events"`
}

// ── Command ───────────────────────────────────────────────────────────────────

const auditCLIVersion = "1.4.2"

var auditCmd = &cobra.Command{
	Use:   "audit",
	Short: "Collect a structured observability snapshot for expert audit",
	Long: `sibil audit collects a local runtime snapshot and writes it to a
timestamped directory. No data is uploaded. The output feeds the
expert audit dossier.

Flags:
  --output       output directory (default: ./sibil-audit-YYYYMMDD-HHMM)
  --json         print audit.json to stdout instead of writing files
  --include-docker  include Docker container inventory (best-effort)
  --no-redact    keep full paths and hostname (operator use only)`,
	RunE: runAudit,
}

var (
	auditOutputDir     string
	auditJSONOnly      bool
	auditBundle        bool
	auditIncludeDocker bool
	auditNoRedact      bool
)

func init() {
	auditCmd.Flags().StringVar(&auditOutputDir, "output", "", "output directory path")
	auditCmd.Flags().BoolVar(&auditJSONOnly, "json", false, "print audit.json to stdout")
	auditCmd.Flags().BoolVar(&auditBundle, "bundle", false, "produce a .tar.gz archive of the audit dossier")
	auditCmd.Flags().BoolVar(&auditIncludeDocker, "include-docker", false, "include Docker inventory")
	auditCmd.Flags().BoolVar(&auditNoRedact, "no-redact", false, "disable redaction (operator use only)")
}

func runAudit(cmd *cobra.Command, args []string) error {
	safe := !auditNoRedact
	now := time.Now()
	ts := now.Format("20060102-1504")

	outDir := auditOutputDir
	if outDir == "" {
		outDir = fmt.Sprintf("sibil-audit-%s", ts)
	}

	cfg, _ := config.Load()
	detector := "auto"
	if cfg != nil {
		detector = cfg.Detector
	}

	fmt.Println("sibil audit — collecting runtime snapshot...")
	fmt.Printf("  safe mode: %v\n", safe)
	fmt.Println()

	var redactions []redactionEvent

	// ── Layer 1: System ──────────────────────────────────────────────────────
	fmt.Print("  [1/4] system snapshot... ")
	sys, err := collect.AuditSystemSnapshot(safe)
	if err != nil {
		fmt.Printf("error: %v\n", err)
	} else {
		fmt.Println("ok")
	}
	if safe && sys != nil {
		redactions = append(redactions, redactionEvent{
			Field:   "system.hostname",
			Pattern: "hostname",
			Note:    "replaced with redacted-host",
		})
	}

	// ── Layer 2: Runtime inventory ───────────────────────────────────────────
	fmt.Print("  [2/4] runtime inventory... ")
	var pm2Svcs []collect.AuditPM2Service
	var sysdSvcs []collect.AuditSystemdService
	docker := collect.DockerResult{Status: "skipped"}

	pm2Svcs, pm2Err := collect.AuditPM2Services(safe)
	if pm2Err != nil {
		pm2Svcs = nil
	}

	if detector == "systemd" || detector == "auto" {
		sysdSvcs, _ = collect.AuditSystemdServices()
	}

	if auditIncludeDocker {
		docker = collect.DockerInventory()
	}

	if safe && len(pm2Svcs) > 0 {
		redactions = append(redactions, redactionEvent{
			Field:   "services.pm2[*].script",
			Pattern: "path",
			Note:    "basename only (--no-redact for full paths)",
		})
	}
	fmt.Println("ok")

	// ── Layer 3: Network exposure ────────────────────────────────────────────
	fmt.Print("  [3/4] network listeners... ")
	listeners, netErr := collect.NetworkListeners()
	netStatus := auditNetwork{Status: "ok", Listeners: listeners}
	if netErr != nil {
		netStatus = auditNetwork{Status: "error", Error: netErr.Error(), Listeners: nil}
		fmt.Printf("error: %v\n", netErr)
	} else {
		fmt.Printf("%d listener(s)\n", len(listeners))
	}

	// ── Layer 4: Observability signals ───────────────────────────────────────
	fmt.Print("  [4/4] observability signals... ")
	obs := buildObservabilitySignals(pm2Svcs, sysdSvcs, listeners, detector)
	fmt.Printf("%d service(s) analyzed\n", len(obs))

	// Risk hints
	hints := buildRiskHints(listeners, obs, sys)

	// ── Assemble report ──────────────────────────────────────────────────────
	report := auditReport{
		GeneratedAt: now.UTC().Format(time.RFC3339),
		CLIVersion:  auditCLIVersion,
		SafeMode:    safe,
		System:      sys,
		Services: auditServices{
			PM2:     pm2Svcs,
			Systemd: sysdSvcs,
			Docker:  docker,
		},
		Network:       netStatus,
		Observability: obs,
		RiskHints:     hints,
	}

	redReport := redactionReport{
		GeneratedAt: now.UTC().Format(time.RFC3339),
		SafeMode:    safe,
		Events:      redactions,
	}

	// ── Output ───────────────────────────────────────────────────────────────
	if auditJSONOnly {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}

	if err := os.MkdirAll(outDir, 0700); err != nil {
		return fmt.Errorf("cannot create output directory: %w", err)
	}

	if err := writeJSON(filepath.Join(outDir, "audit.json"), report); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(outDir, "redaction_report.json"), redReport); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outDir, "audit.md"), []byte(buildMarkdown(report)), 0600); err != nil {
		return err
	}

	fmt.Printf("\n✓ Audit dossier written to: %s/\n", outDir)
	fmt.Printf("    audit.json            — structured data\n")
	fmt.Printf("    audit.md              — human-readable report\n")
	fmt.Printf("    redaction_report.json — redaction log\n")

	if auditBundle {
		bundlePath, err := bundleDossier(outDir)
		if err != nil {
			fmt.Printf("\n⚠ Bundle failed: %v\n", err)
		} else {
			fmt.Printf("    %-22s — compressed archive\n", filepath.Base(bundlePath))
		}
	}

	fmt.Println()
	if len(hints) > 0 {
		fmt.Printf("  %d risk hint(s) detected — see audit.md §Risk Hints\n", len(hints))
	}
	fmt.Println("\nSibil Audit collects the facts.")
	fmt.Println("The expert turns them into decisions.")
	return nil
}

// ── Observability signals ─────────────────────────────────────────────────────

func buildObservabilitySignals(
	pm2 []collect.AuditPM2Service,
	sysd []collect.AuditSystemdService,
	listeners []collect.NetworkListener,
	detector string,
) []obsSignal {
	// Build port → process map for cross-referencing
	portByProcess := map[string]int{}
	for _, l := range listeners {
		if l.Process != "" && l.Port > 0 {
			if _, exists := portByProcess[l.Process]; !exists {
				portByProcess[l.Process] = l.Port
			}
		}
	}

	var signals []obsSignal

	for _, svc := range pm2 {
		sig := obsSignal{
			Service:           svc.Name,
			ManagedBy:         "pm2",
			StackGuess:        guessStack(svc.Name, svc.Interpreter, svc.Script),
			LogsAccessible:    checkLogsAccessible(svc.Name, detector),
			RecentRestarts:    svc.Restarts,
			HasHealthEndpoint: "unknown",
		}
		if port, ok := portByProcess[svc.Name]; ok {
			p := port
			sig.PortDetected = &p
		}
		sig.ObservabilityScore, sig.Gaps = scoreObservability(sig)
		signals = append(signals, sig)
	}

	for _, svc := range sysd {
		sig := obsSignal{
			Service:           svc.Unit,
			ManagedBy:         "systemd",
			StackGuess:        guessStack(svc.Unit, "", ""),
			LogsAccessible:    checkLogsAccessible(svc.Unit, "systemd"),
			RecentRestarts:    0, // systemd doesn't expose restarts in list-units
			HasHealthEndpoint: "unknown",
		}
		if port, ok := portByProcess[svc.Unit]; ok {
			p := port
			sig.PortDetected = &p
		}
		sig.ObservabilityScore, sig.Gaps = scoreObservability(sig)
		signals = append(signals, sig)
	}

	return signals
}

// guessStack returns a best-effort stack name from service metadata.
func guessStack(name, interpreter, script string) string {
	nameLower := strings.ToLower(name)
	scriptLower := strings.ToLower(script)

	knownStacks := map[string]string{
		"nginx": "nginx", "caddy": "caddy", "apache": "apache",
		"redis": "redis", "postgres": "postgres", "mysql": "mysql",
		"mongo": "mongodb", "rabbit": "rabbitmq", "kafka": "kafka",
	}
	for key, stack := range knownStacks {
		if strings.Contains(nameLower, key) {
			return stack
		}
	}

	interp := strings.ToLower(interpreter)
	switch {
	case strings.Contains(interp, "node") || strings.HasSuffix(scriptLower, ".js") || strings.HasSuffix(scriptLower, ".mjs"):
		return "node"
	case strings.Contains(interp, "python") || strings.HasSuffix(scriptLower, ".py"):
		return "python"
	case strings.Contains(interp, "bun"):
		return "bun"
	case strings.Contains(interp, "ruby") || strings.HasSuffix(scriptLower, ".rb"):
		return "ruby"
	case strings.Contains(interp, "php") || strings.HasSuffix(scriptLower, ".php"):
		return "php"
	}
	return "unknown"
}

func checkLogsAccessible(name, detector string) bool {
	_, err := collect.ServiceLogs(name, 1, detector)
	return err == nil
}

func scoreObservability(sig obsSignal) (score float64, gaps []string) {
	if sig.LogsAccessible {
		score += 0.35
	} else {
		gaps = append(gaps, "logs_not_accessible")
	}
	if sig.RecentRestarts < 5 {
		score += 0.30
	} else {
		gaps = append(gaps, "abnormal_restart_count")
	}
	if sig.PortDetected != nil {
		score += 0.25
	} else {
		gaps = append(gaps, "port_not_detected")
	}
	if sig.StackGuess == "unknown" {
		gaps = append(gaps, "stack_unknown")
	} else {
		score += 0.10
	}
	gaps = append(gaps, "health_endpoint_not_confirmed")
	score = math.Round(score*100) / 100
	return
}

// ── Risk hints ────────────────────────────────────────────────────────────────

func buildRiskHints(listeners []collect.NetworkListener, obs []obsSignal, sys *collect.AuditSystem) []riskHint {
	var hints []riskHint

	// Disk pressure
	if sys != nil {
		for _, d := range sys.Disk {
			if d.UsedPercent >= 90 {
				hints = append(hints, riskHint{
					Level:               "high",
					Title:               "Critical disk pressure",
					Evidence:            fmt.Sprintf("%s at %.1f%% used", d.Mount, d.UsedPercent),
					Confidence:          0.99,
					RequiresHumanReview: true,
				})
			} else if d.UsedPercent >= 75 {
				hints = append(hints, riskHint{
					Level:               "medium",
					Title:               "Elevated disk usage",
					Evidence:            fmt.Sprintf("%s at %.1f%% used", d.Mount, d.UsedPercent),
					Confidence:          0.95,
					RequiresHumanReview: true,
				})
			}
		}
	}

	// Public listeners that aren't expected to be public
	knownPublicProcesses := map[string]bool{"nginx": true, "caddy": true, "apache2": true, "httpd": true}
	knownPublicPorts := map[int]bool{22: true, 80: true, 443: true}
	for _, l := range listeners {
		if l.Exposure != "public_interface" {
			continue
		}
		if knownPublicProcesses[l.Process] || knownPublicPorts[l.Port] {
			continue
		}
		process := l.Process
		if process == "" {
			process = "unknown process"
		}
		hints = append(hints, riskHint{
			Level:               "medium",
			Title:               "Unexpected public listener",
			Evidence:            fmt.Sprintf("%s:%d (%s)", l.Address, l.Port, process),
			Confidence:          0.72,
			RequiresHumanReview: true,
		})
	}

	// Abnormal restart counts
	for _, sig := range obs {
		if sig.RecentRestarts >= 5 {
			hints = append(hints, riskHint{
				Level:               "medium",
				Title:               "Abnormal restart count",
				Evidence:            fmt.Sprintf("service %q restarted %d times", sig.Service, sig.RecentRestarts),
				Confidence:          0.81,
				RequiresHumanReview: true,
			})
		}
	}

	// Services with no logs accessible
	noLogs := []string{}
	for _, sig := range obs {
		if !sig.LogsAccessible {
			noLogs = append(noLogs, sig.Service)
		}
	}
	if len(noLogs) > 0 {
		hints = append(hints, riskHint{
			Level:               "low",
			Title:               "Services with inaccessible logs",
			Evidence:            strings.Join(noLogs, ", "),
			Confidence:          0.65,
			RequiresHumanReview: true,
		})
	}

	return hints
}

// ── Markdown report ───────────────────────────────────────────────────────────

func buildMarkdown(r auditReport) string {
	var b strings.Builder

	b.WriteString("# Sibil Audit Report\n\n")
	b.WriteString(fmt.Sprintf("Generated: %s  \n", r.GeneratedAt))
	b.WriteString(fmt.Sprintf("CLI version: %s  \n", r.CLIVersion))
	b.WriteString(fmt.Sprintf("Safe mode: %v  \n\n", r.SafeMode))
	b.WriteString("> Generated locally by Sibil CLI. No logs or secrets were uploaded.\n\n")
	b.WriteString("---\n\n")

	// Executive Summary
	b.WriteString("## Executive Summary\n\n")
	b.WriteString("| Metric | Value |\n|---|---|\n")
	b.WriteString(fmt.Sprintf("| PM2 services | %d |\n", len(r.Services.PM2)))
	b.WriteString(fmt.Sprintf("| systemd services | %d |\n", len(r.Services.Systemd)))
	b.WriteString(fmt.Sprintf("| Network listeners | %d |\n", len(r.Network.Listeners)))
	b.WriteString(fmt.Sprintf("| Observability signals | %d |\n", len(r.Observability)))
	b.WriteString(fmt.Sprintf("| Risk hints | %d |\n", len(r.RiskHints)))
	redacted := "no"
	if r.SafeMode {
		redacted = "yes"
	}
	b.WriteString(fmt.Sprintf("| Redactions applied | %s |\n\n", redacted))

	if len(r.RiskHints) > 0 {
		b.WriteString("**Top risk hints:**\n")
		limit := len(r.RiskHints)
		if limit > 3 {
			limit = 3
		}
		for i, h := range r.RiskHints[:limit] {
			b.WriteString(fmt.Sprintf("%d. `[%s]` %s — %s\n", i+1, strings.ToUpper(h.Level), h.Title, h.Evidence))
		}
		b.WriteString("\n")
	}
	b.WriteString("---\n\n")

	// 1. System
	b.WriteString("## 1. System Snapshot\n\n")
	if r.System != nil {
		s := r.System
		b.WriteString(fmt.Sprintf("| Field | Value |\n|---|---|\n"))
		b.WriteString(fmt.Sprintf("| Hostname | %s |\n", s.Hostname))
		b.WriteString(fmt.Sprintf("| OS | %s |\n", s.OS))
		b.WriteString(fmt.Sprintf("| Kernel | %s |\n", s.Kernel))
		b.WriteString(fmt.Sprintf("| Uptime | %s |\n", formatUptimeSec(s.UptimeSeconds)))
		b.WriteString(fmt.Sprintf("| CPU | %d core(s) |\n", s.CPUCount))
		b.WriteString(fmt.Sprintf("| Load avg | %.2f / %.2f / %.2f |\n", s.LoadAvg[0], s.LoadAvg[1], s.LoadAvg[2]))
		b.WriteString(fmt.Sprintf("| RAM | %d MB used / %d MB total |\n", s.Memory.UsedMB, s.Memory.TotalMB))
		b.WriteString("\n**Disk:**\n\n")
		b.WriteString("| Mount | Used % |\n|---|---|\n")
		for _, d := range s.Disk {
			b.WriteString(fmt.Sprintf("| %s | %.1f%% |\n", d.Mount, d.UsedPercent))
		}
	} else {
		b.WriteString("_System snapshot unavailable._\n")
	}
	b.WriteString("\n")

	// 2. Runtime Inventory
	b.WriteString("## 2. Runtime Inventory\n\n")

	if len(r.Services.PM2) > 0 {
		b.WriteString("### PM2 Services\n\n")
		b.WriteString("| Name | Status | CPU | Memory (MB) | Restarts | Uptime | Stack |\n|---|---|---|---|---|---|---|\n")
		for _, s := range r.Services.PM2 {
			b.WriteString(fmt.Sprintf("| %s | %s | %.1f%% | %.1f | %d | %s | %s |\n",
				s.Name, s.Status, s.CPU, s.MemoryMB, s.Restarts, s.Uptime, s.Interpreter))
		}
		b.WriteString("\n")
	} else {
		b.WriteString("**PM2:** not detected\n\n")
	}

	if len(r.Services.Systemd) > 0 {
		b.WriteString("### systemd Services\n\n")
		b.WriteString("| Unit | Active | Sub-state | Enabled |\n|---|---|---|---|\n")
		for _, s := range r.Services.Systemd {
			b.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n", s.Unit, s.ActiveState, s.SubState, s.Enabled))
		}
		b.WriteString("\n")
	}

	b.WriteString("### Docker\n\n")
	switch r.Services.Docker.Status {
	case "detected":
		if len(r.Services.Docker.Containers) == 0 {
			b.WriteString("Docker detected — no containers found.\n\n")
		} else {
			b.WriteString("| Name | Image | State | Ports | Health |\n|---|---|---|---|---|\n")
			for _, c := range r.Services.Docker.Containers {
				b.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s |\n",
					c.Name, c.Image, c.State, c.Ports, c.Health))
			}
			b.WriteString("\n")
		}
	case "skipped":
		b.WriteString("_Docker inspection skipped. Use --include-docker to enable._\n\n")
	default:
		b.WriteString(fmt.Sprintf("Docker: %s\n\n", r.Services.Docker.Status))
	}

	// 3. Network Exposure
	b.WriteString("## 3. Network Exposure\n\n")
	if r.Network.Status == "error" {
		b.WriteString(fmt.Sprintf("_Network scan unavailable: %s_\n\n", r.Network.Error))
	} else if len(r.Network.Listeners) == 0 {
		b.WriteString("_No listeners detected._\n\n")
	} else {
		b.WriteString("| Address | Port | Process | Exposure | Notes |\n|---|---|---|---|---|\n")
		for _, l := range r.Network.Listeners {
			b.WriteString(fmt.Sprintf("| %s | %d | %s | %s | %s |\n",
				l.Address, l.Port, l.Process, l.Exposure, l.RiskHint))
		}
		b.WriteString("\n")
	}

	// 4. Observability Gaps
	b.WriteString("## 4. Observability Gaps\n\n")
	if len(r.Observability) == 0 {
		b.WriteString("_No services analyzed._\n\n")
	} else {
		b.WriteString("| Service | Managed by | Stack | Logs | Restarts | Port | Score | Gaps |\n|---|---|---|---|---|---|---|---|\n")
		for _, o := range r.Observability {
			port := "—"
			if o.PortDetected != nil {
				port = strconv.Itoa(*o.PortDetected)
			}
			logs := "no"
			if o.LogsAccessible {
				logs = "yes"
			}
			b.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %d | %s | %.2f | %s |\n",
				o.Service, o.ManagedBy, o.StackGuess, logs, o.RecentRestarts, port,
				o.ObservabilityScore, strings.Join(o.Gaps, ", ")))
		}
		b.WriteString("\n")
	}

	// 5. Sibil Monitoring Plan Draft
	b.WriteString("## 5. Sibil Monitoring Plan Draft\n\n")
	b.WriteString("_Based on detected services. Review before applying._\n\n")
	recommended := buildMonitoringPlanDraft(r)
	b.WriteString("**Recommended modules:**\n")
	for _, m := range recommended {
		b.WriteString(fmt.Sprintf("- %s\n", m))
	}
	b.WriteString("\n**Suggested refresh cadence:**\n")
	b.WriteString("- system metrics: 15s\n- services: 10s\n- logs: on demand\n\n")

	// 6. Risk Hints
	b.WriteString("## 6. Risk Hints\n\n")
	if len(r.RiskHints) == 0 {
		b.WriteString("_No risk hints generated._\n\n")
	} else {
		b.WriteString("> Risk hints are automated signals. Each requires human review before action.\n\n")
		for _, h := range r.RiskHints {
			b.WriteString(fmt.Sprintf("**[%s]** %s  \n", strings.ToUpper(h.Level), h.Title))
			b.WriteString(fmt.Sprintf("Evidence: `%s`  \n", h.Evidence))
			b.WriteString(fmt.Sprintf("Confidence: %.0f%%  \n\n", h.Confidence*100))
		}
	}

	// 7. Expert Review Notes
	b.WriteString("## 7. Expert Review Notes\n\n")
	b.WriteString("_This section is intentionally left for the human audit._\n\n")
	b.WriteString("- [ ] Priority risk register\n")
	b.WriteString("- [ ] Fallback and failure-path review\n")
	b.WriteString("- [ ] 7-day remediation roadmap\n")

	return b.String()
}

func buildMonitoringPlanDraft(r auditReport) []string {
	modules := []string{"system"}
	if len(r.Services.PM2) > 0 {
		modules = append(modules, "pm2")
	}
	if len(r.Services.Systemd) > 0 {
		modules = append(modules, "systemd")
	}
	// Detect well-known services from names
	seen := map[string]bool{}
	allNames := []string{}
	for _, s := range r.Services.PM2 {
		allNames = append(allNames, strings.ToLower(s.Name))
	}
	for _, s := range r.Services.Systemd {
		allNames = append(allNames, strings.ToLower(s.Unit))
	}
	for _, proc := range r.Network.Listeners {
		allNames = append(allNames, strings.ToLower(proc.Process))
	}
	candidates := []string{"nginx", "caddy", "redis", "postgres", "mysql", "mongodb", "rabbitmq", "kafka"}
	for _, name := range allNames {
		for _, c := range candidates {
			if strings.Contains(name, c) && !seen[c] {
				modules = append(modules, c)
				seen[c] = true
			}
		}
	}
	if r.Services.Docker.Status == "detected" && len(r.Services.Docker.Containers) > 0 {
		modules = append(modules, "docker")
	}
	return modules
}

// ── Bundle ────────────────────────────────────────────────────────────────────

func bundleDossier(dir string) (string, error) {
	bundlePath := filepath.Clean(dir) + ".tar.gz"
	f, err := os.Create(bundlePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()

	cleanDir := filepath.Clean(dir)
	base := filepath.Base(cleanDir)
	err = filepath.Walk(cleanDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(cleanDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			hdr.Name = base + "/"
		} else {
			hdr.Name = filepath.Join(base, rel)
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		src, err := os.Open(path)
		if err != nil {
			return err
		}
		defer src.Close()
		_, err = io.Copy(tw, src)
		return err
	})
	if err != nil {
		return "", err
	}
	return bundlePath, nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func writeJSON(path string, v any) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func formatUptimeSec(sec uint64) string {
	d := time.Duration(sec) * time.Second
	days := int(d.Hours() / 24)
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	if days > 0 {
		return fmt.Sprintf("%dd %dh", days, hours)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, mins)
	}
	return fmt.Sprintf("%dm", mins)
}

func auditCLIVersionString() string { return auditCLIVersion }

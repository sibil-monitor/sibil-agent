package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sibil-monitor/sibil-agent/config"
	"github.com/spf13/cobra"
)

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove Sibil from this server (shows what will be removed first)",
	Long: `sibil uninstall removes all Sibil components from this server.

Without --yes, it only shows what would be removed (dry run).
Nothing is deleted until you confirm with --yes.

What it removes:
  - The sibil binary
  - The sibil PM2 process (if detected)
  - The sibil systemd unit (if detected)
  - sibil.json config file
  - .sibil-passes/ directory
  - ~/.sibil/ directory (manifest + any cached data)

What it NEVER touches:
  - Your PM2 applications (other than the sibil process)
  - Your systemd services (other than sibil.service)
  - Your logs, databases, and firewall rules

Use --path to specify the working directory where sibil was initialised
if it differs from the current directory.`,
	RunE: runUninstall,
}

func init() {
	uninstallCmd.Flags().Bool("yes", false, "confirm and execute the uninstall")
	uninstallCmd.Flags().String("path", "", "directory where sibil was initialised (if not current directory)")
	uninstallCmd.Flags().String("config", "", "explicit path to sibil.json")
}

// scanEntry represents one item that uninstall will look for.
type scanEntry struct {
	label  string
	kind   string // "file" | "dir" | "pm2" | "systemd"
	path   string
	found  bool
	source string // "manifest" | "scan"
}

func runUninstall(cmd *cobra.Command, args []string) error {
	yes, _ := cmd.Flags().GetBool("yes")
	customPath, _ := cmd.Flags().GetString("path")
	customConfig, _ := cmd.Flags().GetString("config")

	home, _ := os.UserHomeDir()

	// ── 1. Try manifest first ──────────────────────────────────────────────────
	manifest, manifestErr := config.ReadManifest()

	// ── 2. Build scan list ────────────────────────────────────────────────────
	entries := []*scanEntry{}

	// Binary
	binaryPath, _ := os.Executable()
	if resolved, err := filepath.EvalSymlinks(binaryPath); err == nil {
		binaryPath = resolved
	}
	if manifest != nil && manifest.Binary != "" {
		binaryPath = manifest.Binary
	}
	entries = append(entries, &scanEntry{
		label:  fmt.Sprintf("Binary:           %s", binaryPath),
		kind:   "file",
		path:   binaryPath,
		source: manifestSource(manifest, "binary"),
	})

	// Config file — from manifest, flag, custom path, or CWD
	configPaths := candidateConfigPaths(manifest, customConfig, customPath)
	for _, p := range configPaths {
		src := "scan"
		if manifest != nil && manifest.Config == p {
			src = "manifest"
		}
		entries = append(entries, &scanEntry{
			label:  fmt.Sprintf("Config file:      %s", p),
			kind:   "file",
			path:   p,
			source: src,
		})
	}

	// Passes directory — from manifest, next to config, CWD, home
	passesDirs := candidatePassesDirs(manifest, configPaths, customPath, home)
	for _, p := range passesDirs {
		src := "scan"
		if manifest != nil && manifest.PassesDir == p {
			src = "manifest"
		}
		entries = append(entries, &scanEntry{
			label:  fmt.Sprintf("Service passes:   %s/", p),
			kind:   "dir",
			path:   p,
			source: src,
		})
	}

	// ~/.sibil/ directory (manifest + cached data)
	if home != "" {
		sibDir := filepath.Join(home, ".sibil")
		entries = append(entries, &scanEntry{
			label:  fmt.Sprintf("Sibil home dir:   %s/", sibDir),
			kind:   "dir",
			path:   sibDir,
			source: "scan",
		})
	}

	// PM2 process named "sibil"
	entries = append(entries, &scanEntry{
		label:  "PM2 process:      sibil",
		kind:   "pm2",
		path:   "sibil",
		source: "scan",
	})

	// systemd unit sibil.service
	entries = append(entries, &scanEntry{
		label:  "systemd unit:     sibil.service",
		kind:   "systemd",
		path:   "sibil.service",
		source: "scan",
	})

	// ── 3. Check existence ────────────────────────────────────────────────────
	for _, e := range entries {
		switch e.kind {
		case "file":
			_, err := os.Stat(e.path)
			e.found = err == nil
		case "dir":
			_, err := os.Stat(e.path)
			e.found = err == nil
		case "pm2":
			e.found = pm2Running(e.path)
		case "systemd":
			e.found = systemdUnitActive(e.path)
		}
	}

	// ── 4. Print scan results ─────────────────────────────────────────────────
	fmt.Println("sibil uninstall — scan")
	fmt.Println()

	if manifestErr == nil {
		fmt.Println("  Install manifest found — using recorded paths.")
	} else {
		fmt.Println("  No install manifest found — scanning known paths.")
	}
	fmt.Println()

	detected := []*scanEntry{}
	notFound := []*scanEntry{}
	for _, e := range entries {
		if e.found {
			detected = append(detected, e)
		} else {
			notFound = append(notFound, e)
		}
	}

	if len(detected) > 0 {
		fmt.Println("  Detected:")
		for _, e := range detected {
			fmt.Printf("    ✓ %s\n", e.label)
		}
	}
	fmt.Println()
	if len(notFound) > 0 {
		fmt.Println("  Not found:")
		for _, e := range notFound {
			fmt.Printf("    - %s\n", e.label)
		}
	}

	if len(detected) == 0 {
		fmt.Println()
		fmt.Println("Nothing to remove — no Sibil components detected.")
		if customPath == "" {
			fmt.Println()
			fmt.Println("  If Sibil was initialised in a different directory, run:")
			fmt.Println("    sibil uninstall --path /path/to/your/sibil/workdir")
		}
		return nil
	}

	fmt.Println()
	fmt.Println("  This will NOT touch:")
	fmt.Println("    · Your PM2 applications (other than the sibil process)")
	fmt.Println("    · Your systemd services (other than sibil.service)")
	fmt.Println("    · Your logs, databases, and firewall rules")

	if !yes {
		fmt.Println()
		fmt.Println("Dry run — nothing was removed.")
		fmt.Println("To confirm: sibil uninstall --yes")
		if customPath == "" && len(notFound) > 0 {
			fmt.Println()
			fmt.Println("  Note: if you installed Sibil in a custom directory, run:")
			fmt.Println("    sibil uninstall --path /path/to/your/sibil/workdir")
		}
		return nil
	}

	// ── 5. Execute ────────────────────────────────────────────────────────────
	fmt.Println()
	fmt.Println("Uninstalling...")

	hadError := false
	for _, e := range detected {
		if !e.found {
			continue
		}
		switch e.kind {
		case "pm2":
			out, err := exec.Command("pm2", "delete", e.path).CombinedOutput()
			if err != nil {
				fmt.Printf("  ✗ PM2 delete %s: %v — %s\n", e.path, err, strings.TrimSpace(string(out)))
				hadError = true
			} else {
				fmt.Printf("  ✓ PM2 process %q deleted\n", e.path)
				exec.Command("pm2", "save").Run() //nolint:errcheck
			}
		case "systemd":
			if err := disableSystemdUnit(e.path); err != nil {
				fmt.Printf("  ✗ systemd disable %s: %v\n", e.path, err)
				hadError = true
			} else {
				fmt.Printf("  ✓ systemd unit %s disabled and stopped\n", e.path)
			}
		case "file":
			if err := os.Remove(e.path); err != nil {
				fmt.Printf("  ✗ Cannot remove %s: %v\n", e.path, err)
				hadError = true
			} else {
				fmt.Printf("  ✓ Removed %s\n", e.path)
			}
		case "dir":
			if err := os.RemoveAll(e.path); err != nil {
				fmt.Printf("  ✗ Cannot remove %s/: %v\n", e.path, err)
				hadError = true
			} else {
				fmt.Printf("  ✓ Removed %s/\n", e.path)
			}
		}
	}

	// Remove manifest last (after everything else)
	config.RemoveManifest() //nolint:errcheck

	fmt.Println()
	if hadError {
		fmt.Println("⚠  Uninstall completed with errors — see above.")
	} else {
		fmt.Println("✓ Sibil has been removed from this server.")
		fmt.Println()
		fmt.Println("  Your applications and services are untouched.")
	}
	return nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func manifestSource(manifest *config.InstallManifest, _ string) string {
	if manifest != nil {
		return "manifest"
	}
	return "scan"
}

func candidateConfigPaths(manifest *config.InstallManifest, customConfig, customPath string) []string {
	seen := map[string]bool{}
	var paths []string

	add := func(p string) {
		if p == "" {
			return
		}
		abs, err := filepath.Abs(p)
		if err != nil {
			abs = p
		}
		if !seen[abs] {
			seen[abs] = true
			paths = append(paths, abs)
		}
	}

	if manifest != nil && manifest.Config != "" {
		add(manifest.Config)
	}
	if customConfig != "" {
		add(customConfig)
	}
	if customPath != "" {
		add(filepath.Join(customPath, "sibil.json"))
	}
	// CWD
	add("sibil.json")
	// $SIBIL_CONFIG
	if p := os.Getenv("SIBIL_CONFIG"); p != "" {
		add(p)
	}

	return paths
}

func candidatePassesDirs(manifest *config.InstallManifest, configPaths []string, customPath string, home string) []string {
	seen := map[string]bool{}
	var dirs []string

	add := func(p string) {
		if p == "" {
			return
		}
		if !seen[p] {
			seen[p] = true
			dirs = append(dirs, p)
		}
	}

	if manifest != nil && manifest.PassesDir != "" {
		add(manifest.PassesDir)
	}
	// Next to each config file
	for _, cp := range configPaths {
		add(filepath.Join(filepath.Dir(cp), ".sibil-passes"))
	}
	if customPath != "" {
		add(filepath.Join(customPath, ".sibil-passes"))
	}
	// CWD
	if wd, err := os.Getwd(); err == nil {
		add(filepath.Join(wd, ".sibil-passes"))
	}
	// Home
	if home != "" {
		add(filepath.Join(home, ".sibil-passes"))
	}

	return dirs
}

func pm2Running(name string) bool {
	out, err := exec.Command("pm2", "jlist").Output()
	if err != nil {
		return false
	}
	var procs []map[string]any
	if err := json.Unmarshal(out, &procs); err != nil {
		return false
	}
	for _, p := range procs {
		if n, _ := p["name"].(string); n == name {
			return true
		}
	}
	return false
}

func systemdUnitActive(unit string) bool {
	out, err := exec.Command("systemctl", "is-active", unit).Output()
	if err != nil {
		// is-active returns non-zero for inactive/not-found
		return strings.TrimSpace(string(out)) == "active" ||
			strings.TrimSpace(string(out)) == "activating"
	}
	return true
}

func disableSystemdUnit(unit string) error {
	if err := exec.Command("systemctl", "stop", unit).Run(); err != nil {
		return err
	}
	return exec.Command("systemctl", "disable", unit).Run()
}

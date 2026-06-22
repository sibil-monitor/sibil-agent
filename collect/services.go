package collect

import (
	"encoding/json"
	"fmt"
	"math"
	"os/exec"
	"strings"
	"time"
)

type Service struct {
	Name        string  `json:"name"`
	Status      string  `json:"status"` // online | offline | warning
	CPU         float64 `json:"cpu,omitempty"`
	Memory      float64 `json:"memory,omitempty"`
	Uptime      string  `json:"uptime,omitempty"`
	Restarts    int     `json:"restarts,omitempty"`
	Description string  `json:"description,omitempty"`
}

// Services detects running services via PM2 or systemd (depending on detector).
// detector: "auto" | "pm2" | "systemd"
func Services(detector string) []Service {
	if detector == "pm2" {
		if svcs := fromPM2(); svcs != nil {
			return svcs
		}
		return []Service{}
	}
	if detector == "systemd" {
		if svcs := fromSystemd(); svcs != nil {
			return svcs
		}
		return []Service{}
	}
	// auto: try PM2 first, then systemd
	if svcs := fromPM2(); svcs != nil {
		return svcs
	}
	if svcs := fromSystemd(); svcs != nil {
		return svcs
	}
	return []Service{}
}

// ServiceLogs returns the last n lines of logs for a named service.
func ServiceLogs(name string, lines int, detector string) (string, error) {
	if lines <= 0 {
		lines = 100
	}
	// Try PM2 first
	if detector != "systemd" {
		out, err := exec.Command("pm2", "logs", name,
			"--lines", fmt.Sprintf("%d", lines),
			"--nostream", "--raw").Output()
		if err == nil {
			return string(out), nil
		}
	}
	// Try journalctl
	out, err := exec.Command("journalctl", "-u", name+".service",
		"-n", fmt.Sprintf("%d", lines),
		"--no-pager", "--output=short").Output()
	if err != nil {
		return "", fmt.Errorf("no log source found for service %q", name)
	}
	return string(out), nil
}

// Control executes start/stop/restart for a service.
func Control(action, name, detector string) error {
	// Try PM2
	if detector != "systemd" {
		if err := exec.Command("pm2", action, name).Run(); err == nil {
			return nil
		}
	}
	// Try systemd
	if err := exec.Command("systemctl", action, name+".service").Run(); err != nil {
		return fmt.Errorf("cannot %s service %q: %w", action, name, err)
	}
	return nil
}

// ── PM2 ──────────────────────────────────────────────────────────────────────

type pm2Process struct {
	Name  string `json:"name"`
	Monit struct {
		CPU    float64 `json:"cpu"`
		Memory uint64  `json:"memory"`
	} `json:"monit"`
	PM2Env struct {
		Status      string `json:"status"`
		PMUptime    int64  `json:"pm_uptime"`
		RestartTime int    `json:"restart_time"`
		ExecMode    string `json:"exec_mode"`
	} `json:"pm2_env"`
}

func fromPM2() []Service {
	out, err := exec.Command("pm2", "jlist").Output()
	if err != nil {
		return nil
	}
	var procs []pm2Process
	if err := json.Unmarshal(out, &procs); err != nil {
		return nil
	}
	svcs := make([]Service, 0, len(procs))
	for _, p := range procs {
		status := "offline"
		if p.PM2Env.Status == "online" {
			status = "online"
		}
		svcs = append(svcs, Service{
			Name:        p.Name,
			Status:      status,
			CPU:         math.Round(p.Monit.CPU*10) / 10,
			Uptime:      formatUptime(p.PM2Env.PMUptime),
			Restarts:    p.PM2Env.RestartTime,
			Description: "PM2 · " + p.PM2Env.ExecMode,
		})
	}
	return svcs
}

// ── systemd ───────────────────────────────────────────────────────────────────

func fromSystemd() []Service {
	out, err := exec.Command(
		"systemctl", "list-units", "--type=service",
		"--state=loaded", "--no-pager", "--no-legend", "--plain",
	).Output()
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	svcs := make([]Service, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		name := strings.TrimSuffix(fields[0], ".service")
		sub := fields[3] // "running", "exited", "failed", etc.
		status := "warning"
		if sub == "running" {
			status = "online"
		} else if sub == "exited" || sub == "failed" || sub == "dead" {
			status = "offline"
		}
		desc := ""
		if len(fields) > 4 {
			desc = strings.Join(fields[4:], " ")
		}
		svcs = append(svcs, Service{
			Name:        name,
			Status:      status,
			Description: desc,
		})
	}
	return svcs
}

// ── helpers ───────────────────────────────────────────────────────────────────

func formatUptime(startEpochMs int64) string {
	if startEpochMs == 0 {
		return ""
	}
	d := time.Since(time.UnixMilli(startEpochMs))
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

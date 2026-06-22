package collect

import (
	"math"
	"runtime"
	"strings"

	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
)

type AuditSystem struct {
	Hostname      string      `json:"hostname"`
	OS            string      `json:"os"`
	Kernel        string      `json:"kernel"`
	UptimeSeconds uint64      `json:"uptime_seconds"`
	CPUCount      int         `json:"cpu_count"`
	LoadAvg       [3]float64  `json:"load_avg"`
	Memory        AuditMemory `json:"memory"`
	Disk          []AuditDisk `json:"disk"`
}

type AuditMemory struct {
	TotalMB     uint64 `json:"total_mb"`
	UsedMB      uint64 `json:"used_mb"`
	AvailableMB uint64 `json:"available_mb"`
}

type AuditDisk struct {
	Mount       string  `json:"mount"`
	UsedPercent float64 `json:"used_percent"`
}

func AuditSystemSnapshot(redactHostname bool) (*AuditSystem, error) {
	info, err := host.Info()
	if err != nil {
		return nil, err
	}

	hostname := info.Hostname
	if redactHostname {
		hostname = "redacted-host"
	}

	avg, err := load.Avg()
	if err != nil {
		return nil, err
	}

	vm, err := mem.VirtualMemory()
	if err != nil {
		return nil, err
	}

	partitions, _ := disk.Partitions(false)
	var disks []AuditDisk
	seen := map[string]bool{}
	for _, p := range partitions {
		if seen[p.Mountpoint] {
			continue
		}
		seen[p.Mountpoint] = true
		u, err := disk.Usage(p.Mountpoint)
		if err != nil {
			continue
		}
		disks = append(disks, AuditDisk{
			Mount:       p.Mountpoint,
			UsedPercent: math.Round(u.UsedPercent*10) / 10,
		})
	}
	if len(disks) == 0 {
		if u, err := disk.Usage("/"); err == nil {
			disks = []AuditDisk{{Mount: "/", UsedPercent: math.Round(u.UsedPercent*10) / 10}}
		}
	}

	osName := strings.TrimSpace(info.Platform + " " + info.PlatformVersion)

	return &AuditSystem{
		Hostname:      hostname,
		OS:            osName,
		Kernel:        info.KernelVersion,
		UptimeSeconds: info.Uptime,
		CPUCount:      runtime.NumCPU(),
		LoadAvg:       [3]float64{avg.Load1, avg.Load5, avg.Load15},
		Memory: AuditMemory{
			TotalMB:     vm.Total / 1024 / 1024,
			UsedMB:      vm.Used / 1024 / 1024,
			AvailableMB: vm.Available / 1024 / 1024,
		},
		Disk: disks,
	}, nil
}

// AuditPM2Service is a richer PM2 service entry for audit reports.
type AuditPM2Service struct {
	Name        string  `json:"name"`
	Status      string  `json:"status"`
	CPU         float64 `json:"cpu"`
	MemoryMB    float64 `json:"memory_mb"`
	Uptime      string  `json:"uptime,omitempty"`
	Restarts    int     `json:"restarts"`
	ExecMode    string  `json:"exec_mode,omitempty"`
	Interpreter string  `json:"interpreter,omitempty"`
	Script      string  `json:"script,omitempty"`
}

type pm2AuditProcess struct {
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
		Interpreter string `json:"exec_interpreter"`
		Script      string `json:"script"`
	} `json:"pm2_env"`
}

func AuditPM2Services(redactPaths bool) ([]AuditPM2Service, error) {
	out, err := execCommand("pm2", "jlist")
	if err != nil {
		return nil, err
	}
	var procs []pm2AuditProcess
	if err := jsonUnmarshal(out, &procs); err != nil {
		return nil, err
	}
	result := make([]AuditPM2Service, 0, len(procs))
	for _, p := range procs {
		status := "offline"
		if p.PM2Env.Status == "online" {
			status = "online"
		}
		script := p.PM2Env.Script
		if redactPaths && script != "" {
			script = redactPath(script)
		}
		interp := p.PM2Env.Interpreter
		if interp == "node" || interp == "" {
			interp = "node"
		}
		result = append(result, AuditPM2Service{
			Name:        p.Name,
			Status:      status,
			CPU:         math.Round(p.Monit.CPU*10) / 10,
			MemoryMB:    math.Round(float64(p.Monit.Memory)/1024/1024*10) / 10,
			Uptime:      formatUptime(p.PM2Env.PMUptime),
			Restarts:    p.PM2Env.RestartTime,
			ExecMode:    p.PM2Env.ExecMode,
			Interpreter: interp,
			Script:      script,
		})
	}
	return result, nil
}

// AuditSystemdService is a richer systemd entry for audit reports.
type AuditSystemdService struct {
	Unit        string `json:"unit"`
	ActiveState string `json:"active_state"`
	SubState    string `json:"sub_state"`
	Enabled     string `json:"enabled"` // enabled | disabled | static | masked
	Since       string `json:"since,omitempty"`
}

func AuditSystemdServices() ([]AuditSystemdService, error) {
	// --state=active limits to services that are currently active (running/exited/activating).
	// This avoids enumerating every loaded-but-inactive unit (can be 200+).
	out, err := execCommand(
		"systemctl", "list-units", "--type=service",
		"--state=active", "--no-pager", "--no-legend", "--plain",
	)
	if err != nil {
		return nil, err
	}

	// Fetch enabled state in one shot via list-unit-files.
	enabledMap := map[string]string{}
	if eout, err := execCommand("systemctl", "list-unit-files", "--type=service",
		"--no-pager", "--no-legend", "--plain"); err == nil {
		for _, line := range splitLines(string(eout)) {
			f := splitFields(line)
			if len(f) >= 2 {
				enabledMap[f[0]] = f[1]
			}
		}
	}

	lines := splitLines(string(out))
	result := make([]AuditSystemdService, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		fields := splitFields(line)
		if len(fields) < 4 {
			continue
		}
		unitFull := fields[0]
		unit := strings.TrimSuffix(unitFull, ".service")
		active := fields[2]
		sub := fields[3]
		enabled := enabledMap[unitFull]
		if enabled == "" {
			enabled = "unknown"
		}

		result = append(result, AuditSystemdService{
			Unit:        unit,
			ActiveState: active,
			SubState:    sub,
			Enabled:     enabled,
		})
	}
	return result, nil
}

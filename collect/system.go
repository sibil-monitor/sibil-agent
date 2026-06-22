package collect

import (
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
)

type SystemMetrics struct {
	LoadAvg   [3]float64 `json:"loadAvg"`
	MemUsed   uint64     `json:"memUsed"`  // GiB
	MemTotal  uint64     `json:"memTotal"` // GiB
	DiskUsed  float64    `json:"diskUsed"` // percent
	DiskTotal float64    `json:"diskTotal"`
	Uptime    uint64     `json:"uptime"` // seconds
}

func System() (*SystemMetrics, error) {
	avg, err := load.Avg()
	if err != nil {
		return nil, err
	}

	vm, err := mem.VirtualMemory()
	if err != nil {
		return nil, err
	}

	du, err := disk.Usage("/")
	if err != nil {
		return nil, err
	}

	info, err := host.Info()
	if err != nil {
		return nil, err
	}

	return &SystemMetrics{
		LoadAvg:   [3]float64{avg.Load1, avg.Load5, avg.Load15},
		MemUsed:   vm.Used / 1024 / 1024 / 1024,
		MemTotal:  vm.Total / 1024 / 1024 / 1024,
		DiskUsed:  du.UsedPercent,
		DiskTotal: 100,
		Uptime:    info.Uptime,
	}, nil
}

package metrics

import (
	"time"
)

type SystemStats struct {
	CPUUsage  float64   `json:"cpu_usage"`
	RAMUsed   uint64    `json:"ram_used"`
	RAMTotal  uint64    `json:"ram_total"`
	RAMUsage  float64   `json:"ram_usage"`
	DiskUsed  uint64    `json:"disk_used"`
	DiskTotal uint64    `json:"disk_total"`
	DiskUsage float64   `json:"disk_usage"`
	Timestamp time.Time `json:"timestamp"`
}

// CollectStats gathers all stats into a single struct
func CollectStats() (*SystemStats, error) {
	cpu, _ := GetCPUUsage(100 * time.Millisecond)
	ramUsed, ramTotal, ramUsage, _ := GetRAMStats()
	diskUsed, diskTotal, diskUsage, _ := GetDiskStats("/")

	return &SystemStats{
		CPUUsage:  cpu,
		RAMUsed:   ramUsed,
		RAMTotal:  ramTotal,
		RAMUsage:  ramUsage,
		DiskUsed:  diskUsed,
		DiskTotal: diskTotal,
		DiskUsage: diskUsage,
		Timestamp: time.Now(),
	}, nil
}

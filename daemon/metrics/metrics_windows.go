//go:build windows

package metrics

import (
	"math/rand"
	"time"
)

// GetCPUUsage returns mock CPU usage for Windows testing
func GetCPUUsage(delay time.Duration) (float64, error) {
	time.Sleep(delay)
	// Return a random CPU value between 5% and 45% for mock
	return 5.0 + rand.Float64()*40.0, nil
}

// GetRAMStats returns mock RAM stats for Windows testing
func GetRAMStats() (uint64, uint64, float64, error) {
	total := uint64(16 * 1024 * 1024 * 1024) // 16GB
	used := uint64(4 * 1024 * 1024 * 1024)   // 4GB used
	usage := (float64(used) / float64(total)) * 100.0
	return used, total, usage, nil
}

// GetDiskStats returns mock Disk stats for Windows testing
func GetDiskStats(path string) (uint64, uint64, float64, error) {
	total := uint64(256 * 1024 * 1024 * 1024) // 256GB
	used := uint64(80 * 1024 * 1024 * 1024)   // 80GB used
	usage := (float64(used) / float64(total)) * 100.0
	return used, total, usage, nil
}

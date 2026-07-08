//go:build linux

package metrics

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// GetCPUUsage reads /proc/stat twice with a delay to calculate CPU usage
func GetCPUUsage(delay time.Duration) (float64, error) {
	idle1, total1, err := readCPUStats()
	if err != nil {
		return 0, err
	}

	time.Sleep(delay)

	idle2, total2, err := readCPUStats()
	if err != nil {
		return 0, err
	}

	idleDelta := idle2 - idle1
	totalDelta := total2 - total1

	if totalDelta == 0 {
		return 0, nil
	}

	return 100.0 * (1.0 - float64(idleDelta)/float64(totalDelta)), nil
}

func readCPUStats() (uint64, uint64, error) {
	file, err := os.Open("/proc/stat")
	if err != nil {
		return 0, 0, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 5 || fields[0] != "cpu" {
			return 0, 0, fmt.Errorf("invalid /proc/stat format")
		}

		var total uint64
		var idle uint64

		for i, field := range fields[1:] {
			val, err := strconv.ParseUint(field, 10, 64)
			if err != nil {
				return 0, 0, err
			}
			total += val
			if i == 3 { // 4th column is 'idle'
				idle = val
			}
		}
		return idle, total, nil
	}
	return 0, 0, fmt.Errorf("could not read cpu stats")
}

// GetRAMStats reads /proc/meminfo to calculate memory stats
func GetRAMStats() (uint64, uint64, float64, error) {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0, 0, err
	}
	defer file.Close()

	var total, free, buffers, cached uint64
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		key := strings.TrimSuffix(fields[0], ":")
		val, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		switch key {
		case "MemTotal":
			total = val * 1024
		case "MemFree":
			free = val * 1024
		case "Buffers":
			buffers = val * 1024
		case "Cached":
			cached = val * 1024
		}
	}

	used := total - (free + buffers + cached)
	var usage float64
	if total > 0 {
		usage = (float64(used) / float64(total)) * 100.0
	}
	return used, total, usage, nil
}

// GetDiskStats gets disk stats using syscall.Statfs (Linux compat)
func GetDiskStats(path string) (uint64, uint64, float64, error) {
	var stat syscall.Statfs_t
	err := syscall.Statfs(path, &stat)
	if err != nil {
		return 0, 0, 0, err
	}

	total := stat.Blocks * uint64(stat.Bsize)
	free := stat.Bfree * uint64(stat.Bsize)
	used := total - free

	var usage float64
	if total > 0 {
		usage = (float64(used) / float64(total)) * 100.0
	}
	return used, total, usage, nil
}

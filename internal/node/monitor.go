package node

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// SystemStats collects node host metrics.
type SystemStats struct {
	startTime    time.Time
	cpuMu        sync.Mutex
	lastCPUTotal uint64
	lastCPUIdle  uint64
	lastCPUUsage float64
}

// NewSystemStats creates a new system stats collector.
func NewSystemStats() *SystemStats {
	s := &SystemStats{
		startTime: time.Now(),
	}

	if total, idle, err := readLinuxCPUStat(); err == nil {
		s.lastCPUTotal = total
		s.lastCPUIdle = idle
	}

	return s
}

// GetCPUUsage returns host-wide CPU usage percentage.
func (s *SystemStats) GetCPUUsage() float64 {
	total, idle, err := readLinuxCPUStat()
	if err != nil {
		return s.fallbackCPUUsage()
	}

	s.cpuMu.Lock()
	defer s.cpuMu.Unlock()

	if s.lastCPUTotal == 0 || total < s.lastCPUTotal || idle < s.lastCPUIdle {
		s.lastCPUTotal = total
		s.lastCPUIdle = idle
		return s.lastCPUUsage
	}

	totalDelta := total - s.lastCPUTotal
	idleDelta := idle - s.lastCPUIdle

	s.lastCPUTotal = total
	s.lastCPUIdle = idle

	if totalDelta == 0 {
		return s.lastCPUUsage
	}

	usage := 100 * float64(totalDelta-idleDelta) / float64(totalDelta)
	if usage < 0 {
		usage = 0
	}
	if usage > 100 {
		usage = 100
	}

	s.lastCPUUsage = usage
	return usage
}

// GetMemoryUsage returns host-wide used and total memory in MB.
func (s *SystemStats) GetMemoryUsage() (usedMB, totalMB float64) {
	usedMB, totalMB, err := readLinuxMemInfo()
	if err != nil {
		return s.fallbackMemUsage(), 0
	}
	return usedMB, totalMB
}

// GetUptime returns uptime in seconds.
func (s *SystemStats) GetUptime() int64 {
	return int64(time.Since(s.startTime).Seconds())
}

// GetGoRoutines returns the number of goroutines.
func (s *SystemStats) GetGoRoutines() int {
	return runtime.NumGoroutine()
}

func (s *SystemStats) fallbackCPUUsage() float64 {
	numGoroutines := runtime.NumGoroutine()
	numCPU := runtime.NumCPU()
	usage := float64(numGoroutines) / float64(numCPU) * 5.0
	if usage > 100 {
		usage = 100
	}
	return usage
}

func (s *SystemStats) fallbackMemUsage() float64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return float64(m.Alloc) / 1024 / 1024
}

func readLinuxCPUStat() (total, idle uint64, err error) {
	if runtime.GOOS != "linux" {
		return 0, 0, fmt.Errorf("linux cpu stat unavailable")
	}

	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0, 0, err
	}

	line := strings.SplitN(string(data), "\n", 2)[0]
	fields := strings.Fields(line)
	if len(fields) < 5 || fields[0] != "cpu" {
		return 0, 0, fmt.Errorf("unexpected /proc/stat format")
	}

	for i, field := range fields[1:] {
		value, parseErr := strconv.ParseUint(field, 10, 64)
		if parseErr != nil {
			return 0, 0, parseErr
		}
		total += value
		if i == 3 || i == 4 {
			idle += value
		}
	}

	return total, idle, nil
}

func readLinuxMemInfo() (usedMB, totalMB float64, err error) {
	if runtime.GOOS != "linux" {
		return 0, 0, fmt.Errorf("linux meminfo unavailable")
	}

	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, 0, err
	}

	values := make(map[string]uint64)
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		value, parseErr := strconv.ParseUint(fields[1], 10, 64)
		if parseErr != nil {
			continue
		}

		values[strings.TrimSuffix(fields[0], ":")] = value
	}

	totalKB := values["MemTotal"]
	if totalKB == 0 {
		return 0, 0, fmt.Errorf("MemTotal missing from /proc/meminfo")
	}

	availableKB := values["MemAvailable"]
	if availableKB == 0 {
		availableKB = values["MemFree"] + values["Buffers"] + values["Cached"]
	}

	usedKB := uint64(0)
	if totalKB > availableKB {
		usedKB = totalKB - availableKB
	}

	return float64(usedKB) / 1024, float64(totalKB) / 1024, nil
}

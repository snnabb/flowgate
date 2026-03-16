package node

import (
	"runtime"
	"time"
)

// SystemStats collects system resource usage
type SystemStats struct {
	startTime time.Time
}

// NewSystemStats creates a new system stats collector
func NewSystemStats() *SystemStats {
	return &SystemStats{
		startTime: time.Now(),
	}
}

// GetCPUUsage returns an approximate CPU usage percentage
func (s *SystemStats) GetCPUUsage() float64 {
	numGoroutines := runtime.NumGoroutine()
	numCPU := runtime.NumCPU()
	usage := float64(numGoroutines) / float64(numCPU) * 5.0
	if usage > 100 {
		usage = 100
	}
	return usage
}

// GetMemUsage returns memory usage in MB
func (s *SystemStats) GetMemUsage() float64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return float64(m.Alloc) / 1024 / 1024
}

// GetUptime returns uptime in seconds
func (s *SystemStats) GetUptime() int64 {
	return int64(time.Since(s.startTime).Seconds())
}

// GetGoRoutines returns the number of goroutines
func (s *SystemStats) GetGoRoutines() int {
	return runtime.NumGoroutine()
}

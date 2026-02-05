package logview

import (
	"cm/internal/docker"
	"sync"
)

const (
	// MaxStatsHistory is the number of stats samples to keep (60 seconds at 1s intervals)
	MaxStatsHistory = 60
)

// StatsHistory maintains a rolling buffer of container stats for graphing
type StatsHistory struct {
	mu      sync.RWMutex
	samples []docker.ContainerStats
}

// NewStatsHistory creates a new stats history buffer
func NewStatsHistory() *StatsHistory {
	return &StatsHistory{
		samples: make([]docker.ContainerStats, 0, MaxStatsHistory),
	}
}

// Add adds a new stats sample to the history
func (h *StatsHistory) Add(stats docker.ContainerStats) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.samples = append(h.samples, stats)
	if len(h.samples) > MaxStatsHistory {
		h.samples = h.samples[len(h.samples)-MaxStatsHistory:]
	}
}

// GetAll returns a copy of all samples in the history
func (h *StatsHistory) GetAll() []docker.ContainerStats {
	h.mu.RLock()
	defer h.mu.RUnlock()

	result := make([]docker.ContainerStats, len(h.samples))
	copy(result, h.samples)
	return result
}

// GetCPUValues returns all CPU percentage values for graphing
func (h *StatsHistory) GetCPUValues() []float64 {
	h.mu.RLock()
	defer h.mu.RUnlock()

	values := make([]float64, len(h.samples))
	for i, s := range h.samples {
		values[i] = s.CPUPercent
	}
	return values
}

// GetMemoryValues returns all memory percentage values for graphing
func (h *StatsHistory) GetMemoryValues() []float64 {
	h.mu.RLock()
	defer h.mu.RUnlock()

	values := make([]float64, len(h.samples))
	for i, s := range h.samples {
		values[i] = s.MemoryPercent
	}
	return values
}

// Latest returns the most recent stats sample, or nil if empty
func (h *StatsHistory) Latest() *docker.ContainerStats {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if len(h.samples) == 0 {
		return nil
	}
	stats := h.samples[len(h.samples)-1]
	return &stats
}

// Clear removes all samples from the history
func (h *StatsHistory) Clear() {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.samples = h.samples[:0]
}

// Len returns the number of samples in the history
func (h *StatsHistory) Len() int {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return len(h.samples)
}

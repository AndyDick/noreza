package internal

import (
	"fmt"
	"os"
	"sort"
	"sync"
	"time"
)

// LatencyProfiler collects latency samples for input-to-output timing analysis
type LatencyProfiler struct {
	mu      sync.Mutex
	samples []int64 // latency in nanoseconds
}

// NewLatencyProfiler creates a new latency profiler
func NewLatencyProfiler() *LatencyProfiler {
	return &LatencyProfiler{
		samples: make([]int64, 0, 10000),
	}
}

// Record adds a latency sample (in nanoseconds)
func (p *LatencyProfiler) Record(startNs int64) {
	latency := time.Now().UnixNano() - startNs
	p.mu.Lock()
	p.samples = append(p.samples, latency)
	p.mu.Unlock()
}

// WriteToFile writes the latency profile to the specified file
func (p *LatencyProfiler) WriteToFile(path string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.samples) == 0 {
		return fmt.Errorf("no latency samples collected")
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	// Sort samples for percentile calculations
	sorted := make([]int64, len(p.samples))
	copy(sorted, p.samples)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	// Calculate statistics
	var sum int64
	for _, s := range sorted {
		sum += s
	}
	avg := sum / int64(len(sorted))
	min := sorted[0]
	max := sorted[len(sorted)-1]
	p50 := sorted[len(sorted)*50/100]
	p95 := sorted[len(sorted)*95/100]
	p99 := sorted[len(sorted)*99/100]

	// Write header with statistics
	fmt.Fprintf(f, "# Noreza Latency Profile\n")
	fmt.Fprintf(f, "# Samples: %d\n", len(sorted))
	fmt.Fprintf(f, "# Min:     %s\n", formatDuration(min))
	fmt.Fprintf(f, "# Max:     %s\n", formatDuration(max))
	fmt.Fprintf(f, "# Avg:     %s\n", formatDuration(avg))
	fmt.Fprintf(f, "# P50:     %s\n", formatDuration(p50))
	fmt.Fprintf(f, "# P95:     %s\n", formatDuration(p95))
	fmt.Fprintf(f, "# P99:     %s\n", formatDuration(p99))
	fmt.Fprintf(f, "#\n")
	fmt.Fprintf(f, "# Raw samples (nanoseconds):\n")

	// Write all samples
	for _, s := range p.samples {
		fmt.Fprintf(f, "%d\n", s)
	}

	return nil
}

func formatDuration(ns int64) string {
	if ns < 1000 {
		return fmt.Sprintf("%dns", ns)
	} else if ns < 1000000 {
		return fmt.Sprintf("%.2fµs", float64(ns)/1000)
	} else {
		return fmt.Sprintf("%.2fms", float64(ns)/1000000)
	}
}

package cache

import (
	"fmt"
	"math/rand"
	"strings"
	"time"
)

// BenchResult holds the result of a single benchmark run.
type BenchResult struct {
	Pattern   string
	Ops       int
	Duration  time.Duration
	FinalP    int
	HitRate   float64
	Evictions int64
}

// RunBench runs all benchmark patterns and returns a comparison report.
func RunBench(capacity, ops int) []BenchResult {
	patterns := []struct {
		name string
		fn   func(*ARCCache, int)
	}{
		{"scan", benchScan},
		{"frequency", benchFrequency},
		{"mixed", benchMixed},
		{"random", benchRandom},
	}

	results := make([]BenchResult, 0, len(patterns))

	for _, p := range patterns {
		c := NewARCCache(capacity)
		start := time.Now()
		p.fn(c, ops)
		dur := time.Since(start)
		stats := c.Stats()
		c.Stop()

		results = append(results, BenchResult{
			Pattern:   p.name,
			Ops:       ops,
			Duration:  dur,
			FinalP:    stats.P,
			HitRate:   stats.HitRate(),
			Evictions: stats.Evictions,
		})
	}

	return results
}

// PrintBenchResults prints a formatted table of benchmark results.
func PrintBenchResults(capacity, ops int, results []BenchResult) {
	fmt.Printf("\n%s\n", strings.Repeat("─", 60))
	fmt.Printf("  ARC Benchmark  |  capacity=%d  ops=%d\n", capacity, ops)
	fmt.Printf("%s\n\n", strings.Repeat("─", 60))
	fmt.Printf("  %-12s  %8s  %8s  %8s  %10s\n",
		"Pattern", "Hit Rate", "Final p", "Evictions", "Duration")
	fmt.Printf("  %s\n", strings.Repeat("─", 56))
	for _, r := range results {
		fmt.Printf("  %-12s  %7.1f%%  %8d  %8d  %10s\n",
			r.Pattern, r.HitRate, r.FinalP, r.Evictions, r.Duration.Round(time.Microsecond))
	}
	fmt.Println()
}

// benchScan simulates a sequential scan pattern.
// LRU struggles here (thrashing). ARC adapts by growing T1.
func benchScan(c *ARCCache, ops int) {
	keyCount := c.capacity * 2
	for i := 0; i < ops; i++ {
		key := fmt.Sprintf("key-%d", i%keyCount)
		if _, ok := c.Get(key); !ok {
			c.Put(key, key, 0)
		}
	}
}

// benchFrequency simulates a hot-key frequency pattern.
// A small set of keys is accessed far more often than others.
func benchFrequency(c *ARCCache, ops int) {
	hotKeys := c.capacity / 4
	if hotKeys < 1 {
		hotKeys = 1
	}
	for i := 0; i < ops; i++ {
		var key string
		if rand.Float64() < 0.8 {
			key = fmt.Sprintf("hot-%d", i%hotKeys)
		} else {
			key = fmt.Sprintf("cold-%d", i%c.capacity)
		}
		if _, ok := c.Get(key); !ok {
			c.Put(key, key, 0)
		}
	}
}

// benchMixed alternates between scan and frequency phases.
// This is where ARC shines — p shifts as workload changes.
func benchMixed(c *ARCCache, ops int) {
	phase := ops / 4
	for i := 0; i < ops; i++ {
		var key string
		switch (i / phase) % 4 {
		case 0:
			key = fmt.Sprintf("scan-%d", i%c.capacity*2)
		case 1:
			key = fmt.Sprintf("hot-%d", i%(c.capacity/4+1))
		case 2:
			key = fmt.Sprintf("scan2-%d", i%(c.capacity*3))
		case 3:
			key = fmt.Sprintf("hot2-%d", i%(c.capacity/5+1))
		}
		if _, ok := c.Get(key); !ok {
			c.Put(key, key, 0)
		}
	}
}

// benchRandom simulates a uniform random access pattern.
func benchRandom(c *ARCCache, ops int) {
	keyCount := c.capacity * 3
	for i := 0; i < ops; i++ {
		key := fmt.Sprintf("key-%d", rand.Intn(keyCount))
		if _, ok := c.Get(key); !ok {
			c.Put(key, key, 0)
		}
	}
}

// VisualizePMovement shows how p shifts during a mixed workload as a bar chart.
func VisualizePMovement(capacity, ops int) {
	c := NewARCCache(capacity)
	defer c.Stop()

	phase := ops / 4
	snapshots := make([]int, 0, 40)
	labels := make([]string, 0, 40)

	interval := ops / 20
	if interval < 1 {
		interval = 1
	}

	for i := 0; i < ops; i++ {
		var key string
		switch (i / phase) % 4 {
		case 0:
			key = fmt.Sprintf("scan-%d", i%(capacity*2))
		case 1:
			key = fmt.Sprintf("hot-%d", i%(capacity/4+1))
		case 2:
			key = fmt.Sprintf("scan2-%d", i%(capacity*3))
		case 3:
			key = fmt.Sprintf("hot2-%d", i%(capacity/5+1))
		}
		if _, ok := c.Get(key); !ok {
			c.Put(key, key, 0)
		}

		if i%interval == 0 {
			stats := c.Stats()
			snapshots = append(snapshots, stats.P)
			labels = append(labels, []string{"scan", "freq", "scan", "freq"}[(i/phase)%4])
		}
	}

	barWidth := 30
	for i, p := range snapshots {
		filled := 0
		if capacity > 0 {
			filled = p * barWidth / capacity
		}
		bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)
		fmt.Printf("  [%4s] p=%-3d |%s|\n", labels[i], p, bar)
	}
}

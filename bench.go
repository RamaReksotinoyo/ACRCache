package main

import (
	"fmt"
	"math/rand"
	"strings"
	"time"
)

// benchResult holds the result of a single benchmark run.
type benchResult struct {
	pattern   string
	ops       int
	duration  time.Duration
	finalP    int
	hitRate   float64
	evictions int64
}

// runBench runs all benchmark patterns and prints a comparison report.
func runBench(capacity, ops int) {
	patterns := []struct {
		name string
		fn   func(*ARCCache, int)
	}{
		{"scan", benchScan},
		{"frequency", benchFrequency},
		{"mixed", benchMixed},
		{"random", benchRandom},
	}

	results := make([]benchResult, 0, len(patterns))

	fmt.Printf("\n%s\n", strings.Repeat("─", 60))
	fmt.Printf("  ARC Benchmark  |  capacity=%d  ops=%d\n", capacity, ops)
	fmt.Printf("%s\n\n", strings.Repeat("─", 60))

	for _, p := range patterns {
		cache := NewARCCache(capacity)
		start := time.Now()
		p.fn(cache, ops)
		dur := time.Since(start)
		stats := cache.Stats()

		results = append(results, benchResult{
			pattern:   p.name,
			ops:       ops,
			duration:  dur,
			finalP:    stats.P,
			hitRate:   stats.HitRate(),
			evictions: stats.Evictions,
		})
	}

	// Print results table
	fmt.Printf("  %-12s  %8s  %8s  %8s  %10s\n",
		"Pattern", "Hit Rate", "Final p", "Evictions", "Duration")
	fmt.Printf("  %s\n", strings.Repeat("─", 56))
	for _, r := range results {
		fmt.Printf("  %-12s  %7.1f%%  %8d  %8d  %10s\n",
			r.pattern, r.hitRate, r.finalP, r.evictions, r.duration.Round(time.Microsecond))
	}
	fmt.Println()

	// Visualize p movement for mixed pattern
	fmt.Println("  p movement over time (mixed pattern) — tracks ARC adapting:")
	visualizePMovement(capacity, ops/10)
	fmt.Println()
}

// benchScan simulates a sequential scan pattern.
// LRU struggles here (thrashing). ARC adapts by growing T1.
func benchScan(cache *ARCCache, ops int) {
	keyCount := cache.capacity * 2 // more keys than capacity -> thrashing
	for i := 0; i < ops; i++ {
		key := fmt.Sprintf("key-%d", i%keyCount)
		if _, ok := cache.Get(key); !ok {
			cache.Put(key, key)
		}
	}
}

// benchFrequency simulates a hot-key frequency pattern.
// A small set of keys is accessed far more often than others.
func benchFrequency(cache *ARCCache, ops int) {
	hotKeys := cache.capacity / 4
	if hotKeys < 1 {
		hotKeys = 1
	}
	for i := 0; i < ops; i++ {
		var key string
		if rand.Float64() < 0.8 {
			key = fmt.Sprintf("hot-%d", i%hotKeys)
		} else {
			key = fmt.Sprintf("cold-%d", i%cache.capacity)
		}
		if _, ok := cache.Get(key); !ok {
			cache.Put(key, key)
		}
	}
}

// benchMixed alternates between scan and frequency phases.
// This is where ARC shines — p shifts as workload changes.
func benchMixed(cache *ARCCache, ops int) {
	phase := ops / 4
	for i := 0; i < ops; i++ {
		var key string
		switch (i / phase) % 4 {
		case 0: // scan phase
			key = fmt.Sprintf("scan-%d", i%cache.capacity*2)
		case 1: // frequency phase
			key = fmt.Sprintf("hot-%d", i%(cache.capacity/4+1))
		case 2: // scan again
			key = fmt.Sprintf("scan2-%d", i%(cache.capacity*3))
		case 3: // frequency again
			key = fmt.Sprintf("hot2-%d", i%(cache.capacity/5+1))
		}
		if _, ok := cache.Get(key); !ok {
			cache.Put(key, key)
		}
	}
}

// benchRandom simulates a uniform random access pattern.
func benchRandom(cache *ARCCache, ops int) {
	keyCount := cache.capacity * 3
	for i := 0; i < ops; i++ {
		key := fmt.Sprintf("key-%d", rand.Intn(keyCount))
		if _, ok := cache.Get(key); !ok {
			cache.Put(key, key)
		}
	}
}

// visualizePMovement shows how p shifts during a mixed workload as a bar chart.
func visualizePMovement(capacity, ops int) {
	cache := NewARCCache(capacity)
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
		if _, ok := cache.Get(key); !ok {
			cache.Put(key, key)
		}

		if i%interval == 0 {
			stats := cache.Stats()
			snapshots = append(snapshots, stats.P)

			phaseLabel := []string{"scan", "freq", "scan", "freq"}[(i/phase)%4]
			labels = append(labels, phaseLabel)
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

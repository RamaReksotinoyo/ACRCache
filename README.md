# cache

A Go library implementing an [Adaptive Replacement Cache (ARC)](https://en.wikipedia.org/wiki/Adaptive_replacement_cache) with TTL support. Built to learn how ARC works and serve as an embeddable in-memory cache for small Go web applications.

## What is ARC?

ARC is a cache eviction algorithm that adapts between recency (LRU) and frequency (LFU) automatically. It maintains four internal lists:

- **T1** — pages seen exactly once recently
- **T2** — pages seen more than once (promoted from T1)
- **B1 / B2** — ghost entries (keys only, no values) of recently evicted T1/T2 items

A parameter `p` shifts dynamically based on ghost hits — if B1 gets hit, `p` grows (favor recency); if B2 gets hit, `p` shrinks (favor frequency). This is what makes ARC self-tuning.

## Installation

```bash
go get github.com/RamaReksotinoyo/ACRCache
```

## Usage

```go
import "github.com/RamaReksotinoyo/ACRCache"

// Create a cache with a capacity of 512 entries.
// A background goroutine runs every minute to evict expired entries.
c := cache.NewARCCache(512)
defer c.Stop() // release the background goroutine on shutdown

// Store with TTL.
c.Put("session:abc", sessionData, 30*time.Minute)

// Store without expiry.
c.Put("config", appConfig, 0)

// Retrieve — returns (nil, false) on miss or after expiry.
val, ok := c.Get("session:abc")
if ok {
    // cache hit
}

// Delete a specific key.
c.Delete("session:abc")

// Inspect live keys and statistics.
keys := c.Keys()
stats := c.Stats()
fmt.Printf("hit rate: %.1f%%\n", stats.HitRate())

// Clear all entries and reset statistics.
c.Flush()
```

### Web server example

```go
var c = cache.NewARCCache(512)

func init() {
    // Stop the janitor when the process exits.
    // In a real server, wire this into your graceful shutdown.
    runtime.SetFinalizer(c, func(c *cache.ARCCache) { c.Stop() })
}

func handleRequest(w http.ResponseWriter, r *http.Request) {
    key := r.URL.Path

    if val, ok := c.Get(key); ok {
        w.Write(val.([]byte))
        return
    }

    data := fetchFromDB(key)
    c.Put(key, data, 5*time.Minute)
    w.Write(data)
}
```

## API

```go
// NewARCCache creates a cache with the given capacity.
// Starts a background janitor that evicts expired entries every minute.
func NewARCCache(capacity int) *ARCCache

// Put inserts or updates a key-value pair.
// ttl is the time-to-live; pass 0 for no expiration.
func (c *ARCCache) Put(key string, value any, ttl time.Duration)

// Get returns the value and true on hit, or nil and false on miss or expiry.
// Expired entries are removed lazily on access.
func (c *ARCCache) Get(key string) (any, bool)

// Delete removes a key from the live cache.
// Returns true if the key existed.
func (c *ARCCache) Delete(key string) bool

// Flush removes all entries and resets all statistics.
func (c *ARCCache) Flush()

// Keys returns all live, non-expired keys in T1 and T2.
func (c *ARCCache) Keys() []string

// Stats returns a snapshot of hits, misses, evictions, and list sizes.
func (c *ARCCache) Stats() Stats

// Stop shuts down the background janitor goroutine.
// Safe to call multiple times.
func (c *ARCCache) Stop()
```

## Expiry

Expiry works in two layers:

1. **Lazy** — on every `Get`, the entry's deadline is checked. Expired entries are removed immediately and counted as a miss. No background goroutine is needed for correctness.
2. **Proactive** — a janitor goroutine runs every minute and scans T1/T2 to reclaim memory from entries that expired but were never accessed again.

Expired entries are not moved to the ghost lists (B1/B2) because they expired due to time, not memory pressure, so they carry no useful signal for ARC's `p` adaptation.

## Running Tests

```bash
go test ./... -v
go test -bench=. -benchmem
```

## Project Structure

```
arc.go        # ARCCache — T1/T2/B1/B2 lists, eviction, TTL, janitor
lru.go        # doubly linked list with per-node expiry used by all four lists
bench.go      # RunBench, PrintBenchResults, VisualizePMovement
arc_test.go   # unit tests (including TTL) and Go benchmarks
```

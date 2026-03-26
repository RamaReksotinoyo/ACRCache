package main

import "sync"

// Stats holds runtime statistics for the ARC cache.
type Stats struct {
	Hits     int64
	Misses   int64
	Evictions int64
	T1Size   int
	T2Size   int
	B1Size   int
	B2Size   int
	P        int // current target size for T1
}

// HitRate returns the cache hit rate as a percentage.
func (s Stats) HitRate() float64 {
	total := s.Hits + s.Misses
	if total == 0 {
		return 0
	}
	return float64(s.Hits) / float64(total) * 100
}

// ARCCache is an Adaptive Replacement Cache.
//
// It maintains four internal lists:
//   - T1: pages seen exactly once recently (recency)
//   - T2: pages seen at least twice recently (frequency)
//   - B1: ghost entries evicted from T1 (history)
//   - B2: ghost entries evicted from T2 (history)
//
// The parameter p is the adaptive target size for T1.
// When there is a ghost hit in B2, p grows (favor frequency).
// When there is a ghost hit in B1, p shrinks (favor recency).
type ARCCache struct {
	capacity int
	p        int // target size for T1

	t1 *lruList // recently seen once
	t2 *lruList // recently seen twice+
	b1 *lruList // ghost: evicted from T1
	b2 *lruList // ghost: evicted from T2

	hits      int64
	misses    int64
	evictions int64

	mu sync.RWMutex
}

// NewARCCache creates a new ARC cache with the given capacity.
func NewARCCache(capacity int) *ARCCache {
	if capacity < 1 {
		capacity = 1
	}
	return &ARCCache{
		capacity: capacity,
		p:        0,
		t1:       newLRUList(),
		t2:       newLRUList(),
		b1:       newLRUList(),
		b2:       newLRUList(),
	}
}

// Get retrieves a value from the cache.
// Returns the value and true if found, or nil and false if not.
func (c *ARCCache) Get(key string) (any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Cache hit in T1 -> promote to T2
	if node := c.t1.get(key); node != nil {
		c.t1.removeKey(key)
		c.t2.pushFront(key, node.value)
		c.hits++
		return node.value, true
	}

	// Cache hit in T2 -> move to front (most recently used)
	if node := c.t2.get(key); node != nil {
		c.t2.moveToFront(node)
		c.hits++
		return node.value, true
	}

	c.misses++
	return nil, false
}

// Put inserts or updates a key-value pair in the cache.
func (c *ARCCache) Put(key string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Case 1: key is in T1 or T2 -> update value, move to T2
	if node := c.t1.get(key); node != nil {
		c.t1.removeKey(key)
		c.t2.pushFront(key, value)
		return
	}
	if node := c.t2.get(key); node != nil {
		node.value = value
		c.t2.moveToFront(node)
		return
	}

	// Case 2: ghost hit in B1 -> adapt p upward (favor recency/T1)
	if c.b1.has(key) {
		delta := 1
		if c.b2.size > c.b1.size && c.b1.size > 0 {
			delta = c.b2.size / c.b1.size
		}
		c.p = min(c.p+delta, c.capacity)
		c.b1.removeKey(key)
		c.replace(key)
		c.t2.pushFront(key, value)
		return
	}

	// Case 3: ghost hit in B2 -> adapt p downward (favor frequency/T2)
	if c.b2.has(key) {
		delta := 1
		if c.b1.size > c.b2.size && c.b2.size > 0 {
			delta = c.b1.size / c.b2.size
		}
		c.p = max(c.p-delta, 0)
		c.b2.removeKey(key)
		c.replace(key)
		c.t2.pushFront(key, value)
		return
	}

	// Case 4: new key not seen before
	total := c.t1.size + c.b1.size
	if total == c.capacity {
		// T1 + B1 is full
		if c.t1.size < c.capacity {
			c.b1.removeLRU() // evict ghost from B1
			c.replace(key)
		} else {
			// T1 alone fills capacity, evict from T1 directly
			evicted := c.t1.removeLRU()
			if evicted != nil {
				c.evictions++
			}
		}
	} else if total < c.capacity {
		total2 := c.t1.size + c.t2.size + c.b1.size + c.b2.size
		if total2 >= c.capacity {
			if total2 == 2*c.capacity {
				c.b2.removeLRU() // evict ghost from B2
			}
			c.replace(key)
		}
	}

	c.t1.pushFront(key, value)
}

// Delete removes a key from the cache entirely.
// Returns true if the key existed.
func (c *ARCCache) Delete(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.t1.removeKey(key) != nil {
		return true
	}
	if c.t2.removeKey(key) != nil {
		return true
	}
	return false
}

// Flush removes all entries from the cache, resetting it to empty.
func (c *ARCCache) Flush() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.t1 = newLRUList()
	c.t2 = newLRUList()
	c.b1 = newLRUList()
	c.b2 = newLRUList()
	c.p = 0
	c.hits = 0
	c.misses = 0
	c.evictions = 0
}

// Stats returns a snapshot of the current cache statistics.
func (c *ARCCache) Stats() Stats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return Stats{
		Hits:      c.hits,
		Misses:    c.misses,
		Evictions: c.evictions,
		T1Size:    c.t1.size,
		T2Size:    c.t2.size,
		B1Size:    c.b1.size,
		B2Size:    c.b2.size,
		P:         c.p,
	}
}

// Keys returns all keys currently stored in T1 and T2 (the live cache).
func (c *ARCCache) Keys() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	t1Keys := c.t1.keys()
	t2Keys := c.t2.keys()
	return append(t1Keys, t2Keys...)
}

// replace is the ARC eviction subroutine.
// It decides whether to evict from T1 or T2 based on p,
// and moves the evicted entry to the appropriate ghost list.
func (c *ARCCache) replace(key string) {
	t1Bigger := c.t1.size > c.p
	b2HasKey := c.b2.has(key)

	if c.t1.size > 0 && (t1Bigger || (b2HasKey && c.t1.size == c.p)) {
		// Evict LRU from T1 -> move to B1 ghost
		evicted := c.t1.removeLRU()
		if evicted != nil {
			c.b1.pushFront(evicted.key, nil)
			c.evictions++
		}
	} else if c.t2.size > 0 {
		// Evict LRU from T2 -> move to B2 ghost
		evicted := c.t2.removeLRU()
		if evicted != nil {
			c.b2.pushFront(evicted.key, nil)
			c.evictions++
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

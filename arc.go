package cache

import (
	"sync"
	"time"
)

const janitorInterval = time.Minute

// Stats holds runtime statistics for the ARC cache.
type Stats struct {
	Hits      int64
	Misses    int64
	Evictions int64
	T1Size    int
	T2Size    int
	B1Size    int
	B2Size    int
	P         int // current target size for T1
}

// HitRate returns the cache hit rate as a percentage.
func (s Stats) HitRate() float64 {
	total := s.Hits + s.Misses
	if total == 0 {
		return 0
	}
	return float64(s.Hits) / float64(total) * 100
}

// ARCCache is a thread-safe Adaptive Replacement Cache with TTL support.
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
	p        int

	t1 *lruList
	t2 *lruList
	b1 *lruList
	b2 *lruList

	hits      int64
	misses    int64
	evictions int64

	mu   sync.RWMutex
	stop chan struct{}
	once sync.Once
}

// NewARCCache creates a new ARC cache with the given capacity.
// A background goroutine runs every minute to evict expired entries.
// Call Stop when the cache is no longer needed to release the goroutine.
func NewARCCache(capacity int) *ARCCache {
	if capacity < 1 {
		capacity = 1
	}
	c := &ARCCache{
		capacity: capacity,
		t1:       newLRUList(),
		t2:       newLRUList(),
		b1:       newLRUList(),
		b2:       newLRUList(),
		stop:     make(chan struct{}),
	}
	go c.janitor()
	return c
}

// Stop shuts down the background expiry goroutine.
// It is safe to call Stop multiple times.
func (c *ARCCache) Stop() {
	c.once.Do(func() { close(c.stop) })
}

func (c *ARCCache) janitor() {
	ticker := time.NewTicker(janitorInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			c.deleteExpired()
		case <-c.stop:
			return
		}
	}
}

// deleteExpired proactively removes expired entries from T1 and T2.
// Expired entries are simply dropped — they are not moved to the ghost lists
// because they expired due to time, not memory pressure, so they carry no
// useful signal for ARC's p adaptation.
func (c *ARCCache) deleteExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, key := range c.t1.keys() {
		if node := c.t1.get(key); node != nil && node.isExpired() {
			c.t1.removeKey(key)
		}
	}
	for _, key := range c.t2.keys() {
		if node := c.t2.get(key); node != nil && node.isExpired() {
			c.t2.removeKey(key)
		}
	}
}

// Get retrieves a value from the cache.
// Returns (value, true) on a hit, or (nil, false) on a miss or if the entry
// has expired. Expired entries are removed lazily on access.
func (c *ARCCache) Get(key string) (any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Cache hit in T1 — check TTL, then promote to T2.
	if node := c.t1.get(key); node != nil {
		if node.isExpired() {
			c.t1.removeKey(key)
			c.misses++
			return nil, false
		}
		c.t1.removeKey(key)
		c.t2.pushFront(key, node.value, node.expiresAt)
		c.hits++
		return node.value, true
	}

	// Cache hit in T2 — check TTL, then move to front (MRU position).
	if node := c.t2.get(key); node != nil {
		if node.isExpired() {
			c.t2.removeKey(key)
			c.misses++
			return nil, false
		}
		c.t2.moveToFront(node)
		c.hits++
		return node.value, true
	}

	c.misses++
	return nil, false
}

// Put inserts or updates a key-value pair in the cache.
// ttl is the time-to-live for the entry; use 0 for no expiration.
func (c *ARCCache) Put(key string, value any, ttl time.Duration) {
	var expiresAt time.Time
	if ttl > 0 {
		expiresAt = time.Now().Add(ttl)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Case 1: key is already live in T1 or T2 — update value and TTL, move to T2.
	if node := c.t1.get(key); node != nil {
		c.t1.removeKey(key)
		c.t2.pushFront(key, value, expiresAt)
		return
	}
	if node := c.t2.get(key); node != nil {
		node.value = value
		node.expiresAt = expiresAt
		c.t2.moveToFront(node)
		return
	}

	// Case 2: ghost hit in B1 — adapt p upward (favor recency).
	if c.b1.has(key) {
		delta := 1
		if c.b2.size > c.b1.size && c.b1.size > 0 {
			delta = c.b2.size / c.b1.size
		}
		c.p = min(c.p+delta, c.capacity)
		c.b1.removeKey(key)
		c.replace(key)
		c.t2.pushFront(key, value, expiresAt)
		return
	}

	// Case 3: ghost hit in B2 — adapt p downward (favor frequency).
	if c.b2.has(key) {
		delta := 1
		if c.b1.size > c.b2.size && c.b2.size > 0 {
			delta = c.b1.size / c.b2.size
		}
		c.p = max(c.p-delta, 0)
		c.b2.removeKey(key)
		c.replace(key)
		c.t2.pushFront(key, value, expiresAt)
		return
	}

	// Case 4: new key not seen before.
	total := c.t1.size + c.b1.size
	if total == c.capacity {
		// T1 + B1 is full.
		if c.t1.size < c.capacity {
			c.b1.removeLRU() // evict oldest ghost from B1
			c.replace(key)
		} else {
			// T1 alone fills capacity — evict from T1 directly.
			evicted := c.t1.removeLRU()
			if evicted != nil {
				c.evictions++
			}
		}
	} else if total < c.capacity {
		total2 := c.t1.size + c.t2.size + c.b1.size + c.b2.size
		if total2 >= c.capacity {
			if total2 == 2*c.capacity {
				c.b2.removeLRU() // evict oldest ghost from B2
			}
			c.replace(key)
		}
	}

	c.t1.pushFront(key, value, expiresAt)
}

// Delete removes a key from the cache.
// Returns true if the key existed in the live cache (T1 or T2).
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

// Flush removes all entries from the cache and resets all statistics.
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

// Keys returns all live, non-expired keys in T1 and T2.
func (c *ARCCache) Keys() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	now := time.Now()
	t1Keys := c.t1.keys()
	t2Keys := c.t2.keys()

	result := make([]string, 0, len(t1Keys)+len(t2Keys))
	for _, k := range t1Keys {
		if node := c.t1.get(k); node != nil && (node.expiresAt.IsZero() || now.Before(node.expiresAt)) {
			result = append(result, k)
		}
	}
	for _, k := range t2Keys {
		if node := c.t2.get(k); node != nil && (node.expiresAt.IsZero() || now.Before(node.expiresAt)) {
			result = append(result, k)
		}
	}
	return result
}

// replace is the ARC eviction subroutine.
// It decides whether to evict from T1 or T2 based on p,
// and moves the evicted entry into the appropriate ghost list.
func (c *ARCCache) replace(key string) {
	t1Bigger := c.t1.size > c.p
	b2HasKey := c.b2.has(key)

	if c.t1.size > 0 && (t1Bigger || (b2HasKey && c.t1.size == c.p)) {
		// Evict LRU from T1 — demote to B1 ghost.
		evicted := c.t1.removeLRU()
		if evicted != nil {
			c.b1.pushFront(evicted.key, nil, time.Time{})
			c.evictions++
		}
	} else if c.t2.size > 0 {
		// Evict LRU from T2 — demote to B2 ghost.
		evicted := c.t2.removeLRU()
		if evicted != nil {
			c.b2.pushFront(evicted.key, nil, time.Time{})
			c.evictions++
		}
	}
}

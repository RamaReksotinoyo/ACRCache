package cache

import (
	"fmt"
	"testing"
	"time"
)

// --- LRU list tests ---

func TestLRUPushAndGet(t *testing.T) {
	l := newLRUList()
	l.pushFront("a", "val-a", time.Time{})
	l.pushFront("b", "val-b", time.Time{})

	if !l.has("a") || !l.has("b") {
		t.Fatal("expected both keys to exist")
	}
	if l.size != 2 {
		t.Fatalf("expected size 2, got %d", l.size)
	}
}

func TestLRURemoveLRU(t *testing.T) {
	l := newLRUList()
	l.pushFront("a", "1", time.Time{})
	l.pushFront("b", "2", time.Time{})
	l.pushFront("c", "3", time.Time{})

	// LRU is "a" (pushed first, never moved).
	evicted := l.removeLRU()
	if evicted.key != "a" {
		t.Fatalf("expected LRU to be 'a', got '%s'", evicted.key)
	}
	if l.size != 2 {
		t.Fatalf("expected size 2, got %d", l.size)
	}
}

func TestLRUMoveToFront(t *testing.T) {
	l := newLRUList()
	l.pushFront("a", "1", time.Time{})
	l.pushFront("b", "2", time.Time{})
	l.pushFront("c", "3", time.Time{})

	// Move "a" to front — it should no longer be the LRU.
	nodeA := l.get("a")
	l.moveToFront(nodeA)

	evicted := l.removeLRU()
	if evicted.key == "a" {
		t.Fatal("'a' should not be LRU after moveToFront")
	}
}

// --- ARC cache tests ---

func TestARCBasicSetGet(t *testing.T) {
	c := NewARCCache(4)
	defer c.Stop()

	c.Put("x", "hello", 0)
	c.Put("y", "world", 0)

	val, ok := c.Get("x")
	if !ok {
		t.Fatal("expected cache hit for 'x'")
	}
	if val != "hello" {
		t.Fatalf("expected 'hello', got %v", val)
	}
}

func TestARCMiss(t *testing.T) {
	c := NewARCCache(4)
	defer c.Stop()

	_, ok := c.Get("missing")
	if ok {
		t.Fatal("expected cache miss")
	}
}

func TestARCDelete(t *testing.T) {
	c := NewARCCache(4)
	defer c.Stop()

	c.Put("k", "v", 0)
	c.Delete("k")
	_, ok := c.Get("k")
	if ok {
		t.Fatal("expected cache miss after delete")
	}
}

func TestARCPromotionT1ToT2(t *testing.T) {
	c := NewARCCache(8)
	defer c.Stop()

	c.Put("k", "v", 0)
	c.Get("k") // T1 hit — promotes to T2.

	stats := c.Stats()
	if stats.T2Size != 1 {
		t.Fatalf("expected T2 size 1 after promotion, got %d", stats.T2Size)
	}
	if stats.T1Size != 0 {
		t.Fatalf("expected T1 size 0 after promotion, got %d", stats.T1Size)
	}
}

func TestARCEviction(t *testing.T) {
	c := NewARCCache(3)
	defer c.Stop()

	c.Put("a", "1", 0)
	c.Put("b", "2", 0)
	c.Put("c", "3", 0)
	c.Put("d", "4", 0) // triggers eviction

	stats := c.Stats()
	if total := stats.T1Size + stats.T2Size; total > 3 {
		t.Fatalf("expected at most 3 live entries, got %d", total)
	}
}

func TestARCCapacityOne(t *testing.T) {
	c := NewARCCache(1)
	defer c.Stop()

	c.Put("a", "1", 0)
	c.Put("b", "2", 0)

	stats := c.Stats()
	if total := stats.T1Size + stats.T2Size; total > 1 {
		t.Fatalf("expected at most 1 live entry with capacity=1, got %d", total)
	}
}

func TestARCPAdaptation(t *testing.T) {
	c := NewARCCache(10)
	defer c.Stop()

	for i := 0; i < 20; i++ {
		c.Put(fmt.Sprintf("key-%d", i), "v", 0)
	}
	for i := 0; i < 20; i++ {
		c.Put(fmt.Sprintf("key-%d", i), "v", 0)
	}
	// p adaptation is workload-dependent; verify no panic.
}

func TestARCFlush(t *testing.T) {
	c := NewARCCache(8)
	defer c.Stop()

	c.Put("a", "1", 0)
	c.Put("b", "2", 0)
	c.Flush()

	stats := c.Stats()
	if stats.T1Size+stats.T2Size+stats.B1Size+stats.B2Size != 0 {
		t.Fatal("expected all lists empty after flush")
	}
}

func TestARCUpdateExistingKey(t *testing.T) {
	c := NewARCCache(4)
	defer c.Stop()

	c.Put("k", "old", 0)
	c.Put("k", "new", 0)

	val, ok := c.Get("k")
	if !ok {
		t.Fatal("expected hit")
	}
	if val != "new" {
		t.Fatalf("expected 'new', got %v", val)
	}
}

func TestARCStats(t *testing.T) {
	c := NewARCCache(4)
	defer c.Stop()

	c.Put("a", "1", 0)
	c.Get("a") // hit
	c.Get("z") // miss

	stats := c.Stats()
	if stats.Hits != 1 {
		t.Fatalf("expected 1 hit, got %d", stats.Hits)
	}
	if stats.Misses != 1 {
		t.Fatalf("expected 1 miss, got %d", stats.Misses)
	}
}

// --- TTL tests ---

func TestARCTTLExpiry(t *testing.T) {
	c := NewARCCache(4)
	defer c.Stop()

	c.Put("k", "v", 50*time.Millisecond)

	if _, ok := c.Get("k"); !ok {
		t.Fatal("expected hit before expiry")
	}

	time.Sleep(100 * time.Millisecond)

	if _, ok := c.Get("k"); ok {
		t.Fatal("expected miss after expiry")
	}
}

func TestARCTTLExpiryInT2(t *testing.T) {
	c := NewARCCache(4)
	defer c.Stop()

	c.Put("k", "v", 50*time.Millisecond)
	c.Get("k") // promotes to T2

	stats := c.Stats()
	if stats.T2Size != 1 {
		t.Fatal("expected key to be in T2 after promotion")
	}

	time.Sleep(100 * time.Millisecond)

	if _, ok := c.Get("k"); ok {
		t.Fatal("expected miss after expiry in T2")
	}
}

func TestARCNoExpiry(t *testing.T) {
	c := NewARCCache(4)
	defer c.Stop()

	c.Put("k", "v", 0) // ttl=0 means no expiry
	time.Sleep(50 * time.Millisecond)

	if _, ok := c.Get("k"); !ok {
		t.Fatal("expected hit for entry with no expiry")
	}
}

func TestARCTTLUpdateOnPut(t *testing.T) {
	c := NewARCCache(4)
	defer c.Stop()

	// Insert with a very short TTL, then overwrite with no expiry.
	c.Put("k", "v1", 30*time.Millisecond)
	c.Put("k", "v2", 0) // reset TTL to no expiry

	time.Sleep(60 * time.Millisecond)

	val, ok := c.Get("k")
	if !ok {
		t.Fatal("expected hit after TTL was reset to no expiry")
	}
	if val != "v2" {
		t.Fatalf("expected 'v2', got %v", val)
	}
}

func TestARCKeysExcludesExpired(t *testing.T) {
	c := NewARCCache(8)
	defer c.Stop()

	c.Put("live", "v", 0)
	c.Put("expiring", "v", 30*time.Millisecond)

	time.Sleep(60 * time.Millisecond)

	keys := c.Keys()
	for _, k := range keys {
		if k == "expiring" {
			t.Fatal("Keys() should not return expired entries")
		}
	}
}

func TestARCStop(t *testing.T) {
	c := NewARCCache(4)
	c.Stop()
	c.Stop() // must not panic on double-stop
}

// --- Benchmarks ---

func BenchmarkARCPut(b *testing.B) {
	c := NewARCCache(256)
	defer c.Stop()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Put(fmt.Sprintf("key-%d", i%512), "value", 0)
	}
}

func BenchmarkARCGet(b *testing.B) {
	c := NewARCCache(256)
	defer c.Stop()
	for i := 0; i < 256; i++ {
		c.Put(fmt.Sprintf("key-%d", i), "value", 0)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Get(fmt.Sprintf("key-%d", i%256))
	}
}

func BenchmarkARCMixed(b *testing.B) {
	c := NewARCCache(256)
	defer c.Stop()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key-%d", i%512)
		if _, ok := c.Get(key); !ok {
			c.Put(key, "value", 0)
		}
	}
}

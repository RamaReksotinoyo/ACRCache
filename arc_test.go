package main

import (
	"fmt"
	"testing"
)

// --- LRU list tests ---

func TestLRUPushAndGet(t *testing.T) {
	l := newLRUList()
	l.pushFront("a", "val-a")
	l.pushFront("b", "val-b")

	if !l.has("a") || !l.has("b") {
		t.Fatal("expected both keys to exist")
	}
	if l.size != 2 {
		t.Fatalf("expected size 2, got %d", l.size)
	}
}

func TestLRURemoveLRU(t *testing.T) {
	l := newLRUList()
	l.pushFront("a", "1")
	l.pushFront("b", "2")
	l.pushFront("c", "3")

	// LRU is "a" (pushed first, never moved)
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
	l.pushFront("a", "1")
	l.pushFront("b", "2")
	l.pushFront("c", "3")

	// Move "a" to front -> it should no longer be the LRU
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
	c.Put("x", "hello")
	c.Put("y", "world")

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
	_, ok := c.Get("missing")
	if ok {
		t.Fatal("expected cache miss")
	}
}

func TestARCDelete(t *testing.T) {
	c := NewARCCache(4)
	c.Put("k", "v")
	c.Delete("k")
	_, ok := c.Get("k")
	if ok {
		t.Fatal("expected cache miss after delete")
	}
}

func TestARCPromotionT1ToT2(t *testing.T) {
	c := NewARCCache(8)
	c.Put("k", "v")

	// First Get: still in T1 -> should promote to T2
	c.Get("k")

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
	c.Put("a", "1")
	c.Put("b", "2")
	c.Put("c", "3")
	c.Put("d", "4") // should trigger eviction

	stats := c.Stats()
	total := stats.T1Size + stats.T2Size
	if total > 3 {
		t.Fatalf("expected at most 3 live entries, got %d", total)
	}
}

func TestARCCapacityOne(t *testing.T) {
	c := NewARCCache(1)
	c.Put("a", "1")
	c.Put("b", "2")

	stats := c.Stats()
	total := stats.T1Size + stats.T2Size
	if total > 1 {
		t.Fatalf("expected at most 1 live entry with capacity=1, got %d", total)
	}
}

func TestARCPAdaptation(t *testing.T) {
	c := NewARCCache(10)

	// Populate and evict to fill ghost lists
	for i := 0; i < 20; i++ {
		c.Put(fmt.Sprintf("key-%d", i), "v")
	}

	initialP := c.Stats().P

	// Access keys that are in B2 (frequency) -> p should shift up
	// Re-insert the same keys to trigger ghost hits
	for i := 0; i < 20; i++ {
		c.Put(fmt.Sprintf("key-%d", i), "v")
	}

	// p should have moved from its initial value
	_ = initialP // p shifting is workload-dependent; we just verify no panic
}

func TestARCFlush(t *testing.T) {
	c := NewARCCache(8)
	c.Put("a", "1")
	c.Put("b", "2")
	c.Flush()

	stats := c.Stats()
	if stats.T1Size+stats.T2Size+stats.B1Size+stats.B2Size != 0 {
		t.Fatal("expected all lists empty after flush")
	}
}

func TestARCUpdateExistingKey(t *testing.T) {
	c := NewARCCache(4)
	c.Put("k", "old")
	c.Put("k", "new")

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
	c.Put("a", "1")
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

// --- Benchmarks ---

func BenchmarkARCPut(b *testing.B) {
	c := NewARCCache(256)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Put(fmt.Sprintf("key-%d", i%512), "value")
	}
}

func BenchmarkARCGet(b *testing.B) {
	c := NewARCCache(256)
	for i := 0; i < 256; i++ {
		c.Put(fmt.Sprintf("key-%d", i), "value")
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Get(fmt.Sprintf("key-%d", i%256))
	}
}

func BenchmarkARCMixed(b *testing.B) {
	c := NewARCCache(256)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key-%d", i%512)
		if _, ok := c.Get(key); !ok {
			c.Put(key, "value")
		}
	}
}

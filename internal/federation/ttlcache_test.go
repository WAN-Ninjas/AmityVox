package federation

import (
	"sync"
	"testing"
	"time"
)

func TestTTLCache_GetSet(t *testing.T) {
	c := NewTTLCache[string](time.Minute, 10)
	c.Set("key1", "value1")

	val, ok := c.Get("key1")
	if !ok || val != "value1" {
		t.Fatalf("expected value1, got %q (ok=%v)", val, ok)
	}
}

func TestTTLCache_Miss(t *testing.T) {
	c := NewTTLCache[int](time.Minute, 10)
	_, ok := c.Get("nonexistent")
	if ok {
		t.Fatal("expected miss for nonexistent key")
	}
}

func TestTTLCache_Expiry(t *testing.T) {
	c := NewTTLCache[string](10*time.Millisecond, 10)
	c.Set("key1", "value1")

	time.Sleep(20 * time.Millisecond)

	_, ok := c.Get("key1")
	if ok {
		t.Fatal("expected expired entry to be a miss")
	}
	if c.Len() != 0 {
		t.Fatalf("expected len 0 after expiry, got %d", c.Len())
	}
}

func TestTTLCache_Eviction(t *testing.T) {
	c := NewTTLCache[int](time.Minute, 3)

	c.Set("a", 1)
	time.Sleep(time.Millisecond) // ensure different expiry times
	c.Set("b", 2)
	time.Sleep(time.Millisecond)
	c.Set("c", 3)

	// At capacity — adding a new key should evict the oldest ("a").
	c.Set("d", 4)

	if c.Len() != 3 {
		t.Fatalf("expected len 3 after eviction, got %d", c.Len())
	}
	if _, ok := c.Get("a"); ok {
		t.Fatal("expected 'a' to be evicted")
	}
	if v, ok := c.Get("d"); !ok || v != 4 {
		t.Fatalf("expected 'd'=4, got %d (ok=%v)", v, ok)
	}
}

func TestTTLCache_EvictionUpdatesExisting(t *testing.T) {
	c := NewTTLCache[int](time.Minute, 2)
	c.Set("a", 1)
	c.Set("b", 2)

	// Updating an existing key should NOT trigger eviction.
	c.Set("a", 10)
	if c.Len() != 2 {
		t.Fatalf("expected len 2 after update, got %d", c.Len())
	}
	if v, _ := c.Get("a"); v != 10 {
		t.Fatalf("expected updated value 10, got %d", v)
	}
}

func TestTTLCache_Invalidate(t *testing.T) {
	c := NewTTLCache[string](time.Minute, 10)
	c.Set("key1", "value1")
	c.Invalidate("key1")

	_, ok := c.Get("key1")
	if ok {
		t.Fatal("expected miss after invalidation")
	}
}

func TestTTLCache_InvalidateAll(t *testing.T) {
	c := NewTTLCache[int](time.Minute, 10)
	c.Set("a", 1)
	c.Set("b", 2)
	c.Set("c", 3)

	c.InvalidateAll()

	if c.Len() != 0 {
		t.Fatalf("expected len 0 after InvalidateAll, got %d", c.Len())
	}
}

func TestTTLCache_Len(t *testing.T) {
	c := NewTTLCache[string](time.Minute, 10)
	if c.Len() != 0 {
		t.Fatalf("expected len 0, got %d", c.Len())
	}
	c.Set("a", "1")
	c.Set("b", "2")
	if c.Len() != 2 {
		t.Fatalf("expected len 2, got %d", c.Len())
	}
}

func TestTTLCache_Concurrent(t *testing.T) {
	c := NewTTLCache[int](time.Minute, 100)
	var wg sync.WaitGroup

	// 100 goroutines writing and reading concurrently.
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := string(rune('A' + n%26))
			c.Set(key, n)
			c.Get(key)
			c.Invalidate(key)
		}(i)
	}
	wg.Wait()
	// No race detector errors = pass.
}

func TestTTLCache_BoolType(t *testing.T) {
	// Ensure the cache works correctly with bool values (allowedCache pattern).
	c := NewTTLCache[bool](time.Minute, 10)
	c.Set("peer1", true)
	c.Set("peer2", false)

	v1, ok1 := c.Get("peer1")
	v2, ok2 := c.Get("peer2")

	if !ok1 || !v1 {
		t.Fatalf("expected peer1=true, got %v (ok=%v)", v1, ok1)
	}
	if !ok2 || v2 {
		t.Fatalf("expected peer2=false, got %v (ok=%v)", v2, ok2)
	}
}

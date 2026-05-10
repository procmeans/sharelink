package cache

import (
	"testing"
	"time"
)

func TestCache_GetMiss(t *testing.T) {
	c := New(10, time.Minute)
	if _, ok := c.Get("nope"); ok {
		t.Fatal("expected miss")
	}
}

func TestCache_SetAndGet(t *testing.T) {
	c := New(10, time.Minute)
	c.Set("k", "v")
	got, ok := c.Get("k")
	if !ok || got != "v" {
		t.Fatalf("want v/true, got %q/%v", got, ok)
	}
}

func TestCache_TTLExpiry(t *testing.T) {
	c := New(10, 20*time.Millisecond)
	c.Set("k", "v")
	time.Sleep(40 * time.Millisecond)
	if _, ok := c.Get("k"); ok {
		t.Fatal("expected expired miss")
	}
}

func TestCache_OverwriteSameKey(t *testing.T) {
	c := New(10, time.Minute)
	c.Set("k", "v1")
	c.Set("k", "v2")
	got, _ := c.Get("k")
	if got != "v2" {
		t.Fatalf("want v2, got %q", got)
	}
	if c.Len() != 1 {
		t.Fatalf("want len 1, got %d", c.Len())
	}
}

func TestCache_LRUEviction(t *testing.T) {
	c := New(3, time.Minute)
	c.Set("a", "1")
	c.Set("b", "2")
	c.Set("c", "3")
	// "a" is the oldest. Touching "a" via Get should make "b" the oldest.
	if _, ok := c.Get("a"); !ok {
		t.Fatal("a should be present")
	}
	c.Set("d", "4") // evicts the LRU
	if _, ok := c.Get("b"); ok {
		t.Fatal("b should have been evicted (was LRU)")
	}
	for _, k := range []string{"a", "c", "d"} {
		if _, ok := c.Get(k); !ok {
			t.Fatalf("%s should still be present", k)
		}
	}
	if c.Len() != 3 {
		t.Fatalf("want len 3, got %d", c.Len())
	}
}

func TestCache_OverwritePromotes(t *testing.T) {
	c := New(2, time.Minute)
	c.Set("a", "1")
	c.Set("b", "2")
	c.Set("a", "1b") // should promote a, not evict
	c.Set("c", "3")  // should evict b
	if _, ok := c.Get("b"); ok {
		t.Fatal("b should have been evicted")
	}
	if got, _ := c.Get("a"); got != "1b" {
		t.Fatalf("want a=1b, got %q", got)
	}
}

func TestCache_ZeroCapacity(t *testing.T) {
	c := New(0, time.Minute)
	c.Set("a", "1")
	if _, ok := c.Get("a"); ok {
		t.Fatal("zero-capacity cache must hold nothing")
	}
}

func TestCache_ConcurrentReadsAndWrites(t *testing.T) {
	c := New(100, time.Minute)
	const goroutines = 16
	const iterations = 1000

	done := make(chan struct{}, goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer func() { done <- struct{}{} }()
			for i := 0; i < iterations; i++ {
				key := string(rune('a' + (i+id)%26))
				if i%2 == 0 {
					c.Set(key, key)
				} else {
					_, _ = c.Get(key)
				}
			}
		}(g)
	}
	for g := 0; g < goroutines; g++ {
		<-done
	}
}

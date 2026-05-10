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

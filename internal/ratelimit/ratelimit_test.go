package ratelimit

import (
	"testing"
	"time"
)

func TestNoop_AlwaysAllows(t *testing.T) {
	var lim Noop
	for i := 0; i < 100; i++ {
		allowed, retry := lim.Allow("any")
		if !allowed || retry != 0 {
			t.Errorf("Noop.Allow: want allowed=true retry=0, got allowed=%v retry=%d", allowed, retry)
		}
	}
}

func TestInMemory_AllowsWithinLimit(t *testing.T) {
	lim := NewInMemory(3, time.Minute)
	key := "client1"
	for i := 0; i < 3; i++ {
		allowed, retry := lim.Allow(key)
		if !allowed {
			t.Errorf("request %d: expected allowed", i+1)
		}
		if retry != 0 {
			t.Errorf("request %d: expected retry 0, got %d", i+1, retry)
		}
	}
}

func TestInMemory_RejectsOverLimit(t *testing.T) {
	lim := NewInMemory(2, time.Minute)
	key := "client1"
	lim.Allow(key)
	lim.Allow(key)
	allowed, retryAfter := lim.Allow(key)
	if allowed {
		t.Error("expected not allowed after limit exceeded")
	}
	if retryAfter <= 0 {
		t.Errorf("expected positive Retry-After, got %d", retryAfter)
	}
}

func TestInMemory_DifferentKeysIndependent(t *testing.T) {
	lim := NewInMemory(1, time.Minute)
	lim.Allow("a")
	allowedB, _ := lim.Allow("b")
	if !allowedB {
		t.Error("different key should be allowed")
	}
	allowedA, _ := lim.Allow("a")
	if allowedA {
		t.Error("same key over limit should be rejected")
	}
}

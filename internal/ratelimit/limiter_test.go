package ratelimit

import (
	"testing"
	"time"
)

func TestLimiter_Burst(t *testing.T) {
	l := New(100*time.Millisecond, 3)
	start := time.Now()
	for i := 0; i < 3; i++ {
		l.Wait("example.com")
	}
	if time.Since(start) > 50*time.Millisecond {
		t.Error("burst should not block for the first 3 requests")
	}
}

func TestLimiter_TracksDomains(t *testing.T) {
	l := New(10*time.Millisecond, 1)
	l.Wait("a.com")
	l.Wait("b.com")
	if l.Stats() != 2 {
		t.Errorf("expected 2 domains tracked, got %d", l.Stats())
	}
}

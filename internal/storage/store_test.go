package storage

import (
	"sync"
	"testing"
	"time"
)

func TestStore_MarkVisited(t *testing.T) {
	s := New()
	if !s.MarkVisited("https://example.com") {
		t.Error("first visit should return true")
	}
	if s.MarkVisited("https://example.com") {
		t.Error("second visit should return false")
	}
}

func TestStore_SaveAndAll(t *testing.T) {
	s := New()
	s.Save(&Result{URL: "https://a.com", WordCount: 100, StatusCode: 200})
	s.Save(&Result{URL: "https://b.com", WordCount: 200, StatusCode: 200})
	all := s.All()
	if len(all) != 2 {
		t.Errorf("expected 2 results, got %d", len(all))
	}
}

func TestStore_Stats(t *testing.T) {
	s := New()
	s.Save(&Result{URL: "https://a.com", WordCount: 100, StatusCode: 200})
	s.Save(&Result{URL: "https://b.com", WordCount: 300, StatusCode: 404, Error: "HTTP 404"})
	total, errs, avg := s.Stats()
	if total != 2 {
		t.Errorf("expected total=2, got %d", total)
	}
	if errs != 1 {
		t.Errorf("expected errs=1, got %d", errs)
	}
	if avg != 200 {
		t.Errorf("expected avgWords=200, got %d", avg)
	}
}

func TestStore_ConcurrentAccess(t *testing.T) {
	s := New()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			url := "https://example.com/page-" + string(rune('0'+n%10))
			s.MarkVisited(url)
			s.Save(&Result{URL: url, CrawledAt: time.Now()})
		}(i)
	}
	wg.Wait()
	// No race condition = success (run with -race to verify)
	if s.VisitedCount() == 0 {
		t.Error("expected some visited URLs")
	}
}

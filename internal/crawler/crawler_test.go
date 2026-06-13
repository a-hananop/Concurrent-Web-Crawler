package crawler_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	"webcrawler/internal/crawler"
)

// buildTestSite creates a small in-process site:
//   /        → links to /a and /b
//   /a        → links to /c
//   /b        → 404
//   /c        → no further links
func buildTestSite(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprintf(w, `<html><head><title>Home</title></head><body>
			<a href="/a">A</a> <a href="/b">B</a>
		</body></html>`)
	})
	mux.HandleFunc("/a", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `<html><head><title>Page A</title></head><body>
			<a href="/c">C</a>
		</body></html>`)
	})
	mux.HandleFunc("/c", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `<html><head><title>Page C</title></head><body>No links here.</body></html>`)
	})
	return httptest.NewServer(mux)
}

func TestCrawler_Integration(t *testing.T) {
	srv := buildTestSite(t)
	defer srv.Close()

	cfg := crawler.DefaultConfig()
	cfg.Workers        = 2
	cfg.MaxDepth       = 3
	cfg.MaxPages       = 20
	cfg.RatePerDomain  = 10 * time.Millisecond
	cfg.BurstPerDomain = 5
	cfg.RequestTimeout = 5 * time.Second
	cfg.SameDomain     = true

	c := crawler.New(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go c.Run(ctx, []string{srv.URL + "/"})

	// Poll until all expected pages are crawled or timeout.
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		total, _, _ := c.Store().Stats()
		if total >= 3 { // /, /a, /c (and /b as a 404)
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	cancel()

	results := c.Store().All()
	urlSet := make(map[string]bool)
	for _, r := range results {
		urlSet[r.URL] = true
	}

	for _, want := range []string{srv.URL + "/", srv.URL + "/a", srv.URL + "/c"} {
		if !urlSet[want] {
			t.Errorf("expected %s to be crawled", want)
		}
	}

	// /b should be recorded as an error (404)
	for _, r := range results {
		if r.URL == srv.URL+"/b" && r.Error == "" {
			t.Error("/b should have an error (404)")
		}
	}
}

func TestCrawler_MaxPages(t *testing.T) {
	srv := buildTestSite(t)
	defer srv.Close()

	cfg := crawler.DefaultConfig()
	cfg.Workers        = 2
	cfg.MaxDepth       = 5
	cfg.MaxPages       = 2 // hard cap
	cfg.RatePerDomain  = 10 * time.Millisecond
	cfg.BurstPerDomain = 5
	cfg.SameDomain     = true

	c := crawler.New(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go c.Run(ctx, []string{srv.URL + "/"})
	time.Sleep(2 * time.Second)
	cancel()

	total, _, _ := c.Store().Stats()
	if total > 2 {
		t.Errorf("MaxPages=2 but crawled %d pages", total)
	}
}

func TestCrawler_ContextCancel(t *testing.T) {
	srv := buildTestSite(t)
	defer srv.Close()

	cfg := crawler.DefaultConfig()
	cfg.Workers        = 3
	cfg.RatePerDomain  = 1 * time.Second // slow
	cfg.BurstPerDomain = 1
	cfg.SameDomain     = true

	c := crawler.New(cfg)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		c.Run(ctx, []string{srv.URL + "/"})
		close(done)
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Error("crawler did not stop within 3s after context cancel")
	}
}

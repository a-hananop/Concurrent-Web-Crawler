// Package crawler implements a concurrent BFS web crawler.
package crawler

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"webcrawler/internal/parser"
	"webcrawler/internal/ratelimit"
	"webcrawler/internal/storage"
)

// Config holds crawler tuning parameters.
type Config struct {
	// Workers is the number of concurrent goroutines fetching pages.
	Workers int
	// MaxDepth limits how many link-hops from the seed URLs to follow.
	MaxDepth int
	// MaxPages caps the total number of pages crawled (0 = unlimited).
	MaxPages int
	// RatePerDomain is the minimum wait between requests to the same domain.
	RatePerDomain time.Duration
	// BurstPerDomain is the initial token burst for the rate limiter.
	BurstPerDomain int
	// RequestTimeout is the per-request HTTP timeout.
	RequestTimeout time.Duration
	// SameDomain restricts crawling to the seed URL's domain.
	SameDomain bool
	// UserAgent is sent as the HTTP User-Agent header.
	UserAgent string
}

// DefaultConfig returns sensible defaults for a polite crawler.
func DefaultConfig() Config {
	return Config{
		Workers:        5,
		MaxDepth:       3,
		MaxPages:       50,
		RatePerDomain:  500 * time.Millisecond,
		BurstPerDomain: 3,
		RequestTimeout: 10 * time.Second,
		SameDomain:     true,
		UserAgent:      "GoCrawler/1.0 (educational; +https://github.com/example/webcrawler)",
	}
}

// work is a unit of crawl work.
type work struct {
	url   string
	depth int
}

// Crawler orchestrates concurrent crawling.
type Crawler struct {
	cfg     Config
	store   *storage.Store
	limiter *ratelimit.Limiter
	client  *http.Client

	queue    chan work
	wg       sync.WaitGroup
	once     sync.Once
	pagesMu  sync.Mutex
	pagesSeen int

	// OnResult is called after each page is processed (optional).
	OnResult func(*storage.Result)
}

// New creates a Crawler with the given configuration.
func New(cfg Config) *Crawler {
	return &Crawler{
		cfg:     cfg,
		store:   storage.New(),
		limiter: ratelimit.New(cfg.RatePerDomain, cfg.BurstPerDomain),
		client: &http.Client{
			Timeout: cfg.RequestTimeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return fmt.Errorf("stopped after 5 redirects")
				}
				return nil
			},
		},
		queue: make(chan work, 1024),
	}
}

// Run starts the crawl from the given seed URLs and blocks until done.
// It respects ctx for cancellation.
func (c *Crawler) Run(ctx context.Context, seeds []string) {
	// Launch worker pool.
	for i := 0; i < c.cfg.Workers; i++ {
		c.wg.Add(1)
		go c.worker(ctx)
	}

	// Enqueue seeds.
	for _, seed := range seeds {
		if c.store.MarkVisited(seed) {
			select {
			case c.queue <- work{url: seed, depth: 0}:
			case <-ctx.Done():
			}
		}
	}

	// Wait for all workers to drain and close the queue.
	go func() {
		c.wg.Wait()
		c.once.Do(func() { close(c.queue) })
	}()

	// Block until context cancelled or queue closed.
	<-ctx.Done()
}

// Store returns the underlying result store.
func (c *Crawler) Store() *storage.Store { return c.store }

// worker processes items from the queue until it's closed or ctx is done.
func (c *Crawler) worker(ctx context.Context) {
	defer c.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case w, ok := <-c.queue:
			if !ok {
				return
			}
			c.process(ctx, w)
		}
	}
}

// process fetches and parses a single page, then enqueues its links.
func (c *Crawler) process(ctx context.Context, w work) {
	// Respect max pages limit.
	if c.cfg.MaxPages > 0 {
		c.pagesMu.Lock()
		if c.pagesSeen >= c.cfg.MaxPages {
			c.pagesMu.Unlock()
			return
		}
		c.pagesSeen++
		c.pagesMu.Unlock()
	}

	domain := extractDomain(w.url)
	c.limiter.Wait(domain)

	start := time.Now()
	result := &storage.Result{
		URL:       w.url,
		Depth:     w.depth,
		CrawledAt: start,
	}

	req, err := http.NewRequestWithContext(ctx, "GET", w.url, nil)
	if err != nil {
		result.Error = err.Error()
		c.finish(result)
		return
	}
	req.Header.Set("User-Agent", c.cfg.UserAgent)

	resp, err := c.client.Do(req)
	if err != nil {
		result.Error = err.Error()
		result.Duration = time.Since(start)
		c.finish(result)
		return
	}
	defer resp.Body.Close()

	result.StatusCode = resp.StatusCode
	result.Duration = time.Since(start)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		result.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
		c.finish(result)
		return
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		result.Error = "non-HTML content"
		c.finish(result)
		return
	}

	base, _ := url.Parse(w.url)
	// Limit body read to 5 MB.
	body := io.LimitReader(resp.Body, 5*1024*1024)

	page, err := parser.Extract(body, base)
	if err != nil {
		result.Error = "parse error: " + err.Error()
		c.finish(result)
		return
	}

	result.Title = page.Title
	result.MetaDesc = page.MetaDesc
	result.WordCount = page.WordCount
	result.Links = page.Links

	c.finish(result)

	// Enqueue child links.
	if w.depth < c.cfg.MaxDepth {
		for _, link := range page.Links {
			if c.cfg.SameDomain && extractDomain(link) != domain {
				continue
			}
			if !c.store.MarkVisited(link) {
				continue
			}
			select {
			case c.queue <- work{url: link, depth: w.depth + 1}:
			case <-ctx.Done():
				return
			default:
				// Queue full – skip.
				log.Printf("queue full, dropping %s", link)
			}
		}
	}
}

func (c *Crawler) finish(r *storage.Result) {
	c.store.Save(r)
	if c.OnResult != nil {
		c.OnResult(r)
	}
}

func extractDomain(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	return u.Host
}

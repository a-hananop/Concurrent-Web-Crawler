package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
	"webcrawler/internal/crawler"
	"webcrawler/internal/storage"
)

// ── Shared server state ───────────────────────────────────────────────────────

type CrawlState struct {
	mu        sync.RWMutex
	running   bool
	startTime time.Time
	cancel    context.CancelFunc
	crawler   *crawler.Crawler
	events    []storage.Result // live feed buffer
}

var state = &CrawlState{}

// ── API handlers ──────────────────────────────────────────────────────────────

// POST /api/start  { url, workers, depth, pages, rate_ms, same_domain }
func handleStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var req struct {
		URL        string `json:"url"`
		Workers    int    `json:"workers"`
		Depth      int    `json:"depth"`
		Pages      int    `json:"pages"`
		RateMs     int    `json:"rate_ms"`
		SameDomain bool   `json:"same_domain"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request: "+err.Error(), 400)
		return
	}

	state.mu.Lock()
	if state.running {
		state.mu.Unlock()
		http.Error(w, "crawl already running", 409)
		return
	}

	cfg := crawler.DefaultConfig()
	cfg.Workers = orDefault(req.Workers, 5)
	cfg.MaxDepth = orDefault(req.Depth, 3)
	cfg.MaxPages = orDefault(req.Pages, 50)
	cfg.RatePerDomain = time.Duration(orDefault(req.RateMs, 500)) * time.Millisecond
	cfg.BurstPerDomain = 3
	cfg.RequestTimeout = 10 * time.Second
	cfg.SameDomain = req.SameDomain

	c := crawler.New(cfg)
	c.OnResult = func(res *storage.Result) {
		state.mu.Lock()
		state.events = append(state.events, *res)
		state.mu.Unlock()
	}

	ctx, cancel := context.WithCancel(context.Background())
	state.running = true
	state.startTime = time.Now()
	state.cancel = cancel
	state.crawler = c
	state.events = nil
	state.mu.Unlock()

	go func() {
		c.Run(ctx, []string{req.URL})
		state.mu.Lock()
		state.running = false
		state.mu.Unlock()
	}()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "started"})
}

// POST /api/stop
func handleStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	state.mu.Lock()
	if state.cancel != nil {
		state.cancel()
	}
	state.running = false
	state.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "stopped"})
}

// GET /api/status  — metrics + live event feed
func handleStatus(w http.ResponseWriter, r *http.Request) {
	state.mu.RLock()
	running := state.running
	startTime := state.startTime
	events := append([]storage.Result(nil), state.events...)
	var c *crawler.Crawler
	if state.crawler != nil {
		c = state.crawler
	}
	state.mu.RUnlock()

	elapsed := 0.0
	if !startTime.IsZero() {
		elapsed = time.Since(startTime).Seconds()
	}

	total, errCount, avgWords := 0, 0, 0
	if c != nil {
		total, errCount, avgWords = c.Store().Stats()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"running":   running,
		"elapsed":   fmt.Sprintf("%.1f", elapsed),
		"total":     total,
		"errors":    errCount,
		"avg_words": avgWords,
		"events":    events,
	})
}

// GET /api/results  — full result list when done
func handleResults(w http.ResponseWriter, r *http.Request) {
	state.mu.RLock()
	c := state.crawler
	state.mu.RUnlock()

	if c == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]any{})
		return
	}
	results := c.Store().All()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

// ── Main ──────────────────────────────────────────────────────────────────────

func main() {
	port := flag.String("port", "8080", "HTTP server port")
	flag.Parse()

	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("/api/start", cors(handleStart))
	mux.HandleFunc("/api/stop", cors(handleStop))
	mux.HandleFunc("/api/status", cors(handleStatus))
	mux.HandleFunc("/api/results", cors(handleResults))

	// Serve dashboard.html at /
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "dashboard.html")
	})

	srv := &http.Server{Addr: ":" + *port, Handler: mux}

	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
		<-ch
		fmt.Println("\nShutting down...")
		if state.cancel != nil {
			state.cancel()
		}
		srv.Close()
	}()

	fmt.Printf("🕷  Crawler server running at http://localhost:%s\n", *port)
	fmt.Printf("   Open dashboard.html in your browser, or visit http://localhost:%s\n\n", *port)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func orDefault(v, def int) int {
	if v <= 0 {
		return def
	}
	return v
}

func cors(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(204)
			return
		}
		h(w, r)
	}
}

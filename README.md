# GoSpider - Concurrent Web Crawler

GoSpider is a production-quality, concurrent web crawler and server written in Go. It features a robust goroutine worker pool, an interactive web dashboard for real-time monitoring, and polite token-bucket rate limiting.

## Features

- **Interactive Web Dashboard** — real-time visualization of crawl depth, top domains, and live request logs
- **Goroutine Worker Pool** — configurable parallelism with clean shutdown and context propagation
- **Per-domain Token-Bucket Rate Limiter** — polite, server-friendly crawling that avoids overwhelming hosts
- **BFS Link Traversal** — breadth-first search with configurable maximum depth and page limits
- **Deduplication** — atomic test-and-set prevents double-crawling of URLs
- **HTML Parsing** — extracts title, meta description, word counts, and links seamlessly
- **Race-Detector Clean** — all shared state is properly synchronized via mutexes and channels

---

## Project Structure

```text
webcrawler/
├── cmd/
│   └── crawler/
│       └── main.go              # Server entry point, API routes, and shutdown handling
├── internal/
│   ├── crawler/
│   │   ├── crawler.go           # Worker pool, BFS queue, orchestration
│   │   └── crawler_test.go      # Integration tests
│   ├── parser/
│   │   ├── parser.go            # HTML parsing, link extraction
│   │   └── parser_test.go       # Unit tests for link resolution
│   ├── ratelimit/
│   │   ├── limiter.go           # Per-domain token-bucket limiter
│   │   └── limiter_test.go      # Burst and multi-domain tests
│   └── storage/
│       ├── store.go             # Thread-safe result store + dedup set
│       └── store_test.go        # Concurrent-access, stats tests
├── dashboard.html               # Frontend UI dashboard
├── go.mod                       # Go module dependencies
└── README.md                    # Project documentation
```

---

## Quick Start

### Build

Compile the server executable:

```bash
go build -o gospider ./cmd/crawler/
```

### Run

Start the GoSpider server (default port is `8080`):

```bash
./gospider
```

You can optionally specify a custom port:

```bash
./gospider -port 9090
```

### Dashboard & API

Once the server is running, open your web browser and navigate to:

**http://localhost:8080**

From the dashboard, you can configure and launch a new crawl, monitor real-time logs, view depth distributions, and see top domains.

The server also exposes a RESTful API:
- `POST /api/start` — Start a new crawl (accepts JSON config: `url`, `workers`, `depth`, `pages`, `rate_ms`, `same_domain`)
- `POST /api/stop` — Gracefully stop the current crawl
- `GET /api/status` — Get live crawl metrics and real-time event feed
- `GET /api/results` — Fetch the final result list

---

### Test

Run tests across all packages with the race detector enabled:

```bash
go test ./... -race
```

For verbose output and a longer timeout:

```bash
go test ./... -v -race -timeout 60s
```

---

## Architecture

```text
Dashboard (UI) -> POST /api/start
    │
    ▼
main goroutine (Orchestrator)
    │
    ▼
Work Queue (chan work, cap 1024)
    │  fan-out
    ├──► Worker 1 ──► Rate Limiter ──► HTTP Fetch ──► HTML Parser ──► Store
    ├──► Worker 2 ──► Rate Limiter ──► HTTP Fetch ──► HTML Parser ──► Store
    ├──► Worker 3 ──► Rate Limiter ──► HTTP Fetch ──► HTML Parser ──► Store
    └──► Worker N      └─ per-domain bucket           └─ re-enqueue child links
```

### Key Design Decisions

**Buffered channel as work queue**  
`chan work` with capacity 1024 decouples producers (link discovery) from consumers (workers). A non-blocking `select default` drops links when the buffer is full rather than deadlocking — important for very broad crawls.

**Token-bucket rate limiter**  
Each domain maintains an independent bucket. Tokens refill proportionally to elapsed time. Workers call `limiter.Wait(domain)` before every fetch — this single line prevents request storms on any server while allowing full parallelism across different domains.

**Atomic deduplication**  
`store.MarkVisited(url)` acquires a write lock, checks, and sets in one critical section. This ensures that even if 10 workers discover the same link simultaneously, only one fetch is ever issued.

**Context propagation**  
Every goroutine receives a `context.Context`. HTTP requests use `http.NewRequestWithContext`, so an OS interrupt or API `stop` request immediately cancels all in-flight network I/O. No goroutine leaks.

**Clean Server Shutdown**  
The HTTP server listens for `SIGINT`/`SIGTERM` to gracefully cancel the crawler context and shut down the HTTP listener.

---

## Extending the Crawler

**Add a robots.txt parser**
```go
// Check before fetching in worker
if !robotsAllow(domain, path) {
    return
}
```

**Export results to CSV/JSON file**
```go
// Handle writing the `store.All()` array to a file on completion
enc := json.NewEncoder(os.Stdout)
for _, r := range store.All() {
    enc.Encode(r)
}
```

**Plug in a persistent store (e.g., SQLite/PostgreSQL)**  
Implement the same `Save` / `All` / `MarkVisited` interface against a database instead of the default in-memory map.

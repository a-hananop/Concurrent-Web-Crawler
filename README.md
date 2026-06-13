# GoSpider - Concurrent Web Crawler

GoSpider is a production-quality, concurrent web crawler and server written in Go. It features a robust goroutine worker pool, an interactive web dashboard for real-time monitoring, and polite token-bucket rate limiting.

## Features

- **Interactive Web Dashboard** — real-time visualization of crawl depth, top domains, and live request logs
- **Fully Responsive UI** — dashboard gracefully adapts to mobile, tablet, and desktop screen sizes
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

## Dashboard Walkthrough

The GoSpider dashboard provides a rich, real-time interface for controlling and monitoring crawls. Here is a breakdown of its capabilities:

### 1. Starting the Crawler Server
Launch the server via the terminal using `go run ./cmd/crawler/`. The terminal output confirms the server is listening on `http://localhost:8080`, ready for browser connections.

### 2. Initial Dashboard State
Upon opening the dashboard, it displays in an idle state. You can configure parameters such as the seed URL, number of workers, max depth, max pages, and the rate limit. The status indicator confirms it is "Connected — idle".

### 3. Running a Shallow Crawl
Initiating a shallow crawl (e.g., max depth of 1) immediately populates the "Live Results" panel with real-time fetch times and HTTP 200 statuses. The "Pages by Depth" chart visualizes that pages are only discovered at the root and first level.

### 4. Running a Deeper Crawl
By increasing the max depth (e.g., depth 3) and page limits, the crawler branches out further. The live results continue to stream smoothly, and the depth chart dynamically reflects pages being discovered across deeper levels.

### 5. Crawling Different Domains
When targeting external sites with a slower, more polite rate limit (e.g., 1000ms/domain), the dashboard accurately tracks the slower influx of data and correctly identifies the new top-level domains in the "Top Domains" section.

### 6. High-Concurrency Crawls
GoSpider excels under pressure. Cranking the worker count to 10 and lowering the rate limit to 200ms results in a rapid influx of fetched pages. The dashboard handles this highly parallelized data stream without lagging.

### 7. Mobile Responsive Layout
The dashboard features a fully responsive design. On smaller devices (like smartphones), the interface automatically rearranges. The top bar elements stack, and the primary metric cards collapse into a single vertical column, ensuring optimal readability and touch navigation.

### 8. Comprehensive Test Suite
GoSpider includes a robust test suite. Running `go test ./...` confirms that all internal packages (crawler, parser, ratelimit, storage) function correctly, with verbose output detailing the success of every integration and unit test.

---

## Test

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

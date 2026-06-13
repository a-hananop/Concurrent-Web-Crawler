# Concurrent Web Crawler

A production-quality concurrent web crawler written in Go, featuring:

- **Goroutine worker pool** — configurable parallelism with clean shutdown
- **Per-domain token-bucket rate limiter** — polite, server-friendly crawling  
- **BFS link traversal** — breadth-first with configurable max depth
- **Context-aware cancellation** — graceful interrupt / timeout handling
- **Deduplication** — atomic test-and-set prevents double-crawling
- **HTML parsing** — extracts title, meta description, word count, and links
- **Race-detector clean** — all shared state properly synchronized

---

## Project Structure

```
webcrawler/
├── cmd/
│   └── crawler/
│       └── main.go              # CLI entry point, flags, live output, final report
├── internal/
│   ├── crawler/
│   │   ├── crawler.go           # Worker pool, BFS queue, orchestration
│   │   └── crawler_test.go      # Integration tests (httptest server)
│   ├── parser/
│   │   ├── parser.go            # HTML parsing, link extraction
│   │   └── parser_test.go       # Unit tests for link resolution
│   ├── ratelimit/
│   │   ├── limiter.go           # Per-domain token-bucket limiter
│   │   └── limiter_test.go      # Burst and multi-domain tests
│   └── storage/
│       ├── store.go             # Thread-safe result store + dedup set
│       └── store_test.go        # Concurrent-access, stats tests
├── go.mod
└── README.md
```

---

## Quick Start

### Build

```bash
go build -o crawler ./cmd/crawler/
```

### Run

```bash
# Crawl example.com — 5 workers, depth 3, polite 500ms/domain rate
./crawler -url https://example.com

# Aggressive crawl — more workers, faster rate, deeper
./crawler -url https://example.com -workers 10 -depth 5 -rate-ms 200

# Allow following external links
./crawler -url https://example.com -same-domain=false -pages 100

# Short smoke test
./crawler -url https://example.com -pages 5 -depth 1
```

### Test

```bash
# All packages with race detector
go test ./... -race

# Verbose output
go test ./... -v -race -timeout 60s
```

---

## CLI Flags

| Flag            | Default          | Description                                      |
|-----------------|------------------|--------------------------------------------------|
| `-url`          | `https://example.com` | Seed URL to start crawling                  |
| `-workers`      | `5`              | Number of concurrent goroutine workers           |
| `-depth`        | `3`              | Maximum link depth from the seed                 |
| `-pages`        | `50`             | Maximum pages to crawl (0 = unlimited)           |
| `-rate-ms`      | `500`            | Minimum ms between requests per domain           |
| `-burst`        | `3`              | Initial burst token count per domain             |
| `-timeout`      | `10`             | Per-request HTTP timeout in seconds              |
| `-same-domain`  | `true`           | Restrict crawling to the seed's domain           |

---

## Architecture

```
Seed URLs
    │
    ▼
Work Queue (chan work, cap 1024)
    │  fan-out
    ├──► Worker 1 ──► Rate Limiter ──► HTTP Fetch ──► HTML Parser ──► Store
    ├──► Worker 2 ──► Rate Limiter ──► HTTP Fetch ──► HTML Parser ──► Store
    ├──► Worker 3 ──► Rate Limiter ──► HTTP Fetch ──► HTML Parser ──► Store
    └──► Worker N      └─ per-domain token bucket     └─ re-enqueue links
```

### Key Design Decisions

**Buffered channel as work queue**  
`chan work` with capacity 1024 decouples producers (link discovery) from consumers (workers). A non-blocking `select default` drops links when the buffer is full rather than deadlocking — important for very broad crawls.

**Token-bucket rate limiter**  
Each domain maintains an independent bucket. Tokens refill proportionally to elapsed time. Workers call `limiter.Wait(domain)` before every fetch — this single line prevents request storms on any server while allowing full parallelism across different domains.

**Atomic deduplication**  
`store.MarkVisited(url)` acquires a write lock, checks, and sets in one critical section. This ensures that even if 10 workers discover the same link simultaneously, only one fetch is ever issued.

**Context propagation**  
Every goroutine receives a `context.Context`. HTTP requests use `http.NewRequestWithContext`, so an OS interrupt or timeout immediately cancels all in-flight network I/O. No goroutine leaks.

**`sync.WaitGroup` for clean drain**  
Workers decrement the WaitGroup on exit. A supervising goroutine calls `wg.Wait()` then closes the queue channel, signalling any remaining workers to exit naturally.

---

## Sample Output

```
🕷  Starting crawl: https://example.com
   workers=5  depth=3  pages=50  rate=500ms  same-domain=true

[d0] ✓ Example Domain                                           3ms
[d1] ✓ IANA — IANA-managed Reserved Domains                    121ms
[d1] ✗ HTTP 404                                                 45ms
[d2] ✓ About IANA                                               98ms

────────────────────────────────────────────────────────────────
Crawl complete in 2.341s
Pages: 4 total | 1 errors | avg 623 words

DEPTH  STATUS  WORDS  TIME    URL
0      200     342    3ms     https://example.com
1      200     891    121ms   https://www.iana.org/domains/reserved
1      ERR     0      45ms    https://example.com/missing
2      200     658    98ms    https://www.iana.org/about
```

---

## Concurrency Model

```
main goroutine
│
├── go c.Run(ctx, seeds)          ← orchestrator
│       │
│       ├── go worker(ctx)  ×N    ← N worker goroutines
│       │       │
│       │       └── process(ctx, work)
│       │               ├── limiter.Wait(domain)    blocks if throttled
│       │               ├── http.Do(req)            cancellable fetch
│       │               ├── parser.Extract(body)    pure function
│       │               ├── store.Save(result)      mutex-protected write
│       │               └── queue <- child links    non-blocking send
│       │
│       └── go wg.Wait → close(queue)   ← drainer
│
└── signal.Notify → cancel()            ← SIGINT handler
```

---

## Extending the Crawler

**Add a robots.txt parser**
```go
// Check before fetching
if !robotsAllow(domain, path) {
    return
}
```

**Export results to JSON**
```go
enc := json.NewEncoder(os.Stdout)
for _, r := range store.All() {
    enc.Encode(r)
}
```

**Plug in a persistent store (e.g. SQLite)**  
Implement the same `Save` / `All` / `MarkVisited` interface against a database instead of the in-memory map.

**Add a politeness delay beyond rate limiting**  
Wrap `limiter.Wait` with an additional `time.Sleep` proportional to the last server response time (crawl-delay header).

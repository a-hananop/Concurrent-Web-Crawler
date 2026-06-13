// Package storage provides thread-safe in-memory storage for crawl results.
package storage

import (
	"sync"
	"time"
)

// Result is the crawl outcome for a single URL.
type Result struct {
	URL        string
	Title      string
	MetaDesc   string
	Links      []string
	WordCount  int
	StatusCode int
	Depth      int
	Duration   time.Duration
	Error      string
	CrawledAt  time.Time
}

// Store is a concurrent-safe map of URL → Result.
type Store struct {
	mu      sync.RWMutex
	results map[string]*Result
	visited map[string]bool
}

// New returns an empty Store.
func New() *Store {
	return &Store{
		results: make(map[string]*Result),
		visited: make(map[string]bool),
	}
}

// MarkVisited records a URL as visited. Returns false if already visited.
func (s *Store) MarkVisited(u string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.visited[u] {
		return false
	}
	s.visited[u] = true
	return true
}

// Save stores a crawl result.
func (s *Store) Save(r *Result) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.results[r.URL] = r
}

// All returns a snapshot of all stored results.
func (s *Store) All() []*Result {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Result, 0, len(s.results))
	for _, r := range s.results {
		out = append(out, r)
	}
	return out
}

// Stats returns summary statistics.
func (s *Store) Stats() (total, errors, avgWords int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	total = len(s.results)
	wordSum := 0
	for _, r := range s.results {
		if r.Error != "" {
			errors++
		}
		wordSum += r.WordCount
	}
	if total > 0 {
		avgWords = wordSum / total
	}
	return
}

// VisitedCount returns how many URLs have been queued for visiting.
func (s *Store) VisitedCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.visited)
}

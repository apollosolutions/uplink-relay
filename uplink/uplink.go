package uplink

import "sync"

// RoundRobinSelector manages rotating through uplink URLs in a round-robin fashion.
type RoundRobinSelector struct {
	urls      []string   // List of URLs to cycle through.
	mu        sync.Mutex // Mutex for thread-safe operation.
	nextIndex int        // Index of the next URL to use.
}

// NewRoundRobinSelector initializes a new RoundRobinSelector with the given URLs.
func NewRoundRobinSelector(urls []string) *RoundRobinSelector {
	return &RoundRobinSelector{
		urls:      urls,
		nextIndex: 0,
	}
}

// Next returns the next URL in the round-robin sequence.
func (rr *RoundRobinSelector) Next() string {
	rr.mu.Lock()
	defer rr.mu.Unlock()
	if len(rr.urls) == 0 {
		return ""
	}
	url := rr.urls[rr.nextIndex]
	rr.nextIndex = (rr.nextIndex + 1) % len(rr.urls)
	return url
}

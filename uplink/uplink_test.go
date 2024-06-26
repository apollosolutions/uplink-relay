package uplink

import "testing"

var urls = []string{"http://example.com", "http://example.org", "http://example.net"}

func TestNewRoundRobinSelector(t *testing.T) {
	rr := NewRoundRobinSelector(urls)

	if len(rr.urls) != len(urls) {
		t.Errorf("Expected %d URLs, but got %d", len(urls), len(rr.urls))
	}

	for i, url := range urls {
		if rr.urls[i] != url {
			t.Errorf("Expected URL at index %d to be %s, but got %s", i, url, rr.urls[i])
		}
	}

	if rr.nextIndex != 0 {
		t.Errorf("Expected nextIndex to be 0, but got %d", rr.nextIndex)
	}
}

func TestRoundRobinSelectorNext(t *testing.T) {
	rr := NewRoundRobinSelector(urls)
	for _, expectedURL := range urls {
		nextURL := rr.Next()
		if nextURL != expectedURL {
			t.Errorf("Expected next URL to be %s, but got %s", expectedURL, nextURL)
		}
	}
}

func TestRoundRobinSelectorEmpty(t *testing.T) {
	rr := NewRoundRobinSelector([]string{})
	next := rr.Next()
	if next != "" {
		t.Errorf("Expected empty URL, but got %s", next)
	}
}

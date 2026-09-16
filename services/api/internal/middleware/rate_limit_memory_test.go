package middleware

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// The limit has to hold when Redis does not. It did not: on a Redis error the
// limiter logged and let the request through, so twelve wrong-password
// attempts against the deployed API in a few seconds were all answered 401 and
// none refused, against a documented limit of five a minute.

func TestTheLimitHoldsWithoutRedis(t *testing.T) {
	m := &memoryLimiter{windows: map[string]*memoryWindow{}, swept: time.Now()}

	const limit = 5
	key := "rate_limit:auth:198.51.100.7"

	for attempt := 1; attempt <= limit; attempt++ {
		allowed, remaining, _ := m.allow(key, limit, time.Minute)
		if !allowed {
			t.Fatalf("attempt %d was refused while within the limit of %d", attempt, limit)
		}
		if want := limit - attempt; remaining != want {
			t.Errorf("after attempt %d, %d remaining, want %d", attempt, remaining, want)
		}
	}

	// The sixth is the one that matters.
	allowed, remaining, reset := m.allow(key, limit, time.Minute)
	if allowed {
		t.Error("the attempt after the limit was allowed through")
	}
	if remaining != 0 {
		t.Errorf("remaining was %d after the limit, want 0", remaining)
	}
	if reset.Before(time.Now()) {
		t.Error("the reset time is in the past")
	}
}

func TestOneClientDoesNotLimitAnother(t *testing.T) {
	m := &memoryLimiter{windows: map[string]*memoryWindow{}, swept: time.Now()}

	for i := 0; i < 5; i++ {
		m.allow("rate_limit:auth:198.51.100.7", 5, time.Minute)
	}

	// A different address is a different window: one officer exhausting their
	// attempts must not lock out the station next door.
	if allowed, _, _ := m.allow("rate_limit:auth:203.0.113.9", 5, time.Minute); !allowed {
		t.Error("a second client was refused because the first had exhausted its limit")
	}
}

func TestTheWindowSlides(t *testing.T) {
	m := &memoryLimiter{windows: map[string]*memoryWindow{}, swept: time.Now()}
	key := "rate_limit:test"

	for i := 0; i < 3; i++ {
		if allowed, _, _ := m.allow(key, 3, 40*time.Millisecond); !allowed {
			t.Fatalf("attempt %d refused while within the limit", i+1)
		}
	}
	if allowed, _, _ := m.allow(key, 3, 40*time.Millisecond); allowed {
		t.Fatal("the fourth attempt was allowed within the window")
	}

	// Once the window has passed, the client is allowed again rather than
	// locked out for ever.
	time.Sleep(60 * time.Millisecond)
	if allowed, _, _ := m.allow(key, 3, 40*time.Millisecond); !allowed {
		t.Error("the client is still refused after the window has passed")
	}
}

// The limiter is on every request, so it is hit from many goroutines at once.
func TestTheLimiterIsSafeUnderConcurrency(t *testing.T) {
	m := &memoryLimiter{windows: map[string]*memoryWindow{}, swept: time.Now()}

	const clients, attempts, limit = 20, 10, 6

	var wg sync.WaitGroup
	allowedCounts := make([]int, clients)

	for client := 0; client < clients; client++ {
		wg.Add(1)
		go func(client int) {
			defer wg.Done()
			key := fmt.Sprintf("rate_limit:test:%d", client)
			for i := 0; i < attempts; i++ {
				if allowed, _, _ := m.allow(key, limit, time.Minute); allowed {
					allowedCounts[client]++
				}
			}
		}(client)
	}
	wg.Wait()

	for client, allowed := range allowedCounts {
		if allowed != limit {
			t.Errorf("client %d was allowed %d of %d attempts, want exactly %d",
				client, allowed, attempts, limit)
		}
	}
}

// A process that runs for months must not keep one entry per address it has
// ever seen.
func TestOldWindowsAreDiscarded(t *testing.T) {
	m := &memoryLimiter{windows: map[string]*memoryWindow{}, swept: time.Now().Add(-2 * time.Minute)}

	for i := 0; i < 50; i++ {
		m.allow(fmt.Sprintf("rate_limit:test:%d", i), 5, 10*time.Millisecond)
	}
	if len(m.windows) != 50 {
		t.Fatalf("expected 50 windows, got %d", len(m.windows))
	}

	time.Sleep(30 * time.Millisecond)
	m.swept = time.Now().Add(-2 * time.Minute) // due a sweep
	m.allow("rate_limit:test:fresh", 5, 10*time.Millisecond)

	if len(m.windows) > 2 {
		t.Errorf("%d windows survived the sweep, want the stale ones discarded", len(m.windows))
	}
}

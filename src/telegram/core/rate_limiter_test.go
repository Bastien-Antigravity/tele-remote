package core

import (
	"testing"
	"time"
)

// -----------------------------------------------------------------------------

func TestRateLimiter_BurstAllowedThenThrottled(t *testing.T) {
	rl := NewRateLimiter(3, 500*time.Millisecond, 1*time.Second)
	key := ConversationKey{ChatID: 100, UserID: 200}

	// 1-3: within limit
	for i := 1; i <= 3; i++ {
		allowed, warn := rl.Allow(key)
		if !allowed {
			t.Fatalf("Request %d expected to be allowed, got denied", i)
		}
		if warn {
			t.Fatalf("Request %d expected warn=false, got true", i)
		}
	}

	// 4: throttled, first warning
	allowed, warn := rl.Allow(key)
	if allowed {
		t.Fatalf("Request 4 expected to be denied, got allowed")
	}
	if !warn {
		t.Fatalf("Request 4 expected warn=true on first throttle, got false")
	}

	// 5: throttled, no warning (in cooldown)
	allowed, warn = rl.Allow(key)
	if allowed {
		t.Fatalf("Request 5 expected to be denied, got allowed")
	}
	if warn {
		t.Fatalf("Request 5 expected warn=false during cooldown, got true")
	}

	// Wait for window to expire
	time.Sleep(550 * time.Millisecond)

	// 6: should be allowed again
	allowed, warn = rl.Allow(key)
	if !allowed {
		t.Fatalf("Request 6 after window expected to be allowed, got denied")
	}
	if warn {
		t.Fatalf("Request 6 expected warn=false, got true")
	}
}

// -----------------------------------------------------------------------------

func TestRateLimiter_IndependentConversations(t *testing.T) {
	rl := NewRateLimiter(2, 500*time.Millisecond, 1*time.Second)
	key1 := ConversationKey{ChatID: 100, UserID: 200}
	key2 := ConversationKey{ChatID: 100, UserID: 300}

	// Saturate key1
	rl.Allow(key1)
	rl.Allow(key1)
	allowed1, _ := rl.Allow(key1)
	if allowed1 {
		t.Fatalf("Key1 expected to be throttled on 3rd request")
	}

	// Key2 must still be allowed
	allowed2, _ := rl.Allow(key2)
	if !allowed2 {
		t.Fatalf("Key2 expected to be allowed independently of Key1")
	}
}

// -----------------------------------------------------------------------------

func TestRateLimiter_NilSafe(t *testing.T) {
	var rl *RateLimiter
	key := ConversationKey{ChatID: 100, UserID: 200}
	allowed, warn := rl.Allow(key)
	if !allowed || warn {
		t.Fatalf("Nil RateLimiter should safely allow requests")
	}
}

// -----------------------------------------------------------------------------

func TestRateLimiter_MaxTrackedClientsEviction(t *testing.T) {
	rl := NewRateLimiter(5, 500*time.Millisecond, 1*time.Second)

	for i := 0; i < MaxTrackedClients+10; i++ {
		key := ConversationKey{ChatID: int64(i + 1), UserID: int64(i + 1)}
		allowed, _ := rl.Allow(key)
		if !allowed {
			t.Fatalf("First request for key %v expected to be allowed", key)
		}
	}

	rl.mu.Lock()
	count := len(rl.clients)
	rl.mu.Unlock()

	if count > MaxTrackedClients {
		t.Fatalf("Expected len(rl.clients) <= %d, got %d", MaxTrackedClients, count)
	}
}


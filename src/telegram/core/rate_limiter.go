package core

/*
ESSENTIAL PROCESS:
Enforces sliding-window rate limiting and anti-flooding controls per Telegram ConversationKey
to protect microservices and prevent Telegram API 429 Too Many Requests penalties.

DATA FLOW:
1. RateLimitMiddleware intercepts incoming updates before logging or handler execution.
2. Allow checks recent timestamps for the client; if within limit, records timestamp and allows.
3. If limit is exceeded, throttles the request and alerts the user at most once per cooldown window.
4. Subsequent flooded requests within cooldown are silently dropped to avoid outbound spam.

KEY PARAMETERS:
- maxRequests: Maximum permitted requests within the sliding window (default 5).
- window: Sliding window duration (default 2s).
- cooldown: Minimum interval between user throttle warnings (default 3s).
*/

import (
	"sync"
	"time"

	tb "gopkg.in/telebot.v3"
)

// -----------------------------------------------------------------------------

const (
	// DefaultBurstLimit allows up to 60 requests per window for high-speed UI interaction.
	DefaultBurstLimit = 60
	// DefaultRateWindow is the sliding window duration.
	DefaultRateWindow = 2 * time.Second
	// DefaultRateCooldown is the minimum interval between throttle warnings sent to the user.
	DefaultRateCooldown = 3 * time.Second
	// MaxTrackedClients caps the in-memory rate limiter client map to prevent unbounded growth.
	MaxTrackedClients = 4096
)

type clientBucket struct {
	timestamps  []time.Time
	lastWarning time.Time
	lastSeen    time.Time
}

// -----------------------------------------------------------------------------

// RateLimiter tracks recent activity per ConversationKey and enforces flood protection.
type RateLimiter struct {
	mu          sync.Mutex
	maxRequests int
	window      time.Duration
	cooldown    time.Duration
	clients     map[ConversationKey]*clientBucket
	lastCleanup time.Time
}

// -----------------------------------------------------------------------------

// NewRateLimiter instantiates a sliding-window rate limiter with sensible defaults.
func NewRateLimiter(maxRequests int, window, cooldown time.Duration) *RateLimiter {
	if maxRequests <= 0 {
		maxRequests = DefaultBurstLimit
	}
	if window <= 0 {
		window = DefaultRateWindow
	}
	if cooldown <= 0 {
		cooldown = DefaultRateCooldown
	}
	return &RateLimiter{
		maxRequests: maxRequests,
		window:      window,
		cooldown:    cooldown,
		clients:     make(map[ConversationKey]*clientBucket),
		lastCleanup: time.Now(),
	}
}

// -----------------------------------------------------------------------------

// Allow determines if an update from the given ConversationKey is permitted.
// Returns (allowed=true, sendWarning=false) if permitted.
// If throttled, returns (allowed=false, sendWarning=true) on first excess,
// and (allowed=false, sendWarning=false) for subsequent floods within cooldown.
func (rl *RateLimiter) Allow(key ConversationKey) (bool, bool) {
	if rl == nil {
		return true, false
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()

	// Lazy cleanup every 60 seconds
	if now.Sub(rl.lastCleanup) > time.Minute {
		cutoff := now.Add(-2 * rl.window)
		for k, b := range rl.clients {
			if len(b.timestamps) == 0 && b.lastWarning.Before(cutoff) {
				delete(rl.clients, k)
			} else if len(b.timestamps) > 0 && b.timestamps[len(b.timestamps)-1].Before(cutoff) && b.lastWarning.Before(cutoff) {
				delete(rl.clients, k)
			}
		}
		rl.lastCleanup = now
	}

	bucket, exists := rl.clients[key]
	if !exists {
		if len(rl.clients) >= MaxTrackedClients {
			for k, b := range rl.clients {
				if now.Sub(b.lastSeen) > 2*rl.window && now.Sub(b.lastWarning) > 2*rl.window {
					delete(rl.clients, k)
				}
			}
			if len(rl.clients) >= MaxTrackedClients {
				var oldestKey ConversationKey
				var oldestTime time.Time
				first := true
				for k, b := range rl.clients {
					if first || b.lastSeen.Before(oldestTime) {
						oldestKey = k
						oldestTime = b.lastSeen
						first = false
					}
				}
				if !first {
					delete(rl.clients, oldestKey)
				}
			}
		}
		bucket = &clientBucket{
			timestamps: make([]time.Time, 0, rl.maxRequests),
		}
		rl.clients[key] = bucket
	}
	bucket.lastSeen = now

	// Evict timestamps outside the sliding window
	threshold := now.Add(-rl.window)
	validIdx := 0
	for i, ts := range bucket.timestamps {
		if ts.After(threshold) {
			validIdx = i
			break
		}
		validIdx = i + 1
	}
	if validIdx > 0 {
		bucket.timestamps = bucket.timestamps[validIdx:]
	}

	if len(bucket.timestamps) < rl.maxRequests {
		bucket.timestamps = append(bucket.timestamps, now)
		return true, false
	}

	// Limit exceeded: decide whether to send throttle warning
	if now.Sub(bucket.lastWarning) >= rl.cooldown {
		bucket.lastWarning = now
		return false, true
	}

	return false, false
}

// -----------------------------------------------------------------------------

// RateLimitMiddleware drops rapid updates per ConversationKey before executing handlers.
func (bot *Bot) RateLimitMiddleware() tb.MiddlewareFunc {
	return func(next tb.HandlerFunc) tb.HandlerFunc {
		return func(c tb.Context) error {
			if bot == nil || bot.Limiter == nil {
				return next(c)
			}

			convKey := GetConvKey(c)
			allowed, sendWarning := bot.Limiter.Allow(convKey)
			if !allowed {
				if sendWarning {
					bot.Log.Warning("Rate limit exceeded; throttling client",
						"chat_id", convKey.ChatID, "user_id", convKey.UserID)
					if c.Callback() != nil {
						_ = c.Respond(&tb.CallbackResponse{
							Text:      "⏳ Slow down: too many requests.",
							ShowAlert: true,
						})
					} else {
						_ = bot.Send(c, "⏳ Slow down: too many requests. Please wait a moment.")
					}
				}
				// Drop the flooded update silently
				return nil
			}
			return next(c)
		}
	}
}


package middleware

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"thinker/internal/auth"
)

type slidingWindowEntry struct {
	timestamps []time.Time
	lastSeen   time.Time
}

type AuthRateLimiter struct {
	mu            sync.Mutex
	ipEntries     map[string]*slidingWindowEntry
	acctEntries   map[string]*slidingWindowEntry
	ipMaxReqs     int
	ipWindow      time.Duration
	acctMaxReqs   int
	acctWindow    time.Duration
	lastCleanupAt time.Time
}

func NewAuthRateLimiter(ipMaxReqs int, ipWindow time.Duration, acctMaxReqs int, acctWindow time.Duration) *AuthRateLimiter {
	return &AuthRateLimiter{
		ipEntries:     make(map[string]*slidingWindowEntry),
		acctEntries:   make(map[string]*slidingWindowEntry),
		ipMaxReqs:     ipMaxReqs,
		ipWindow:      ipWindow,
		acctMaxReqs:   acctMaxReqs,
		acctWindow:    acctWindow,
		lastCleanupAt: time.Now().UTC(),
	}
}

func (rl *AuthRateLimiter) LimitEndpoint() gin.HandlerFunc {
	return func(c *gin.Context) {
		now := time.Now().UTC()
		clientIP := c.ClientIP()
		if clientIP == "" {
			clientIP = "unknown"
		}

		var acctKey string
		if c.Request.Method == http.MethodPost {
			rawEmail := strings.TrimSpace(c.PostForm("email"))
			if rawEmail != "" {
				if normEmail, err := auth.NormalizeEmail(rawEmail); err == nil {
					acctKey = normEmail
				} else {
					acctKey = strings.ToLower(rawEmail)
				}
			}
		}

		allowed, retryAfter := rl.allow(clientIP, acctKey, now)
		if !allowed {
			retrySec := int(retryAfter.Seconds())
			if retrySec < 1 {
				retrySec = 60
			}
			c.Header("Retry-After", fmt.Sprintf("%d", retrySec))

			msg := "Demasiados intentos en poco tiempo. Por favor espera unos minutos antes de volver a intentarlo."
			if strings.HasPrefix(c.Request.URL.Path, "/api/") || strings.Contains(c.GetHeader("Accept"), "application/json") {
				c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": msg})
				return
			}
			c.String(http.StatusTooManyRequests, msg)
			c.Abort()
			return
		}

		c.Next()
	}
}

func (rl *AuthRateLimiter) allow(ipKey, acctKey string, now time.Time) (bool, time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if now.Sub(rl.lastCleanupAt) > 5*time.Minute {
		rl.cleanupLocked(now)
		rl.lastCleanupAt = now
	}

	if rl.ipMaxReqs > 0 && ipKey != "" {
		if ok, retry := checkSlidingWindow(rl.ipEntries, ipKey, rl.ipMaxReqs, rl.ipWindow, now); !ok {
			return false, retry
		}
	}

	if rl.acctMaxReqs > 0 && acctKey != "" {
		if ok, retry := checkSlidingWindow(rl.acctEntries, acctKey, rl.acctMaxReqs, rl.acctWindow, now); !ok {
			return false, retry
		}
	}

	if rl.ipMaxReqs > 0 && ipKey != "" {
		recordSlidingWindow(rl.ipEntries, ipKey, now)
	}
	if rl.acctMaxReqs > 0 && acctKey != "" {
		recordSlidingWindow(rl.acctEntries, acctKey, now)
	}

	return true, 0
}

func checkSlidingWindow(store map[string]*slidingWindowEntry, key string, maxReqs int, window time.Duration, now time.Time) (bool, time.Duration) {
	entry, exists := store[key]
	if !exists {
		return true, 0
	}

	cutoff := now.Add(-window)
	valid := entry.timestamps[:0]
	for _, ts := range entry.timestamps {
		if ts.After(cutoff) {
			valid = append(valid, ts)
		}
	}
	entry.timestamps = valid
	entry.lastSeen = now

	if len(entry.timestamps) >= maxReqs {
		oldest := entry.timestamps[0]
		retryAfter := oldest.Add(window).Sub(now)
		if retryAfter < time.Second {
			retryAfter = time.Second
		}
		return false, retryAfter
	}
	return true, 0
}

func recordSlidingWindow(store map[string]*slidingWindowEntry, key string, now time.Time) {
	entry, exists := store[key]
	if !exists {
		entry = &slidingWindowEntry{}
		store[key] = entry
	}
	entry.timestamps = append(entry.timestamps, now)
	entry.lastSeen = now
}

func (rl *AuthRateLimiter) cleanupLocked(now time.Time) {
	for k, v := range rl.ipEntries {
		if now.Sub(v.lastSeen) > rl.ipWindow*2 {
			delete(rl.ipEntries, k)
		}
	}
	for k, v := range rl.acctEntries {
		if now.Sub(v.lastSeen) > rl.acctWindow*2 {
			delete(rl.acctEntries, k)
		}
	}
}

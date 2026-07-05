package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// rlWindow is one client's request tally for the current fixed window.
type rlWindow struct {
	count   int
	resetAt time.Time
}

type rateLimiter struct {
	mu     sync.Mutex
	hits   map[string]*rlWindow
	max    int
	window time.Duration
}

// RateLimit returns middleware that allows at most max requests per window per
// client IP, replying 429 (with a Retry-After header) once the limit is passed.
// It's a simple in-memory fixed-window limiter — enough to blunt brute-force
// login attempts on this single-instance console; it is not a distributed
// limiter. Memory is bounded by opportunistically dropping expired windows.
func RateLimit(max int, window time.Duration) gin.HandlerFunc {
	rl := &rateLimiter{hits: make(map[string]*rlWindow), max: max, window: window}
	return func(c *gin.Context) {
		ip := c.ClientIP()
		now := time.Now()

		rl.mu.Lock()
		w := rl.hits[ip]
		if w == nil || now.After(w.resetAt) {
			w = &rlWindow{resetAt: now.Add(rl.window)}
			rl.hits[ip] = w
		}
		w.count++
		over := w.count > rl.max
		retryAfter := time.Until(w.resetAt)
		if len(rl.hits) > 4096 {
			for k, v := range rl.hits {
				if now.After(v.resetAt) {
					delete(rl.hits, k)
				}
			}
		}
		rl.mu.Unlock()

		if over {
			secs := int(retryAfter.Seconds()) + 1
			c.Header("Retry-After", strconv.Itoa(secs))
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error": fmt.Sprintf("too many attempts — please wait %d seconds and try again", secs),
			})
			c.Abort()
			return
		}
		c.Next()
	}
}

package agentd

import (
	"net"
	"net/http"
	"sync"

	"golang.org/x/time/rate"
)

// ipLimiter tracks a per-IP token bucket. New visitors get a fresh
// limiter on first contact; the bucket is updated lazily.
type ipLimiter struct {
	mu       sync.Mutex
	visitors map[string]*rate.Limiter
	r        rate.Limit
	burst    int
}

func newIPLimiter(r rate.Limit, burst int) *ipLimiter {
	return &ipLimiter{visitors: map[string]*rate.Limiter{}, r: r, burst: burst}
}

func (l *ipLimiter) limiterFor(ip string) *rate.Limiter {
	l.mu.Lock()
	defer l.mu.Unlock()
	lim, ok := l.visitors[ip]
	if ok {
		return lim
	}
	lim = rate.NewLimiter(l.r, l.burst)
	l.visitors[ip] = lim
	return lim
}

// rateLimitMiddleware throttles per-client-IP. A blocked request
// returns 429 Too Many Requests with a Retry-After hint. Returns
// next as-is when l is nil (no limit configured).
func rateLimitMiddleware(next http.Handler, l *ipLimiter) http.Handler {
	if l == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			ip = r.RemoteAddr
		}
		lim := l.limiterFor(ip)
		if !lim.Allow() {
			w.Header().Set("Retry-After", "1")
			http.Error(w, "rate limited", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

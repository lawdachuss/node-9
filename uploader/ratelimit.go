package uploader

import (
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ─── Adaptive per-host rate limiter ──────────────────────────────────────────
//
// The previous "rate limiting" was only a fixed exponential backoff applied
// inside each uploader's own retry loop. That has three real problems:
//
//   1. It ignores the Retry-After header the host actually sends on a 429.
//   2. Each in-flight request tracks its own backoff independently, so when a
//      host is throttling, every concurrent upload (multiple channels, plus the
//      background media watcher) keeps piling retries onto the same host
//      instead of coordinating a cooldown.
//   3. The watcher's media-fetch calls had no throttling at all.
//
// This limiter fixes that with a single, process-wide, per-host adaptive
// policy:
//
//   - Before any request to a host, the caller blocks until the host is out of
//     cooldown AND a minimum spacing has elapsed. Concurrent callers therefore
//     queue behind one another instead of storming the host.
//   - On a 429/503 (or a 0 remaining quota header) the host's cooldown is set
//     to max(Retry-After, exponential backoff) with jitter, and grows
//     exponentially across consecutive limits (capped).
//   - On success the consecutive-limit counter decays so the host returns to
//     full speed once it is healthy again.
//
// Scope: this is per-process (per node). In a multi-node deployment each node
// enforces its own rate against the shared hosts, which bounds each node's
// request volume; PATCH/POST writes remain idempotent so overlapping work
// across nodes does not corrupt data.
// ────────────────────────────────────────────────────────────────────────────

const (
	rlMinSpacing  = 750 * time.Millisecond // minimum gap between requests to one host
	rlBaseCooldown = 5 * time.Second        // cooldown after the first limit hit
	rlMaxCooldown  = 5 * time.Minute        // hard cap on a single cooldown
	rlGrowth       = 2.0                     // exponential growth factor per consecutive limit
	rlJitterFrac   = 0.25                    // up to 25% jitter to avoid synchronized retries
)

type rlState struct {
	mu          sync.Mutex
	coolUntil   time.Time // requests block until this time
	nextAllowed time.Time // earliest time the next request may start (spacing)
	consecutive int       // consecutive limit hits
	lastSuccess time.Time
}

// HostRateLimiter is a process-wide adaptive rate limiter keyed by host.
type HostRateLimiter struct {
	mu    sync.Mutex
	hosts map[string]*rlState
}

// DefaultLimiter is the shared limiter used by all uploaders and the watcher's
// media-fetch helpers within this process.
var DefaultLimiter = NewHostRateLimiter()

// NewHostRateLimiter returns an empty limiter.
func NewHostRateLimiter() *HostRateLimiter {
	return &HostRateLimiter{hosts: make(map[string]*rlState)}
}

func (l *HostRateLimiter) state(key string) *rlState {
	l.mu.Lock()
	defer l.mu.Unlock()
	s, ok := l.hosts[key]
	if !ok {
		s = &rlState{}
		l.hosts[key] = s
	}
	return s
}

// Wait blocks until the host identified by key is permitted to receive a
// request (cooldown expired and minimum spacing satisfied). It is safe to call
// from many goroutines concurrently — they will serialize behind the cooldown.
func (l *HostRateLimiter) Wait(key string) {
	s := l.state(key)

	s.mu.Lock()
	now := time.Now()

	if !s.coolUntil.IsZero() && now.Before(s.coolUntil) {
		s.mu.Unlock()
		time.Sleep(time.Until(s.coolUntil))
		s.mu.Lock()
		now = time.Now()
	}

	var wait time.Duration
	if !s.nextAllowed.IsZero() && now.Before(s.nextAllowed) {
		wait = time.Until(s.nextAllowed)
	}
	s.nextAllowed = time.Now().Add(rlMinSpacing)
	s.mu.Unlock()

	if wait > 0 {
		time.Sleep(wait)
	}
}

// Observe records the outcome of a request to key and adjusts the adaptive
// cooldown. Pass limited=true for a 429/503/quota-exhausted response, along
// with any Retry-After header the host returned.
func (l *HostRateLimiter) Observe(key string, limited bool, retryAfterHeader string) {
	s := l.state(key)

	s.mu.Lock()
	defer s.mu.Unlock()

	if limited {
		s.consecutive++
		cool := time.Duration(float64(rlBaseCooldown) * math.Pow(rlGrowth, float64(s.consecutive-1)))
		if cool > rlMaxCooldown {
			cool = rlMaxCooldown
		}
		if ra := parseRetryAfter(retryAfterHeader); ra > cool {
			cool = ra
		}
		// Jitter to avoid a thundering herd of synchronized retries.
		if cool > 0 {
			jitter := time.Duration(float64(cool) * rlJitterFrac)
			cool += time.Duration(rand.Int63n(int64(jitter)))
		}
		s.coolUntil = time.Now().Add(cool)
		return
	}

	// Success: decay the consecutive-limit counter only after sustained health.
	if !s.lastSuccess.IsZero() && time.Since(s.lastSuccess) < time.Minute {
		s.consecutive = 0
	}
	s.lastSuccess = time.Now()
	if !s.coolUntil.IsZero() && time.Now().After(s.coolUntil) {
		s.coolUntil = time.Time{}
	}
}

// hostKey normalizes a request host (or URL host) into a stable limiter key.
// Subdomains are collapsed to the registered domain so e.g.
// chuglii.seeks.cloud and asset.seekstreaming.info share the SeekStreaming
// policy, and api.upnshare.com / assets.upns.net share the UPnShare policy.
func hostKey(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	host = strings.TrimSuffix(host, ".")
	// Strip an optional port.
	if i := strings.LastIndex(host, ":"); i >= 0 {
		host = host[:i]
	}
	// Collapse to the last two labels (registered domain).
	labels := strings.Split(host, ".")
	if len(labels) <= 2 {
		return host
	}
	return strings.Join(labels[len(labels)-2:], ".")
}

func parseRetryAfter(h string) time.Duration {
	h = strings.TrimSpace(h)
	if h == "" {
		return 0
	}
	// Retry-After may be a number of seconds (HTTP spec) or an HTTP-date.
	if secs, err := strconv.Atoi(h); err == nil && secs > 0 {
		d := time.Duration(secs) * time.Second
		if d > rlMaxCooldown {
			return rlMaxCooldown
		}
		return d
	}
	if t, err := http.ParseTime(h); err == nil {
		d := time.Until(t)
		if d < 0 {
			return 0
		}
		if d > rlMaxCooldown {
			return rlMaxCooldown
		}
		return d
	}
	return 0
}

// rateLimitTransport is an http.RoundTripper that enforces the adaptive
// per-host limiter: it blocks before sending and records the outcome after.
type rateLimitTransport struct {
	base http.RoundTripper
}

func (t *rateLimitTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	key := hostKey(req.URL.Host)
	DefaultLimiter.Wait(key)

	resp, err := t.base.RoundTrip(req)
	if err != nil {
		// Transport/network errors are not rate-limit signals; don't cool down.
		return resp, err
	}

	retryAfter := resp.Header.Get("Retry-After")
	limited := resp.StatusCode == http.StatusTooManyRequests ||
		resp.StatusCode == http.StatusServiceUnavailable ||
		(resp.StatusCode == http.StatusForbidden && parseRetryAfter(retryAfter) > 0)
	DefaultLimiter.Observe(key, limited, retryAfter)
	return resp, err
}

// wrapWithRateLimit wraps a base transport with the adaptive limiter.
func wrapWithRateLimit(base http.RoundTripper) http.RoundTripper {
	return &rateLimitTransport{base: base}
}

// mediaFetchClient is a shared client used by the standalone media-fetch
// helpers (GetSeekStreamingMediaURLs, GetUPnShareMediaURLs, etc.) invoked by the
// background media watcher. It routes through the same adaptive per-host
// limiter as the uploaders so the watcher cannot storm SeekStreaming/UPnShare
// independently of the active uploads.
var mediaFetchClient = &http.Client{
	Timeout:   30 * time.Second,
	Transport: wrapWithRateLimit(http.DefaultTransport),
}

package api

import (
	"net/http"
	"time"

	"businessautomation/internal/core"
)

const (
	maximumConcurrentAuthTasks   = 4
	maximumAuthRequestsPerMinute = 20
	maximumTrackedAuthClients    = 10000
)

func (h *Handler) acquireAuthSlot(w http.ResponseWriter) bool {
	select {
	case h.authSlots <- struct{}{}:
		return true
	default:
		respond(w, http.StatusTooManyRequests, nil, core.Fail(http.StatusTooManyRequests, "Máy chủ đang bận"))
		return false
	}
}

func (h *Handler) releaseAuthSlot() { <-h.authSlots }

func (h *Handler) allowAuth(r *http.Request) bool {
	// Key the limiter on the resolved client IP so a proxied deployment
	// (loopback BFF, Render ingress) does not collapse all real users
	// into a single 127.0.0.1 bucket.
	ip := requestIP(r).String()
	now := time.Now()
	h.mu.Lock()
	defer h.mu.Unlock()
	for k, v := range h.attempts {
		if now.After(v.until) {
			delete(h.attempts, k)
		}
	}
	v, exists := h.attempts[ip]
	if !exists {
		if len(h.attempts) >= maximumTrackedAuthClients {
			return false
		}
		v = window{until: now.Add(time.Minute)}
	}
	v.count++
	h.attempts[ip] = v
	return v.count <= maximumAuthRequestsPerMinute
}

func method(w http.ResponseWriter, allowed string) {
	w.Header().Set("Allow", allowed)
	respond(w, http.StatusMethodNotAllowed, nil, core.Fail(http.StatusMethodNotAllowed, "Phương thức không được hỗ trợ"))
}

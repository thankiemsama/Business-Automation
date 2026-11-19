package telemetry

import (
	"crypto/subtle"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

var buckets = [...]time.Duration{
	5 * time.Millisecond, 10 * time.Millisecond, 25 * time.Millisecond,
	50 * time.Millisecond, 100 * time.Millisecond, 250 * time.Millisecond,
	500 * time.Millisecond, time.Second, 2500 * time.Millisecond,
	5 * time.Second, 10 * time.Second, 30 * time.Second,
}

type histogram struct {
	bins  [len(buckets) + 1]atomic.Uint64
	nanos atomic.Uint64
}

func (h *histogram) observe(d time.Duration) {
	index := 0
	for index < len(buckets) && d > buckets[index] {
		index++
	}
	h.bins[index].Add(1)
	h.nanos.Add(uint64(d))
}

func (h *histogram) write(w io.Writer, name string) {
	fmt.Fprintf(w, "# TYPE %s histogram\n", name)
	var count uint64
	for index, bound := range buckets {
		count += h.bins[index].Load()
		fmt.Fprintf(w, "%s_bucket{le=%q} %d\n", name, fmt.Sprintf("%g", bound.Seconds()), count)
	}
	count += h.bins[len(buckets)].Load()
	fmt.Fprintf(w, "%s_bucket{le=\"+Inf\"} %d\n%s_count %d\n%s_sum %g\n", name, count, name, count, name,
		float64(h.nanos.Load())/float64(time.Second))
}

var (
	requests          [6]atomic.Uint64
	httpDuration      histogram
	writeLockDuration histogram
	writeLockErrors   atomic.Uint64
)

func ObserveWriteLock(duration time.Duration, acquired bool) {
	writeLockDuration.observe(duration)
	if !acquired {
		writeLockErrors.Add(1)
	}
}

type response struct {
	http.ResponseWriter
	status int
}

func (r *response) Unwrap() http.ResponseWriter { return r.ResponseWriter }

func (r *response) WriteHeader(status int) {
	if status >= 200 && r.status == 0 {
		r.status = status
	}
	r.ResponseWriter.WriteHeader(status)
}

func (r *response) Write(data []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(data)
}

func Handler(next http.Handler, token string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/metrics" {
			if token == "" || r.Method != http.MethodGet ||
				subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+token)) != 1 {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			fmt.Fprintln(w, "# TYPE platform_http_requests_total counter")
			for class := 2; class <= 5; class++ {
				fmt.Fprintf(w, "platform_http_requests_total{status_class=\"%dxx\"} %d\n", class, requests[class].Load())
			}
			httpDuration.write(w, "platform_http_request_duration_seconds")
			writeLockDuration.write(w, "platform_postgres_write_lock_wait_seconds")
			fmt.Fprintln(w, "# TYPE platform_postgres_write_lock_errors_total counter")
			fmt.Fprintf(w, "platform_postgres_write_lock_errors_total %d\n", writeLockErrors.Load())
			return
		}
		start := time.Now()
		tracked := &response{ResponseWriter: w}
		next.ServeHTTP(tracked, r)
		if strings.HasPrefix(w.Header().Get("Content-Type"), "text/event-stream") {
			return
		}
		class := tracked.status / 100
		if tracked.status == 0 {
			class = 2
		}
		if class >= 2 && class <= 5 {
			requests[class].Add(1)
		}
		httpDuration.observe(time.Since(start))
	})
}

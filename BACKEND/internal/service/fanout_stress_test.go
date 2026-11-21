package service

import (
	"sync"
	"testing"
	"time"
)

// TestMemoryFanoutConcurrentWatchers hammers Watch/Publish/stop across
// many goroutines and tenants to shake out data races in the fanout map
// management. It is not a substitute for the Go race detector, but it
// fails loudly on deadlocks, channel overflow, or lost updates.
func TestMemoryFanoutConcurrentWatchers(t *testing.T) {
	fanout := NewMemoryFanout()
	const tenants = 32
	const workers = 16

	var wg sync.WaitGroup
	stopFn := make([]func(), workers)
	done := make(chan struct{})

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			scope := "tenant-" + string(rune('a'+w%tenants))
			cursor, events, stop := fanout.Watch(scope)
			stopFn[w] = stop
			last := cursor
			for {
				select {
				case c := <-events:
					if c == last {
						t.Error("duplicate cursor delivered")
						return
					}
					last = c
				case <-done:
					return
				}
			}
		}(w)
	}

	// Publishers churn across all tenants while watchers are alive.
	var pwg sync.WaitGroup
	quit := make(chan struct{})
	for p := 0; p < 8; p++ {
		pwg.Add(1)
		go func(p int) {
			defer pwg.Done()
			for i := 0; ; i++ {
				select {
				case <-quit:
					return
				default:
					fanout.Publish([]string{"tenant-" + string(rune('a'+i%tenants))})
					time.Sleep(time.Microsecond)
				}
			}
		}(p)
	}

	time.Sleep(250 * time.Millisecond)
	close(quit)
	pwg.Wait()
	close(done)
	wg.Wait()
	for _, s := range stopFn {
		if s != nil {
			s()
		}
	}
}

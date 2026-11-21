package service

import (
	"os"
	"testing"
	"time"
)

// TestMemoryFanoutPublishAndWatch locks the single-process behavior:
// Watch returns the current cursor; Publish after Watch delivers the
// new cursor to that watcher only; a stopped watcher receives nothing.
func TestMemoryFanoutPublishAndWatch(t *testing.T) {
	fanout := NewMemoryFanout()
	cursor, events, stop := fanout.Watch("tenant-a")
	defer stop()

	fanout.Publish([]string{"tenant-a"})
	select {
	case got := <-events:
		if got == cursor {
			t.Fatal("publish delivered the pre-watch cursor")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("publish did not deliver a cursor")
	}

	// An unrelated scope must not deliver.
	_, events3, stop3 := fanout.Watch("tenant-c")
	defer stop3()
	fanout.Publish([]string{"tenant-a"})
	select {
	case <-events3:
		t.Fatal("unrelated scope received a publish")
	default:
	}

	// A stopped watcher must not deliver.
	_, stopped, stopFn := fanout.Watch("tenant-d")
	stopFn()
	fanout.Publish([]string{"tenant-d"})
	select {
	case <-stopped:
		t.Fatal("stopped watcher still receives events")
	default:
	}
}

// TestMemoryFanoutSlowWatcherNeverBlocks locks the non-blocking delivery
// contract: a watcher that stops draining must not stall Publish for
// other watchers on the same scope.
func TestMemoryFanoutSlowWatcherNeverBlocks(t *testing.T) {
	fanout := NewMemoryFanout()
	// Slow watcher: subscribe but never drain, to prove Publish does not block on it.
	_, _, stopSlow := fanout.Watch("tenant-a")
	defer stopSlow()
	_, fast, stopFast := fanout.Watch("tenant-a")
	defer stopFast()

	done := make(chan struct{})
	go func() {
		fanout.Publish([]string{"tenant-a"})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("publish blocked on a slow watcher")
	}
	select {
	case <-fast:
	default:
		t.Fatal("fast watcher did not receive the publish")
	}
}

// TestMemoryFanoutCursorMonotonic locks that the cursor strictly
// increases across publishes on the same scope.
func TestMemoryFanoutCursorMonotonic(t *testing.T) {
	fanout := NewMemoryFanout()
	_, events, stop := fanout.Watch("t")
	defer stop()

	var cursors []string
	for i := 0; i < 5; i++ {
		fanout.Publish([]string{"t"})
		select {
		case c := <-events:
			cursors = append(cursors, c)
		case <-time.After(2 * time.Second):
			t.Fatal("no cursor delivered")
		}
	}
	for i := 1; i < len(cursors); i++ {
		if cursors[i] <= cursors[i-1] {
			t.Fatalf("cursor did not advance: %q -> %q", cursors[i-1], cursors[i])
		}
	}
}

// TestNATSFanoutRoundTrip connects two fanout instances to the same
// NATS server (when NEXA_NATS_TEST_URL is set) and verifies cross
// connection delivery: a Publish on one is observed by a Watch on the
// other. Skipped without a server.
func TestNATSFanoutRoundTrip(t *testing.T) {
	url := os.Getenv("NEXA_NATS_TEST_URL")
	if url == "" {
		t.Skip("set NEXA_NATS_TEST_URL to a reachable NATS server")
	}
	a, err := OpenNATSFanout(url, "roundtrip")
	if err != nil {
		t.Fatalf("open fanout A: %v", err)
	}
	defer a.Close()
	b, err := OpenNATSFanout(url, "roundtrip")
	if err != nil {
		t.Fatalf("open fanout B: %v", err)
	}
	defer b.Close()

	_, events, stop := a.Watch("cross-tenant")
	defer stop()
	b.Publish([]string{"cross-tenant"})
	select {
	case <-events:
	case <-time.After(5 * time.Second):
		t.Fatal("cross-connection publish was not delivered")
	}
}

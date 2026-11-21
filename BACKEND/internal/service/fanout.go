// Package service — EventFanout: cross-replica SSE event delivery.
//
// EventFanout abstracts the "something changed, go refetch" signal that
// powers the /events SSE endpoint. In a single-process deployment the
// in-memory implementation is used. When NEXA_NATS_URL is set, a NATS
// JetStream stream propagates events across replicas so every replica's
// SSE connections see changes made by any replica.
package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// EventFanout delivers "scope changed" signals to SSE watchers.
// The cursor is a monotonically increasing string that clients send back
// as Last-Event-ID on reconnect; if the server cursor has advanced, the
// client receives a refresh instruction and refetches.
type EventFanout interface {
	// Watch subscribes to events for scope. Returns the current cursor,
	// a channel that delivers new cursors, and a stop function.
	Watch(scope string) (string, <-chan string, func())
	// Publish signals that the given scopes have changed.
	Publish(scopes []string)
}

// ── Memory implementation (default, single-process) ──────────────

// MemoryFanout keeps watchers in-process. Safe for single-replica
// deployments. Cursor format: "{epoch}-{revision}".
type MemoryFanout struct {
	mu       sync.RWMutex
	watchers map[string]map[chan string]struct{}
	revision uint64
	epoch    string
}

var _ EventFanout = (*MemoryFanout)(nil)

func NewMemoryFanout() *MemoryFanout {
	b := make([]byte, 8)
	rand.Read(b)
	return &MemoryFanout{
		watchers: make(map[string]map[chan string]struct{}),
		epoch:    hex.EncodeToString(b),
	}
}

func (m *MemoryFanout) cursor() string {
	return m.epoch + "-" + strconv.FormatUint(m.revision, 10)
}

func (m *MemoryFanout) Watch(scope string) (string, <-chan string, func()) {
	m.mu.Lock()
	ch := make(chan string, 1)
	if m.watchers[scope] == nil {
		m.watchers[scope] = make(map[chan string]struct{})
	}
	m.watchers[scope][ch] = struct{}{}
	cursor := m.cursor()
	m.mu.Unlock()
	return cursor, ch, func() {
		m.mu.Lock()
		delete(m.watchers[scope], ch)
		if len(m.watchers[scope]) == 0 {
			delete(m.watchers, scope)
		}
		m.mu.Unlock()
	}
}

func (m *MemoryFanout) Publish(scopes []string) {
	m.mu.Lock()
	m.revision++
	cursor := m.cursor()
	type target struct{ scope string; subs []chan string }
	targets := make([]target, 0, len(scopes))
	for _, scope := range scopes {
		subs := make([]chan string, 0, len(m.watchers[scope]))
		for sub := range m.watchers[scope] {
			subs = append(subs, sub)
		}
		if len(subs) > 0 {
			targets = append(targets, target{scope, subs})
		}
	}
	m.mu.Unlock()
	for _, t := range targets {
		for _, sub := range t.subs {
			select {
			case <-sub:
			default:
			}
			sub <- cursor
		}
	}
}

// ── NATS JetStream implementation (multi-replica) ───────────────

const natsSSEStream = "NEXA_SSE"

// NATSFanout propagates scope-change events through a NATS JetStream
// stream. Every replica publishes to sse.{scope} and consumes the same
// stream, so all replicas stay in sync. The cursor is the JetStream
// sequence number, which is global and monotonically increasing.
type NATSFanout struct {
	nc       *nats.Conn
	js       jetstream.JetStream
	mu       sync.RWMutex
	watchers map[string]map[chan string]struct{}
	revision uint64 // mirrors the latest NATS sequence seen
	epoch    string // shared across replicas (NEXA_FANOUT_EPOCH or stream name)
	sub      *nats.Subscription
}

var _ EventFanout = (*NATSFanout)(nil)

// NATSFanoutFromEnvironment creates a NATS-backed fanout when
// NEXA_NATS_URL is set; returns (nil, nil) otherwise so the caller
// falls back to the memory implementation.
func NATSFanoutFromEnvironment() (*NATSFanout, error) {
	url := os.Getenv("NEXA_NATS_URL")
	if url == "" {
		return nil, nil
	}
	return OpenNATSFanout(url, os.Getenv("NEXA_FANOUT_EPOCH"))
}

// OpenNATSFanout connects to a NATS server at url and wires the SSE
// fanout. When epoch is empty, the stream name is used so replicas
// share a cursor namespace. The caller owns Close.
func OpenNATSFanout(url, epoch string) (*NATSFanout, error) {
	if epoch == "" {
		epoch = natsSSEStream
	}
	opts := []nats.Option{
		nats.ReconnectWait(3 * time.Second),
		nats.MaxReconnects(-1),
	}
	if user := os.Getenv("NEXA_NATS_USER"); user != "" {
		opts = append(opts, nats.UserInfo(user, os.Getenv("NEXA_NATS_PASSWORD")))
	}
	nc, err := nats.Connect(url, opts...)
	if err != nil {
		return nil, fmt.Errorf("NATS connect: %w", err)
	}
	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("NATS JetStream: %w", err)
	}
	// Create the stream (idempotent: an existing stream with the same
	// managed config is accepted, a different one is a real error).
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err = js.CreateStream(ctx, jetstream.StreamConfig{
		Name:      natsSSEStream,
		Subjects:  []string{"sse.*"},
		Retention: jetstream.LimitsPolicy,
		MaxAge:    1 * time.Hour,
		Storage:   jetstream.MemoryStorage,
	})
	if err != nil && !errors.Is(err, jetstream.ErrStreamNameAlreadyInUse) {
		nc.Close()
		return nil, fmt.Errorf("NATS create stream: %w", err)
	}
	f := &NATSFanout{
		nc:       nc,
		js:       js,
		epoch:    epoch,
		watchers: make(map[string]map[chan string]struct{}),
	}
	if err := f.startForwarding(); err != nil {
		nc.Close()
		return nil, err
	}
	return f, nil
}

// natsSubject converts a logical scope to a NATS subject token.
// "chat:t1:c1" → "sse.chat.t1.c1", "t1" → "sse.t1".
func natsSubject(scope string) string {
	return "sse." + strings.ReplaceAll(scope, ":", ".")
}

// startForwarding subscribes to all sse.* subjects and forwards
// incoming events to local watchers.
func (n *NATSFanout) startForwarding() error {
	sub, err := n.nc.Subscribe("sse.*", func(m *nats.Msg) {
		scope := strings.TrimPrefix(m.Subject, "sse.")
		scope = strings.ReplaceAll(scope, ".", ":")
		seq := n.sequenceFromMsg(m)
		n.mu.Lock()
		if seq > n.revision {
			n.revision = seq
		}
		cursor := n.cursor()
		subs := make([]chan string, 0, len(n.watchers[scope]))
		for ch := range n.watchers[scope] {
			subs = append(subs, ch)
		}
		n.mu.Unlock()
		for _, ch := range subs {
			select {
			case <-ch:
			default:
			}
			ch <- cursor
		}
	})
	if err != nil {
		return fmt.Errorf("NATS subscribe sse.*: %w", err)
	}
	n.sub = sub
	return nil
}

func (n *NATSFanout) cursor() string {
	return n.epoch + "-" + strconv.FormatUint(n.revision, 10)
}

// sequenceFromMsg extracts the JetStream stream sequence from a message.
// In JetStream, the Reply field carries the "STREAM.seq" token when the
// message is published to a stream.
func (n *NATSFanout) sequenceFromMsg(m *nats.Msg) uint64 {
	meta, err := m.Metadata()
	if err == nil && meta != nil {
		return meta.Sequence.Stream
	}
	// Fallback: reply token is "STREAM.SEQ"
	parts := strings.Split(m.Reply, ".")
	if len(parts) == 2 {
		seq, _ := strconv.ParseUint(parts[1], 10, 64)
		return seq
	}
	return 0
}

func (n *NATSFanout) Watch(scope string) (string, <-chan string, func()) {
	n.mu.Lock()
	ch := make(chan string, 1)
	if n.watchers[scope] == nil {
		n.watchers[scope] = make(map[chan string]struct{})
	}
	n.watchers[scope][ch] = struct{}{}
	cursor := n.cursor()
	n.mu.Unlock()
	return cursor, ch, func() {
		n.mu.Lock()
		delete(n.watchers[scope], ch)
		if len(n.watchers[scope]) == 0 {
			delete(n.watchers, scope)
		}
		n.mu.Unlock()
	}
}

func (n *NATSFanout) Publish(scopes []string) {
	for _, scope := range scopes {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_, err := n.js.Publish(ctx, natsSubject(scope), []byte("{}"))
		cancel()
		if err != nil {
			slog.Warn("NATS SSE publish failed", "scope", scope, "error", err)
		}
		// The local subscription callback (startForwarding) picks up
		// every published message, including this replica's own, and
		// forwards it to local watchers. No duplicate delivery.
	}
}

// Close releases the NATS connection.
func (n *NATSFanout) Close() {
	if n.sub != nil {
		_ = n.sub.Unsubscribe()
	}
	n.nc.Close()
}

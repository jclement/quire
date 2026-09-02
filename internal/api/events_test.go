// The SSE broadcaster sits on the indexer's hot path: every applied change
// calls Publish. If that ever blocks — one slow tab, one tab that went away
// without unsubscribing — indexing stops and the whole app appears to freeze.
// Non-blocking fan-out is therefore a safety property, not an optimisation,
// and it had no test.
package api

import (
	"context"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jclement/quire/internal/index"
)

func TestBroadcastReachesEverySubscriber(t *testing.T) {
	b := NewBroadcaster()
	first, second := b.subscribe(), b.subscribe()
	t.Cleanup(func() { b.unsubscribe(first); b.unsubscribe(second) })

	b.Publish(index.Event{Path: "notes/x.md", Action: "upsert"})

	for name, ch := range map[string]chan index.Event{"first": first, "second": second} {
		select {
		case ev := <-ch:
			if ev.Path != "notes/x.md" {
				t.Errorf("%s received %+v", name, ev)
			}
		case <-time.After(time.Second):
			t.Errorf("%s received nothing", name)
		}
	}
}

// TestPublishNeverBlocks is the important one. A client that stops reading
// fills its buffer; Publish must drop for that client and keep going rather
// than wedge the indexer behind it.
func TestPublishNeverBlocks(t *testing.T) {
	b := NewBroadcaster()
	stalled := b.subscribe() // never read from
	healthy := b.subscribe()
	t.Cleanup(func() { b.unsubscribe(stalled); b.unsubscribe(healthy) })

	done := make(chan struct{})
	go func() {
		defer close(done)
		// Far more than the channel buffer, so the stalled client is
		// definitely full well before the end.
		for range 1000 {
			b.Publish(index.Event{Path: "notes/x.md", Action: "upsert"})
			<-healthy // keep the healthy client draining
		}
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		// Drain the stalled client so the blocked publisher can finish and
		// the test binary can exit; otherwise a regression here shows up as
		// the whole package timing out 30s later rather than as this
		// message.
		go func() {
			for range stalled {
			}
		}()
		t.Fatal("Publish blocked on a client that stopped reading — this would freeze indexing")
	}
}

func TestUnsubscribeStopsDelivery(t *testing.T) {
	b := NewBroadcaster()
	ch := b.subscribe()
	b.unsubscribe(ch)

	b.Publish(index.Event{Path: "notes/x.md", Action: "upsert"})

	select {
	case ev := <-ch:
		t.Errorf("received %+v after unsubscribing", ev)
	default:
	}
	// And the client is gone from the map, so a long-lived server does not
	// accumulate channels for tabs that closed.
	b.mu.Lock()
	remaining := len(b.clients)
	b.mu.Unlock()
	if remaining != 0 {
		t.Errorf("%d clients still registered after unsubscribe", remaining)
	}
}

// TestConcurrentPublishAndSubscribe is here for `go test -race`: the indexer
// publishes from its own goroutine while tabs come and go.
func TestConcurrentPublishAndSubscribe(t *testing.T) {
	b := NewBroadcaster()
	var wg sync.WaitGroup

	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				b.Publish(index.Event{Path: "notes/x.md", Action: "upsert"})
			}
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				ch := b.subscribe()
				b.unsubscribe(ch)
			}
		}()
	}
	wg.Wait()

	b.mu.Lock()
	remaining := len(b.clients)
	b.mu.Unlock()
	if remaining != 0 {
		t.Errorf("%d clients leaked", remaining)
	}
}

// TestEventStreamDisconnectsCleanly: when a tab goes away the handler must
// return and drop its subscription, or every closed tab leaks a channel and
// a goroutine for the life of the process.
func TestEventStreamDisconnectsCleanly(t *testing.T) {
	broadcaster := NewBroadcaster()
	s := &Server{Events: broadcaster, Version: "test"}

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest("GET", "/api/v1/events", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.handleEvents(rec, req)
	}()

	// Wait for the subscription to appear, then publish something.
	deadline := time.After(2 * time.Second)
	for {
		broadcaster.mu.Lock()
		n := len(broadcaster.clients)
		broadcaster.mu.Unlock()
		if n == 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("the stream never subscribed")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	broadcaster.Publish(index.Event{Path: "notes/x.md", Action: "upsert"})
	time.Sleep(50 * time.Millisecond)

	cancel() // the tab closes
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handleEvents did not return when the client disconnected")
	}

	broadcaster.mu.Lock()
	remaining := len(broadcaster.clients)
	broadcaster.mu.Unlock()
	if remaining != 0 {
		t.Errorf("%d clients still registered after disconnect", remaining)
	}

	body := rec.Body.String()
	if !strings.Contains(body, ": connected") {
		t.Error("the stream should announce itself before any event")
	}
	if !strings.Contains(body, "event: doc") || !strings.Contains(body, "notes/x.md") {
		t.Errorf("the published event did not reach the stream: %q", body)
	}
}

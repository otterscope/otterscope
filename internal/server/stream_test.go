package server

import (
	"sync"
	"testing"
)

func TestHubBroadcast(t *testing.T) {
	h := newHub()
	a := h.subscribe()
	b := h.subscribe()

	h.broadcast()
	for _, ch := range []chan struct{}{a, b} {
		select {
		case <-ch:
		default:
			t.Fatal("subscriber did not receive tick")
		}
	}

	// Coalescing: two broadcasts with no drain leave exactly one pending.
	h.broadcast()
	h.broadcast()
	<-a
	select {
	case <-a:
		t.Fatal("ticks should coalesce to one")
	default:
	}

	// Unsubscribe closes the channel and broadcast no longer panics.
	h.unsubscribe(a)
	h.unsubscribe(b)
	h.broadcast()

	// Idempotent unsubscribe is safe.
	h.unsubscribe(a)
}

func TestHubConcurrent(t *testing.T) {
	h := newHub()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ch := h.subscribe()
			h.broadcast()
			<-ch
			h.unsubscribe(ch)
		}()
	}
	for i := 0; i < 50; i++ {
		h.broadcast()
	}
	wg.Wait()
}

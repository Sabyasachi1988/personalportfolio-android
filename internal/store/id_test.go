package store

import (
	"sync"
	"testing"
)

func TestNewID_UniqueEvenInTightLoop(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 10000; i++ {
		id := NewID("ast")
		if seen[id] {
			t.Fatalf("duplicate ID generated at iteration %d: %s", i, id)
		}
		seen[id] = true
	}
}

// TestNewID_UniqueUnderConcurrentBurst hammers NewID from many goroutines
// at once to maximise the chance of hitting the same clock tick - the
// exact scenario that caused duplicate Asset IDs during CAS import commit.
func TestNewID_UniqueUnderConcurrentBurst(t *testing.T) {
	const goroutines = 50
	const perGoroutine = 200

	var mu sync.Mutex
	seen := make(map[string]bool)
	var wg sync.WaitGroup

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				id := NewID("ast")
				mu.Lock()
				if seen[id] {
					t.Errorf("duplicate ID generated: %s", id)
				}
				seen[id] = true
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if len(seen) != goroutines*perGoroutine {
		t.Errorf("expected %d unique IDs, got %d", goroutines*perGoroutine, len(seen))
	}
}

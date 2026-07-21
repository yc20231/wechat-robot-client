package dedup

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestMemoryCacheAllowsOnlyOneConcurrentFirstSeen(t *testing.T) {
	cache := NewMemoryCache(time.Hour)
	start := make(chan struct{})
	var firstSeen int32
	var wait sync.WaitGroup
	for range 100 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			if !cache.Seen("same-message") {
				atomic.AddInt32(&firstSeen, 1)
			}
		}()
	}
	close(start)
	wait.Wait()
	if firstSeen != 1 {
		t.Fatalf("first-seen count = %d, want 1", firstSeen)
	}
}

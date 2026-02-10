package federation

import (
	"sync"
	"time"
)

// HLC implements a Hybrid Logical Clock for causal ordering of events across
// federated instances. It combines a physical wall clock with a logical counter
// to maintain happens-before ordering without a central coordinator.
type HLC struct {
	mu      sync.Mutex
	wallMs  int64 // physical time in milliseconds
	counter uint32
}

// HLCTimestamp represents a single HLC timestamp with wall time and counter.
type HLCTimestamp struct {
	WallMs  int64  `json:"wall_ms"`
	Counter uint32 `json:"counter"`
}

// NewHLC creates a new Hybrid Logical Clock.
func NewHLC() *HLC {
	return &HLC{}
}

// Now generates a new HLC timestamp. The timestamp is guaranteed to be
// monotonically increasing even if the wall clock hasn't advanced.
func (h *HLC) Now() HLCTimestamp {
	h.mu.Lock()
	defer h.mu.Unlock()

	physMs := time.Now().UnixMilli()

	if physMs > h.wallMs {
		h.wallMs = physMs
		h.counter = 0
	} else {
		h.counter++
	}

	return HLCTimestamp{
		WallMs:  h.wallMs,
		Counter: h.counter,
	}
}

// Update merges a received remote HLC timestamp with the local clock, ensuring
// the local clock is at least as advanced as the remote timestamp.
func (h *HLC) Update(remote HLCTimestamp) HLCTimestamp {
	h.mu.Lock()
	defer h.mu.Unlock()

	physMs := time.Now().UnixMilli()

	if physMs > h.wallMs && physMs > remote.WallMs {
		h.wallMs = physMs
		h.counter = 0
	} else if remote.WallMs > h.wallMs {
		h.wallMs = remote.WallMs
		h.counter = remote.Counter + 1
	} else if h.wallMs == remote.WallMs {
		if remote.Counter > h.counter {
			h.counter = remote.Counter + 1
		} else {
			h.counter++
		}
	} else {
		// h.wallMs > remote.WallMs — local is ahead
		h.counter++
	}

	return HLCTimestamp{
		WallMs:  h.wallMs,
		Counter: h.counter,
	}
}

// Before returns true if timestamp a happened before timestamp b.
func (a HLCTimestamp) Before(b HLCTimestamp) bool {
	if a.WallMs != b.WallMs {
		return a.WallMs < b.WallMs
	}
	return a.Counter < b.Counter
}

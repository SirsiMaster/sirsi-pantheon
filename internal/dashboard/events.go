package dashboard

import (
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Event is a single SSE event pushed to the dashboard.
type Event struct {
	ID   uint64 `json:"id"`
	Type string `json:"type"` // "output", "complete", "error", "status"
	Data string `json:"data"` // JSON payload
}

// EventBuffer is a thread-safe ring buffer for SSE events.
// Multiple HTTP clients can read concurrently; each tracks its own offset.
type EventBuffer struct {
	mu   sync.RWMutex
	buf  []Event
	cap  int
	head int    // next write position (mod cap)
	seq  uint64 // monotonic sequence — each Push increments this
}

// NewEventBuffer creates a ring buffer with the given capacity.
func NewEventBuffer(capacity int) *EventBuffer {
	if capacity <= 0 {
		capacity = 256
	}
	return &EventBuffer{
		buf: make([]Event, capacity),
		cap: capacity,
	}
}

// Push appends an event to the buffer. Thread-safe.
func (eb *EventBuffer) Push(e Event) {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	e.ID = eb.seq
	eb.buf[eb.head] = e
	eb.head = (eb.head + 1) % eb.cap
	eb.seq++
}

// Since returns all events with ID >= sinceID. Returns at most cap events.
// Thread-safe for concurrent readers.
func (eb *EventBuffer) Since(sinceID uint64) []Event {
	eb.mu.RLock()
	defer eb.mu.RUnlock()

	if eb.seq == 0 {
		return nil
	}

	// Clamp sinceID to the oldest available event.
	oldest := uint64(0)
	if eb.seq > uint64(eb.cap) {
		oldest = eb.seq - uint64(eb.cap)
	}
	if sinceID < oldest {
		sinceID = oldest
	}

	count := int(eb.seq - sinceID)
	if count <= 0 {
		return nil
	}
	if count > eb.cap {
		count = eb.cap
	}

	result := make([]Event, 0, count)
	for i := sinceID; i < eb.seq; i++ {
		idx := int(i) % eb.cap
		result = append(result, eb.buf[idx])
	}
	return result
}

// Seq returns the current sequence number (next ID to be assigned).
func (eb *EventBuffer) Seq() uint64 {
	eb.mu.RLock()
	defer eb.mu.RUnlock()
	return eb.seq
}

// apiEvents serves an SSE stream from the event buffer.
// The client connects, receives all events since Last-Event-ID (or since=N param),
// then polls every second for new events until the client disconnects.
func (s *Server) apiEvents(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Events == nil {
		writeError(w, "event stream not available", http.StatusServiceUnavailable)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// Determine starting sequence.
	var sinceID uint64
	if lastID := r.Header.Get("Last-Event-ID"); lastID != "" {
		if n, err := strconv.ParseUint(lastID, 10, 64); err == nil {
			sinceID = n + 1
		}
	} else if v := r.URL.Query().Get("since"); v != "" {
		if n, err := strconv.ParseUint(v, 10, 64); err == nil {
			sinceID = n
		}
	}

	ctx := r.Context()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		events := s.cfg.Events.Since(sinceID)
		for _, e := range events {
			fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", e.ID, e.Type, e.Data)
			sinceID = e.ID + 1
		}
		if len(events) > 0 {
			flusher.Flush()
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// continue polling
		}
	}
}

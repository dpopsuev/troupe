// memlog.go — in-memory EventLog implementation.
package signal

import (
	"sync"
	"time"
)

var _ EventLog = (*MemLog)(nil)

// MemLog is a thread-safe, append-only in-memory event log.
type MemLog struct {
	mu     sync.Mutex
	events []Event
	hooks  []func(Event)
}

// NewMemLog creates an empty in-memory event log.
func NewMemLog() *MemLog {
	return &MemLog{}
}

func (l *MemLog) Emit(e Event) int {
	l.mu.Lock()
	defer l.mu.Unlock()

	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now().UTC()
	}
	idx := len(l.events)
	l.events = append(l.events, e)
	for _, fn := range l.hooks {
		fn(e)
	}
	return idx
}

func (l *MemLog) Since(index int) []Event {
	l.mu.Lock()
	defer l.mu.Unlock()

	if index < 0 {
		index = 0
	}
	if index >= len(l.events) {
		return nil
	}
	out := make([]Event, len(l.events)-index)
	copy(out, l.events[index:])
	return out
}

func (l *MemLog) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.events)
}

func (l *MemLog) OnEmit(fn func(Event)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.hooks = append(l.hooks, fn)
}

// ByTraceID returns all events with the given trace ID, in emission order.
func (l *MemLog) ByTraceID(traceID string) []Event {
	l.mu.Lock()
	defer l.mu.Unlock()

	var result []Event
	for _, e := range l.events {
		if e.TraceID == traceID {
			result = append(result, e)
		}
	}
	return result
}

// Bus returns a backward-compatible Bus adapter over this EventLog.
// Emits Signal as Data payload on Event. Reads convert back.
// Use this for consumers that still expect signal.Bus.
func (l *MemLog) Bus() *busAdapter {
	return &busAdapter{log: l}
}

// busAdapter wraps EventLog as the legacy Bus interface.
type busAdapter struct {
	log *MemLog
}

func (a *busAdapter) Emit(s *Signal) int {
	return a.log.Emit(Event{
		Source: s.Agent,
		Kind:   s.Event,
		Data:   *s,
	})
}

func (a *busAdapter) Since(index int) []Signal {
	events := a.log.Since(index)
	if events == nil {
		return nil
	}
	signals := make([]Signal, len(events))
	for i, e := range events {
		if s, ok := e.Data.(Signal); ok {
			signals[i] = s
		}
	}
	return signals
}

func (a *busAdapter) Len() int { return a.log.Len() }

func (a *busAdapter) OnEmit(fn func(Signal)) {
	a.log.OnEmit(func(e Event) {
		if s, ok := e.Data.(Signal); ok {
			fn(s)
		}
	})
}

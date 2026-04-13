// durable.go — DurableEventLog: EventLog backed by a pluggable EventStore.
//
// Wraps MemLog (in-memory, fast queries) + EventStore (durable persistence).
// On Emit: append to both. On Replay: read from store into memory.
// Satisfies EventLog interface. Legacy Bus consumers use Bus() adapter.
//
// TRP-TSK-78, TRP-GOL-13
package signal

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
)

var _ EventLog = (*DurableEventLog)(nil)

// DurableEventLog wraps a MemLog with persistent tee-write to an EventStore.
// On crash recovery, Replay() reads the store and populates the in-memory log.
type DurableEventLog struct {
	inner *MemLog
	store EventStore
	log   *slog.Logger
	mu    sync.Mutex
}

// NewDurableEventLog creates a durable event log backed by the given store.
func NewDurableEventLog(store EventStore) *DurableEventLog {
	d := &DurableEventLog{
		inner: NewMemLog(),
		store: store,
	}
	return d
}

// NewDurableJSONLines creates a DurableEventLog backed by a JSON-Lines file.
// Convenience constructor for the common case.
func NewDurableJSONLines(path string) (*DurableEventLog, error) {
	store, err := NewJSONLinesStore(path)
	if err != nil {
		return nil, err
	}
	return NewDurableEventLog(store), nil
}

// WithLogger sets the structured logger for ORANGE/YELLOW instrumentation.
func (d *DurableEventLog) WithLogger(l *slog.Logger) *DurableEventLog {
	d.log = l
	return d
}

// Emit appends an event to both the in-memory log and the durable store.
func (d *DurableEventLog) Emit(e Event) int {
	idx := d.inner.Emit(e)

	d.mu.Lock()
	defer d.mu.Unlock()
	if d.store != nil {
		events := d.inner.Since(idx)
		if len(events) > 0 {
			if err := d.store.Append(events[0]); err != nil && d.log != nil {
				d.log.WarnContext(context.Background(), "event store append failed",
					slog.String("operation", "append"),
					slog.String("error", err.Error()),
				)
			}
		}
	}
	return idx
}

// Since returns events from index onward.
func (d *DurableEventLog) Since(index int) []Event {
	return d.inner.Since(index)
}

// Len returns the total number of events.
func (d *DurableEventLog) Len() int {
	return d.inner.Len()
}

// OnEmit registers a callback invoked on every Emit.
func (d *DurableEventLog) OnEmit(fn func(Event)) {
	d.inner.OnEmit(fn)
}

// ByTraceID returns all events with the given trace ID.
func (d *DurableEventLog) ByTraceID(traceID string) []Event {
	return d.inner.ByTraceID(traceID)
}

// Replay reads persisted events from the store into the in-memory log.
// Call once on startup before any new Emit calls.
func (d *DurableEventLog) Replay() (int, error) {
	events, err := d.store.ReadSince(0)
	if err != nil {
		if d.log != nil {
			d.log.WarnContext(context.Background(), "event store replay failed",
				slog.String("operation", "replay"),
				slog.String("error", err.Error()),
			)
		}
		return 0, fmt.Errorf("replay event store: %w", err)
	}
	for i := range events {
		d.inner.Emit(events[i])
	}
	if d.log != nil {
		d.log.InfoContext(context.Background(), "event store replay completed",
			slog.Int("count", len(events)),
		)
	}
	return len(events), nil
}

// Close flushes and closes the underlying store.
func (d *DurableEventLog) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.store != nil {
		if err := d.store.Close(); err != nil {
			if d.log != nil {
				d.log.WarnContext(context.Background(), "event store close failed",
					slog.String("operation", "close"),
					slog.String("error", err.Error()),
				)
			}
			return err
		}
	}
	return nil
}

// Store returns the underlying EventStore for inspection.
func (d *DurableEventLog) Store() EventStore { return d.store }

// Path returns the file path if backed by JSONLinesStore, empty otherwise.
func (d *DurableEventLog) Path() string {
	if jl, ok := d.store.(*JSONLinesStore); ok {
		return jl.Path()
	}
	return ""
}

// Bus returns a backward-compatible Bus adapter.
func (d *DurableEventLog) Bus() *busAdapter {
	return d.inner.Bus()
}

// --- Legacy DurableBus (deprecated, use DurableEventLog) ---

// DurableBus is the legacy name. Use DurableEventLog for new code.
//
// Deprecated: use NewDurableEventLog or NewDurableJSONLines.
type DurableBus = DurableEventLog

// NewDurableBus creates a DurableEventLog backed by a JSON-Lines file.
//
// Deprecated: use NewDurableJSONLines.
func NewDurableBus(path string) (*DurableEventLog, error) {
	return NewDurableJSONLines(path)
}

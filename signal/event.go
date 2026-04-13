// event.go — canonical Event type for the Aeon ecosystem.
//
// Envelope + typed payload (Data any). The envelope routes (ID, Kind,
// Source, Timestamp). The payload carries domain-specific data.
// Consumers type-switch on Data — no Meta maps, no string keys.
//
// Migration: Signal struct becomes a Data payload. Bus becomes EventLog.
package signal

import "time"

// Event is an immutable fact on the event log.
// The envelope has standard fields. Domain data goes in Data.
type Event struct {
	ID        string    `json:"id"`
	ParentID  string    `json:"parent_id,omitempty"`
	TraceID   string    `json:"trace_id,omitempty"`
	Timestamp time.Time `json:"ts"`
	Source    string    `json:"source"`
	Kind     string    `json:"kind"`
	Data     any       `json:"data,omitempty"`
}

// EventLog is an append-only log with sequential indexing.
// Implementations must be safe for concurrent use.
//
// This is the write side of CQRS. Projections (read side) subscribe
// via OnEmit and build query-optimized views.
type EventLog interface {
	// Emit appends an event and returns its sequential index (0-based).
	// Timestamp is set automatically if zero.
	Emit(e Event) int

	// Since returns all events from index onward (inclusive).
	// Returns nil if index >= Len(). Negative index returns all.
	// The returned slice is a copy.
	Since(index int) []Event

	// Len returns the total number of events emitted.
	Len() int

	// OnEmit registers a callback invoked on every Emit.
	OnEmit(fn func(Event))
}

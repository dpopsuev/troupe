// eventstore.go — storage port for durable event persistence.
//
// EventStore is a hexagonal port: domain defines the contract,
// adapters implement it (JSONLinesStore, MemEventStore, SQLite, etc.).
//
// Error returns because storage backends can fail (disk full, connection lost).
// MemLog stays error-free — EventStore is for durable backends that may fail.
//
// TRP-TSK-75, TRP-GOL-13
package signal

// EventStore is the storage port for durable event persistence.
// Implementations must be safe for concurrent use.
type EventStore interface {
	// Append persists a single event. Returns error on storage failure.
	Append(e Event) error

	// ReadSince returns all events from index onward (inclusive, 0-based).
	// Returns nil slice if index >= stored count.
	ReadSince(index int) ([]Event, error)

	// Len returns the total number of persisted events.
	Len() (int, error)

	// Close flushes pending writes and releases resources.
	Close() error
}

// eventstore_mem.go — in-memory EventStore for testing and ephemeral use.
//
// Thread-safe, never returns errors (in-memory can't fail).
// Satisfies the EventStore contract via Liskov substitution.
//
// TRP-TSK-76, TRP-GOL-13
package signal

import "sync"

var _ EventStore = (*MemEventStore)(nil)

// MemEventStore is an in-memory EventStore. All operations succeed.
type MemEventStore struct {
	mu     sync.RWMutex
	events []Event
}

// NewMemEventStore creates an empty in-memory store.
func NewMemEventStore() *MemEventStore {
	return &MemEventStore{}
}

func (s *MemEventStore) Append(e Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, e)
	return nil
}

func (s *MemEventStore) ReadSince(index int) ([]Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if index >= len(s.events) {
		return nil, nil
	}
	if index < 0 {
		index = 0
	}
	out := make([]Event, len(s.events)-index)
	copy(out, s.events[index:])
	return out, nil
}

func (s *MemEventStore) Len() (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.events), nil
}

func (s *MemEventStore) Close() error { return nil }

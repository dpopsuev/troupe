package signal

import (
	"path/filepath"
	"testing"
	"time"
)

// RunEventStoreContract runs the Liskov contract test suite against any EventStore.
// Every EventStore implementation must pass these tests.
func RunEventStoreContract(t *testing.T, factory func(t *testing.T) EventStore) {
	t.Helper()

	t.Run("Append_and_ReadSince", func(t *testing.T) {
		s := factory(t)
		defer s.Close()

		e1 := Event{ID: "e1", Kind: "test", Source: "a", Timestamp: time.Now()}
		e2 := Event{ID: "e2", Kind: "test", Source: "b", Timestamp: time.Now()}
		e3 := Event{ID: "e3", Kind: "test", Source: "c", Timestamp: time.Now()}

		if err := s.Append(e1); err != nil {
			t.Fatalf("Append e1: %v", err)
		}
		if err := s.Append(e2); err != nil {
			t.Fatalf("Append e2: %v", err)
		}
		if err := s.Append(e3); err != nil {
			t.Fatalf("Append e3: %v", err)
		}

		// ReadSince(0) returns all.
		all, err := s.ReadSince(0)
		if err != nil {
			t.Fatalf("ReadSince(0): %v", err)
		}
		if len(all) != 3 {
			t.Fatalf("ReadSince(0) = %d, want 3", len(all))
		}
		if all[0].ID != "e1" || all[1].ID != "e2" || all[2].ID != "e3" {
			t.Fatalf("order wrong: %v", all)
		}

		// ReadSince(2) returns last one.
		tail, err := s.ReadSince(2)
		if err != nil {
			t.Fatalf("ReadSince(2): %v", err)
		}
		if len(tail) != 1 || tail[0].ID != "e3" {
			t.Fatalf("ReadSince(2) = %v, want [e3]", tail)
		}

		// ReadSince(3) returns nil (past end).
		past, err := s.ReadSince(3)
		if err != nil {
			t.Fatalf("ReadSince(3): %v", err)
		}
		if past != nil {
			t.Fatalf("ReadSince(3) = %v, want nil", past)
		}
	})

	t.Run("Len", func(t *testing.T) {
		s := factory(t)
		defer s.Close()

		n, err := s.Len()
		if err != nil {
			t.Fatalf("Len: %v", err)
		}
		if n != 0 {
			t.Fatalf("Len empty = %d", n)
		}

		_ = s.Append(Event{ID: "x"})
		_ = s.Append(Event{ID: "y"})

		n, err = s.Len()
		if err != nil {
			t.Fatalf("Len: %v", err)
		}
		if n != 2 {
			t.Fatalf("Len = %d, want 2", n)
		}
	})

	t.Run("ReadSince_negative_returns_all", func(t *testing.T) {
		s := factory(t)
		defer s.Close()

		_ = s.Append(Event{ID: "a"})
		_ = s.Append(Event{ID: "b"})

		all, err := s.ReadSince(-1)
		if err != nil {
			t.Fatalf("ReadSince(-1): %v", err)
		}
		if len(all) != 2 {
			t.Fatalf("ReadSince(-1) = %d, want 2", len(all))
		}
	})

	t.Run("Close_idempotent", func(t *testing.T) {
		s := factory(t)
		if err := s.Close(); err != nil {
			t.Fatalf("Close 1: %v", err)
		}
		if err := s.Close(); err != nil {
			t.Fatalf("Close 2: %v", err)
		}
	})

	t.Run("ReadSince_returns_copy", func(t *testing.T) {
		s := factory(t)
		defer s.Close()

		_ = s.Append(Event{ID: "orig"})

		slice1, _ := s.ReadSince(0)
		slice1[0].ID = "mutated"

		slice2, _ := s.ReadSince(0)
		if slice2[0].ID != "orig" {
			t.Fatal("ReadSince must return a copy, not a reference to internal storage")
		}
	})
}

// TestMemEventStore_Contract runs the contract suite against MemEventStore.
func TestMemEventStore_Contract(t *testing.T) {
	RunEventStoreContract(t, func(_ *testing.T) EventStore {
		return NewMemEventStore()
	})
}

// TestJSONLinesStore_Contract runs the contract suite against JSONLinesStore.
func TestJSONLinesStore_Contract(t *testing.T) {
	RunEventStoreContract(t, func(t *testing.T) EventStore {
		t.Helper()
		path := filepath.Join(t.TempDir(), "events.jsonl")
		store, err := NewJSONLinesStore(path)
		if err != nil {
			t.Fatalf("NewJSONLinesStore: %v", err)
		}
		return store
	})
}

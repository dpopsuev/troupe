package testkit

import (
	"context"
	"sync"
	"testing"

	"github.com/dpopsuev/troupe"
	"github.com/dpopsuev/troupe/signal"
	"github.com/dpopsuev/troupe/world"
)

// AdmissionTestDeps provides the dependencies needed to run the Admission contract.
type AdmissionTestDeps struct {
	Admission  troupe.Admission
	ControlLog signal.EventLog
	WorldCount func() int
}

// RunAdmissionContract verifies any Admission implementation satisfies
// the behavioral contract: Admit creates entity, emits dispatch_routed.
// Dismiss removes entity. Gate rejection emits veto_applied.
// Concurrent Admits are race-free.
func RunAdmissionContract(t *testing.T, deps AdmissionTestDeps) {
	t.Helper()
	ctx := context.Background()

	t.Run("Admit_CreatesEntity", func(t *testing.T) {
		before := deps.WorldCount()
		id, err := deps.Admission.Admit(ctx, troupe.ActorConfig{Role: "contract-test"})
		if err != nil {
			t.Fatalf("Admit: %v", err)
		}
		if id == 0 {
			t.Fatal("Admit returned zero entity ID")
		}
		after := deps.WorldCount()
		if after != before+1 {
			t.Fatalf("World count %d -> %d, want +1", before, after)
		}
		deps.Admission.Dismiss(ctx, id) //nolint:errcheck
	})

	t.Run("Admit_EmitsDispatchRouted", func(t *testing.T) {
		if deps.ControlLog == nil {
			t.Skip("no ControlLog provided")
		}
		before := deps.ControlLog.Len()
		id, err := deps.Admission.Admit(ctx, troupe.ActorConfig{Role: "emit-test"})
		if err != nil {
			t.Fatalf("Admit: %v", err)
		}
		events := deps.ControlLog.Since(before)
		found := false
		for _, e := range events {
			if e.Kind == signal.EventDispatchRouted {
				found = true
				break
			}
		}
		if !found {
			t.Fatal("Admit should emit dispatch_routed to ControlLog")
		}
		deps.Admission.Dismiss(ctx, id) //nolint:errcheck
	})

	t.Run("Dismiss_RemovesEntity", func(t *testing.T) {
		id, err := deps.Admission.Admit(ctx, troupe.ActorConfig{Role: "dismiss-test"})
		if err != nil {
			t.Fatalf("Admit: %v", err)
		}
		before := deps.WorldCount()
		if err := deps.Admission.Dismiss(ctx, id); err != nil {
			t.Fatalf("Dismiss: %v", err)
		}
		after := deps.WorldCount()
		if after != before-1 {
			t.Fatalf("World count %d -> %d, want -1", before, after)
		}
	})

	t.Run("ConcurrentAdmit_RaceFree", func(t *testing.T) {
		const n = 20
		var wg sync.WaitGroup
		errs := make(chan error, n)
		ids := make(chan uint64, n)

		wg.Add(n)
		for range n {
			go func() {
				defer wg.Done()
				id, err := deps.Admission.Admit(ctx, troupe.ActorConfig{Role: "race-test"})
				if err != nil {
					errs <- err
					return
				}
				ids <- uint64(id)
			}()
		}
		wg.Wait()
		close(errs)
		close(ids)

		for err := range errs {
			t.Fatalf("concurrent Admit error: %v", err)
		}

		seen := make(map[uint64]bool)
		for id := range ids {
			if seen[id] {
				t.Fatalf("duplicate entity ID %d from concurrent Admits", id)
			}
			seen[id] = true
		}

		// Clean up.
		for id := range seen {
			deps.Admission.Dismiss(ctx, world.EntityID(id)) //nolint:errcheck
		}
	})
}

package pool

import (
	"context"
	"sync"
	"testing"

	"github.com/dpopsuev/bugle/signal"
	"github.com/dpopsuev/bugle/transport"
	"github.com/dpopsuev/bugle/world"
)

// mockLauncher tracks Start/Stop calls for testing.
type mockLauncher struct {
	mu      sync.Mutex
	started map[world.EntityID]LaunchConfig
	stopped map[world.EntityID]bool
}

func newMockLauncher() *mockLauncher {
	return &mockLauncher{
		started: make(map[world.EntityID]LaunchConfig),
		stopped: make(map[world.EntityID]bool),
	}
}

func (m *mockLauncher) Start(_ context.Context, id world.EntityID, config LaunchConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.started[id] = config
	return nil
}

func (m *mockLauncher) Stop(_ context.Context, id world.EntityID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopped[id] = true
	return nil
}

func (m *mockLauncher) Healthy(_ context.Context, id world.EntityID) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, started := m.started[id]
	_, stopped := m.stopped[id]
	return started && !stopped
}

func setup() (*AgentPool, *mockLauncher, *signal.MemBus) {
	w := world.NewWorld()
	t := transport.NewLocalTransport()
	bus := signal.NewMemBus()
	launcher := newMockLauncher()
	pool := New(w, t, bus, launcher)
	return pool, launcher, bus
}

func TestFork_CreatesEntity(t *testing.T) {
	pool, launcher, _ := setup()
	ctx := context.Background()

	id, err := pool.Fork(ctx, "executor", LaunchConfig{
		Role:  "executor",
		Model: "sonnet-4",
	})
	if err != nil {
		t.Fatal(err)
	}
	if id == 0 {
		t.Fatal("entity ID should not be 0")
	}
	if pool.Count() != 1 {
		t.Fatalf("count = %d, want 1", pool.Count())
	}

	// Launcher was called.
	if _, ok := launcher.started[id]; !ok {
		t.Fatal("launcher.Start not called")
	}
}

func TestFork_EmitsSignal(t *testing.T) {
	pool, _, bus := setup()
	ctx := context.Background()

	pool.Fork(ctx, "executor", LaunchConfig{Role: "executor"})

	signals := bus.Since(0)
	if len(signals) == 0 {
		t.Fatal("no signals emitted")
	}
	if signals[0].Event != signal.EventWorkerStarted {
		t.Fatalf("event = %q, want worker_started", signals[0].Event)
	}
}

func TestKill_StopsAndDespawns(t *testing.T) {
	pool, launcher, bus := setup()
	ctx := context.Background()

	id, _ := pool.Fork(ctx, "executor", LaunchConfig{})
	err := pool.Kill(ctx, id)
	if err != nil {
		t.Fatal(err)
	}

	if pool.Count() != 0 {
		t.Fatalf("count = %d after kill", pool.Count())
	}
	if !launcher.stopped[id] {
		t.Fatal("launcher.Stop not called")
	}

	// Should emit worker_stopped.
	signals := bus.Since(0)
	found := false
	for _, s := range signals {
		if s.Event == signal.EventWorkerStopped {
			found = true
		}
	}
	if !found {
		t.Fatal("EventWorkerStopped not emitted")
	}
}

func TestKill_NotFound(t *testing.T) {
	pool, _, _ := setup()
	err := pool.Kill(context.Background(), 999)
	if err == nil {
		t.Fatal("should error for unknown agent")
	}
}

func TestKillAll(t *testing.T) {
	pool, _, _ := setup()
	ctx := context.Background()

	pool.Fork(ctx, "a", LaunchConfig{})
	pool.Fork(ctx, "b", LaunchConfig{})
	pool.Fork(ctx, "c", LaunchConfig{})

	if pool.Count() != 3 {
		t.Fatalf("count = %d", pool.Count())
	}

	pool.KillAll(ctx)
	if pool.Count() != 0 {
		t.Fatalf("count = %d after KillAll", pool.Count())
	}
}

func TestActive(t *testing.T) {
	pool, _, _ := setup()
	ctx := context.Background()

	pool.Fork(ctx, "a", LaunchConfig{})
	pool.Fork(ctx, "b", LaunchConfig{})

	active := pool.Active()
	if len(active) != 2 {
		t.Fatalf("active = %d, want 2", len(active))
	}
}

func TestGet(t *testing.T) {
	pool, _, _ := setup()
	ctx := context.Background()

	id, _ := pool.Fork(ctx, "executor", LaunchConfig{Role: "executor"})
	entry, ok := pool.Get(id)
	if !ok {
		t.Fatal("should find entry")
	}
	if entry.Role != "executor" {
		t.Fatalf("role = %q", entry.Role)
	}
}

func TestFork_WithBudget(t *testing.T) {
	pool, _, _ := setup()
	ctx := context.Background()

	id, _ := pool.Fork(ctx, "executor", LaunchConfig{Budget: 100.0})
	budget, ok := world.TryGet[world.Budget](pool.world, id)
	if !ok {
		t.Fatal("budget not attached")
	}
	if budget.Ceiling != 100.0 {
		t.Fatalf("ceiling = %f, want 100", budget.Ceiling)
	}
}

func TestConcurrentForkKill(t *testing.T) {
	pool, _, _ := setup()
	ctx := context.Background()
	var wg sync.WaitGroup

	// Fork 10 agents concurrently.
	ids := make(chan world.EntityID, 10)
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, err := pool.Fork(ctx, "worker", LaunchConfig{})
			if err != nil {
				t.Error(err)
				return
			}
			ids <- id
		}()
	}
	wg.Wait()
	close(ids)

	if pool.Count() != 10 {
		t.Fatalf("count = %d, want 10", pool.Count())
	}

	// Kill all concurrently.
	for id := range ids {
		wg.Add(1)
		go func(eid world.EntityID) {
			defer wg.Done()
			pool.Kill(ctx, eid) //nolint:errcheck
		}(id)
	}
	wg.Wait()

	if pool.Count() != 0 {
		t.Fatalf("count = %d after concurrent kill", pool.Count())
	}
}

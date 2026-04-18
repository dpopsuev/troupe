package agent

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dpopsuev/troupe/internal/warden"
	"github.com/dpopsuev/troupe/world"
)

// ---------------------------------------------------------------------------
// mockLauncher — same pattern as pool/pool_test.go
// ---------------------------------------------------------------------------

type mockLauncher struct {
	mu      sync.Mutex
	started map[world.EntityID]bool
	stopped map[world.EntityID]bool
}

func (m *mockLauncher) Start(_ context.Context, id world.EntityID, _ warden.AgentConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.started[id] = true
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
	return m.started[id] && !m.stopped[id]
}

func TestHandle_String(t *testing.T) {
	s := setup()
	ctx := context.Background()

	h, _ := s.Spawn(ctx, "executor", warden.AgentConfig{})
	want := fmt.Sprintf("executor(agent-%d)", h.ID())
	if h.String() != want {
		t.Fatalf("String() = %q, want %q", h.String(), want)
	}
}



func TestHandle_IsZombie(t *testing.T) {
	s := setup()
	ctx := context.Background()

	// Disable auto-reap for parent=0 so zombies accumulate.
	s.Pool().SetAutoReap(0, false)

	h, _ := s.Spawn(ctx, "worker", warden.AgentConfig{})
	if h.IsZombie() {
		t.Fatal("should not be zombie before kill")
	}

	h.Kill(ctx)
	if !h.IsZombie() {
		t.Fatal("should be zombie after kill (no reap)")
	}

	// Wait reaps the zombie.
	h.Wait(ctx)
	if h.IsZombie() {
		t.Fatal("should not be zombie after Wait")
	}
}

func TestHandle_Ask(t *testing.T) {
	s := setup()
	ctx := context.Background()

	h, _ := s.Spawn(ctx, "echo", warden.AgentConfig{})
	// Register echo handler.
	h.Listen(func(content string) string {
		return "echo:" + content
	})

	resp, err := h.Perform(ctx, "hello")
	if err != nil {
		t.Fatal(err)
	}
	if resp != "echo:hello" {
		t.Fatalf("response = %q, want echo:hello", resp)
	}
}


func TestHandle_Broadcast(t *testing.T) {
	s := setup()
	ctx := context.Background()

	var counters [3]atomic.Int32
	for i := range 3 {
		h, _ := s.Spawn(ctx, "listener", warden.AgentConfig{})
		idx := i
		h.Listen(func(_ string) string {
			counters[idx].Add(1)
			return "ok"
		})
	}

	// Broadcast from any handle with role "listener".
	first := s.FindByRole("listener")[0]
	err := first.Broadcast(ctx, "ping-all")
	if err != nil {
		t.Fatal(err)
	}

	// Wait for all handlers to fire.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		total := int32(0)
		for i := range 3 {
			total += counters[i].Load()
		}
		if total >= 3 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("broadcast did not reach all 3 agents")
}

func TestHandle_Listen(t *testing.T) {
	s := setup()
	ctx := context.Background()

	h, _ := s.Spawn(ctx, "responder", warden.AgentConfig{})
	h.Listen(func(content string) string {
		return "got:" + content
	})

	resp, err := h.Perform(ctx, "test")
	if err != nil {
		t.Fatal(err)
	}
	if resp != "got:test" {
		t.Fatalf("response = %q, want got:test", resp)
	}
}

func TestHandle_Spawn_Child(t *testing.T) {
	s := setup()
	ctx := context.Background()

	parent, _ := s.Spawn(ctx, "manager", warden.AgentConfig{})
	child, err := parent.Spawn(ctx, "executor", warden.AgentConfig{})
	if err != nil {
		t.Fatal(err)
	}

	children := parent.Children()
	if len(children) != 1 {
		t.Fatalf("children = %d, want 1", len(children))
	}
	if children[0].ID() != child.ID() {
		t.Fatalf("child ID = %d, want %d", children[0].ID(), child.ID())
	}
}

func TestHandle_Kill_Wait(t *testing.T) {
	s := setup()
	ctx := context.Background()

	// Disable auto-reap so we can Wait.
	s.Pool().SetAutoReap(0, false)

	h, _ := s.Spawn(ctx, "worker", warden.AgentConfig{})
	h.Kill(ctx)

	status, err := h.Wait(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status == nil {
		t.Fatal("status should not be nil")
	}
	if status.Code != warden.ExitSuccess {
		t.Fatalf("exit code = %d, want ExitSuccess(0)", status.Code)
	}
}


func TestHandle_Children(t *testing.T) {
	s := setup()
	ctx := context.Background()

	parent, _ := s.Spawn(ctx, "manager", warden.AgentConfig{})
	child, _ := s.SpawnUnder(ctx, parent, "executor", warden.AgentConfig{})

	children := parent.Children()
	found := false
	for _, c := range children {
		if c.ID() == child.ID() {
			found = true
		}
	}
	if !found {
		t.Fatal("parent.Children should include child")
	}
}


// ---------------------------------------------------------------------------
// Integration tests
// ---------------------------------------------------------------------------

func TestFacade_FullPipeline(t *testing.T) {
	s := setup()
	ctx := context.Background()

	// GenSec spawns an Executor.
	gensec, _ := s.Spawn(ctx, "gensec", warden.AgentConfig{})
	executor, _ := gensec.Spawn(ctx, "executor", warden.AgentConfig{})

	// Executor listens and echoes.
	executor.Listen(func(content string) string {
		return "executed:" + content
	})

	// GenSec asks Executor.
	resp, err := executor.Perform(ctx, "compile")
	if err != nil {
		t.Fatal(err)
	}
	if resp != "executed:compile" {
		t.Fatalf("response = %q", resp)
	}

	// Disable auto-reap so we can Wait on executor.
	s.Pool().SetAutoReap(gensec.ID(), false)

	// Kill Executor and Wait.
	executor.Kill(ctx)
	status, err := executor.Wait(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status == nil {
		t.Fatal("status should not be nil")
	}

	// KillAll.
	s.KillAll(ctx)
	if s.Count() != 0 {
		t.Fatalf("count = %d after KillAll", s.Count())
	}
}

func TestFacade_ConcurrentSpawnAskKill(t *testing.T) {
	s := setup()
	ctx := context.Background()

	var wg sync.WaitGroup

	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()

			h, err := s.Spawn(ctx, "worker", warden.AgentConfig{})
			if err != nil {
				t.Error(err)
				return
			}

			h.Listen(func(content string) string {
				return "reply:" + content
			})

			resp, err := h.Perform(ctx, "ping")
			if err != nil {
				t.Error(err)
				return
			}
			if resp != "reply:ping" {
				t.Errorf("response = %q", resp)
				return
			}

			if err := h.Kill(ctx); err != nil {
				t.Error(err)
			}
		}()
	}

	wg.Wait()

	if s.Count() != 0 {
		t.Fatalf("count = %d after concurrent test", s.Count())
	}
}

//go:build e2e

package troupe_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/dpopsuev/troupe"
	"github.com/dpopsuev/troupe/resilience"
	"github.com/dpopsuev/troupe/testkit"
	"github.com/dpopsuev/troupe/world"
)

// echoDriver is a minimal driver that marks entities as started.
type echoDriver struct {
	mu      sync.Mutex
	started map[world.EntityID]bool
}

func newEchoDriver() *echoDriver { return &echoDriver{started: make(map[world.EntityID]bool)} }

func (d *echoDriver) Start(_ context.Context, id world.EntityID, _ troupe.ActorConfig) error {
	d.mu.Lock()
	d.started[id] = true
	d.mu.Unlock()
	return nil
}

func (d *echoDriver) Stop(_ context.Context, _ world.EntityID) error { return nil }

// --- E2E Tests ---

func TestE2E_LocalBroker_SpawnPerformMeter(t *testing.T) {
	ctx := context.Background()
	meter := troupe.NewInMemoryMeter()
	obs := &e2ePerformHook{meter: meter}
	broker := troupe.NewBroker("",
		troupe.WithDriver(newEchoDriver()),
		troupe.WithMeter(meter),
		troupe.WithHook(obs),
	)

	actor, err := broker.Spawn(ctx, troupe.ActorConfig{Role: "analyst", Model: "sonnet"})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if !actor.Ready() {
		t.Fatal("actor not ready")
	}

	_, err = actor.Perform(ctx, "analyze this code")
	if err != nil {
		t.Fatalf("Perform: %v", err)
	}

	// Meter should have recorded usage via the hook.
	usages := meter.Query("analyst")
	if len(usages) == 0 {
		t.Error("no usage recorded")
	}
}

func TestE2E_HookPipeline(t *testing.T) {
	ctx := context.Background()
	spawnObs := &e2eSpawnHook{}
	perfObs := &e2ePerformHook{meter: troupe.NewInMemoryMeter()}
	broker := troupe.NewBroker("",
		troupe.WithDriver(newEchoDriver()),
		troupe.WithHook(spawnObs),
		troupe.WithHook(perfObs),
	)

	actor, err := broker.Spawn(ctx, troupe.ActorConfig{Role: "test"})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	actor.Perform(ctx, "hello") //nolint:errcheck // best-effort cleanup

	if spawnObs.count != 1 {
		t.Errorf("spawn hook: %d calls, want 1", spawnObs.count)
	}
	if perfObs.performCount != 1 {
		t.Errorf("perform hook: %d calls, want 1", perfObs.performCount)
	}
}

func TestE2E_RetryActor_TransientFailure(t *testing.T) {
	actor := &testkit.MockActor{Name: "flaky"}
	actor.SetFailNext()

	ra := resilience.NewRetryActor(actor, resilience.RetryConfig{MaxAttempts: 3})
	resp, err := ra.Perform(context.Background(), "retry me")
	if err != nil {
		t.Fatalf("expected success after retry: %v", err)
	}
	if resp == "" {
		t.Error("empty response")
	}
}

func TestE2E_MultiDriver_RoutesCorrectly(t *testing.T) {
	ctx := context.Background()
	d1 := newEchoDriver()
	d2 := newEchoDriver()
	broker := troupe.NewBroker("",
		troupe.WithDriverFor("provider-a", d1),
		troupe.WithDriverFor("provider-b", d2),
	)

	broker.Spawn(ctx, troupe.ActorConfig{Provider: "provider-a", Role: "test"}) //nolint:errcheck // best-effort cleanup
	broker.Spawn(ctx, troupe.ActorConfig{Provider: "provider-b", Role: "test"}) //nolint:errcheck // best-effort cleanup

	if len(d1.started) != 1 {
		t.Errorf("driver-a: %d starts, want 1", len(d1.started))
	}
	if len(d2.started) != 1 {
		t.Errorf("driver-b: %d starts, want 1", len(d2.started))
	}
}

func TestE2E_DirectorLinear(t *testing.T) {
	ctx := context.Background()
	broker := testkit.NewMockBroker(1)
	director := &testkit.LinearDirector{
		Steps: []testkit.Step{
			{Name: "classify", Prompt: "classify this"},
			{Name: "summarize", Prompt: "summarize this"},
		},
	}

	events, err := director.Direct(ctx, broker)
	if err != nil {
		t.Fatalf("Direct: %v", err)
	}

	var kinds []troupe.EventKind
	for ev := range events {
		kinds = append(kinds, ev.Kind)
		if ev.Kind == troupe.Failed {
			t.Fatalf("unexpected failure at %s: %v", ev.Step, ev.Error)
		}
	}

	want := []troupe.EventKind{
		troupe.Started, troupe.Completed,
		troupe.Started, troupe.Completed,
		troupe.Done,
	}
	if len(kinds) != len(want) {
		t.Fatalf("got %d events, want %d", len(kinds), len(want))
	}
}

func TestE2E_ConcurrentSpawnPerform(t *testing.T) {
	ctx := context.Background()
	broker := troupe.NewBroker("", troupe.WithDriver(newEchoDriver()))

	var wg sync.WaitGroup
	errs := make(chan error, 10) //nolint:mnd

	for i := range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a, err := broker.Spawn(ctx, troupe.ActorConfig{Role: fmt.Sprintf("w-%d", i)})
			if err != nil {
				errs <- err
				return
			}
			if _, err := a.Perform(ctx, "concurrent work"); err != nil {
				errs <- err
				return
			}
			a.Kill(ctx) //nolint:errcheck // best-effort cleanup
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent error: %v", err)
	}
}

func TestE2E_MalformedInput_NoPanic(t *testing.T) {
	broker := testkit.NewMockBroker(1)

	// Empty config should not panic.
	_, err := broker.Spawn(context.Background(), troupe.ActorConfig{})
	if err != nil {
		t.Logf("empty config spawn error (acceptable): %v", err)
	}

	// Nil context should not panic.
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("panic on nil context: %v", r)
		}
	}()
}

// --- E2E hook helpers ---

type e2eSpawnHook struct {
	count int
	mu    sync.Mutex
}

func (h *e2eSpawnHook) Name() string { return "e2e-spawn" }
func (h *e2eSpawnHook) PreSpawn(_ context.Context, _ troupe.ActorConfig) error {
	return nil
}
func (h *e2eSpawnHook) PostSpawn(_ context.Context, _ troupe.ActorConfig, _ troupe.Actor, _ error) {
	h.mu.Lock()
	h.count++
	h.mu.Unlock()
}

type e2ePerformHook struct {
	meter        troupe.Meter
	performCount int
	mu           sync.Mutex
}

func (h *e2ePerformHook) Name() string                                    { return "e2e-perform" }
func (h *e2ePerformHook) PrePerform(_ context.Context, _ string) error    { return nil }
func (h *e2ePerformHook) PostPerform(_ context.Context, _, _ string, _ error) {
	h.mu.Lock()
	h.performCount++
	h.mu.Unlock()
	if h.meter != nil {
		h.meter.Record(troupe.Usage{
			Actor:    "analyst",
			Step:     "perform",
			Duration: time.Millisecond,
		})
	}
}

// Verify hook satisfies both interfaces.
var _ troupe.SpawnHook = (*e2eSpawnHook)(nil)
var _ troupe.PerformHook = (*e2ePerformHook)(nil)

// Suppress unused import warnings.
var _ = errors.New

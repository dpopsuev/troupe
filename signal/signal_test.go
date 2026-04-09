package signal_test

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/dpopsuev/troupe/signal"
)

// ---------------------------------------------------------------------------
// MemBus
// ---------------------------------------------------------------------------

func TestMemBus_EmitAndSince(t *testing.T) {
	bus := signal.NewMemBus()
	if bus.Len() != 0 {
		t.Fatalf("new bus len: got %d, want 0", bus.Len())
	}

	idx := bus.Emit(&signal.Signal{
		Event:  "test",
		Agent:  "agent1",
		CaseID: "C1",
		Step:   "F0",
		Meta:   map[string]string{"k": "v"},
	})
	if idx != 0 {
		t.Fatalf("first Emit index: got %d, want 0", idx)
	}
	if bus.Len() != 1 {
		t.Fatalf("after emit len: got %d, want 1", bus.Len())
	}

	sigs := bus.Since(0)
	if len(sigs) != 1 {
		t.Fatalf("Since(0) len: got %d, want 1", len(sigs))
	}
	if sigs[0].Event != "test" || sigs[0].Agent != "agent1" || sigs[0].CaseID != "C1" || sigs[0].Step != "F0" {
		t.Errorf("signal: event=%q agent=%q case_id=%q step=%q",
			sigs[0].Event, sigs[0].Agent, sigs[0].CaseID, sigs[0].Step)
	}
	if sigs[0].Meta["k"] != "v" {
		t.Errorf("meta: got %v", sigs[0].Meta)
	}
	if sigs[0].Timestamp == "" {
		t.Error("timestamp should be auto-set")
	}
}

func TestMemBus_EmitPreservesTimestamp(t *testing.T) {
	bus := signal.NewMemBus()
	bus.Emit(&signal.Signal{Event: "a", Timestamp: "2026-01-01T00:00:00Z"})
	sigs := bus.Since(0)
	if sigs[0].Timestamp != "2026-01-01T00:00:00Z" {
		t.Errorf("timestamp was overwritten: got %q", sigs[0].Timestamp)
	}
}

func TestMemBus_Since(t *testing.T) {
	bus := signal.NewMemBus()
	bus.Emit(&signal.Signal{Event: "a"})
	bus.Emit(&signal.Signal{Event: "b"})
	bus.Emit(&signal.Signal{Event: "c"})

	if bus.Len() != 3 {
		t.Fatalf("len: got %d, want 3", bus.Len())
	}

	s0 := bus.Since(0)
	if len(s0) != 3 {
		t.Fatalf("Since(0): got %d, want 3", len(s0))
	}
	s1 := bus.Since(1)
	if len(s1) != 2 {
		t.Fatalf("Since(1): got %d, want 2", len(s1))
	}
	if s1[0].Event != "b" {
		t.Errorf("Since(1)[0].Event: got %q, want b", s1[0].Event)
	}
	s3 := bus.Since(3)
	if s3 != nil {
		t.Errorf("Since(3): got %v, want nil", s3)
	}
	sNeg := bus.Since(-1)
	if len(sNeg) != 3 {
		t.Errorf("Since(-1) should clamp to 0: got len %d", len(sNeg))
	}
}

func TestMemBus_Len(t *testing.T) {
	bus := signal.NewMemBus()
	for i := 0; i < 5; i++ {
		bus.Emit(&signal.Signal{Event: "e"})
		if bus.Len() != i+1 {
			t.Errorf("after %d emits: Len()=%d, want %d", i+1, bus.Len(), i+1)
		}
	}
}

func TestMemBus_OnEmitCallback(t *testing.T) {
	bus := signal.NewMemBus()

	var received []string
	bus.OnEmit(func(s signal.Signal) {
		received = append(received, s.Event)
	})

	bus.Emit(&signal.Signal{Event: "alpha"})
	bus.Emit(&signal.Signal{Event: "beta"})

	if len(received) != 2 {
		t.Fatalf("callback count: got %d, want 2", len(received))
	}
	if received[0] != "alpha" || received[1] != "beta" {
		t.Errorf("callback events: got %v", received)
	}
}

func TestMemBus_MultipleOnEmitCallbacks(t *testing.T) {
	bus := signal.NewMemBus()

	var count1, count2 int
	bus.OnEmit(func(_ signal.Signal) { count1++ })
	bus.OnEmit(func(_ signal.Signal) { count2++ })

	bus.Emit(&signal.Signal{Event: "x"})

	if count1 != 1 || count2 != 1 {
		t.Errorf("both callbacks should fire: count1=%d count2=%d", count1, count2)
	}
}

func TestMemBus_ConcurrentSafety(t *testing.T) {
	bus := signal.NewMemBus()
	const goroutines = 10
	const opsPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(gid int) {
			defer wg.Done()
			for i := 0; i < opsPerGoroutine; i++ {
				bus.Emit(&signal.Signal{
					Event: fmt.Sprintf("g%d-op%d", gid, i),
					Agent: "test",
				})
			}
		}(g)
	}
	wg.Wait()

	total := goroutines * opsPerGoroutine
	if bus.Len() != total {
		t.Errorf("concurrent Len: got %d, want %d", bus.Len(), total)
	}
	sigs := bus.Since(0)
	if len(sigs) != total {
		t.Errorf("concurrent Since(0): got %d, want %d", len(sigs), total)
	}
}

func TestMemBus_ImplementsBusInterface(t *testing.T) {
	var _ signal.Bus = signal.NewMemBus()
}

// ---------------------------------------------------------------------------
// DurableBus
// ---------------------------------------------------------------------------

func TestDurableBus_EmitAndReplay(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "signals.jsonl")

	bus, err := signal.NewDurableBus(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	bus.Emit(&signal.Signal{Event: "case_start", Agent: "worker-1", CaseID: "C1", Step: "recall"})
	bus.Emit(&signal.Signal{
		Event:  "step_complete",
		Agent:  "worker-1",
		CaseID: "C1",
		Step:   "recall",
		Meta:   map[string]string{"outcome": "hit"},
	})
	bus.Emit(&signal.Signal{Event: "case_start", Agent: "worker-1", CaseID: "C2", Step: "recall"})

	if bus.Len() != 3 {
		t.Fatalf("expected 3 signals, got %d", bus.Len())
	}

	if err := bus.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// Verify file exists and has content.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Size() == 0 {
		t.Error("signal file should not be empty")
	}

	// Create a new bus and replay.
	bus2, err := signal.NewDurableBus(path)
	if err != nil {
		t.Fatalf("create for replay: %v", err)
	}
	defer bus2.Close()

	count, err := bus2.Replay()
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if count != 3 {
		t.Errorf("replayed %d signals, want 3", count)
	}
	if bus2.Len() != 3 {
		t.Errorf("bus has %d signals after replay, want 3", bus2.Len())
	}

	signals := bus2.Since(0)
	if signals[0].Event != "case_start" {
		t.Errorf("first signal: got %s, want case_start", signals[0].Event)
	}
	if signals[1].Meta["outcome"] != "hit" {
		t.Error("second signal should have outcome=hit meta")
	}
}

func TestDurableBus_ReplayMissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing.jsonl")

	bus, err := signal.NewDurableBus(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Close first to release the write handle, then delete the file.
	bus.Close()
	os.Remove(path)

	// Replay on missing file should succeed with 0 count.
	bus2, err := signal.NewDurableBus(path)
	if err != nil {
		t.Fatalf("create for replay: %v", err)
	}
	defer bus2.Close()

	count, err := bus2.Replay()
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if count != 0 {
		t.Errorf("replay count: got %d, want 0", count)
	}
}

func TestDurableBus_AppendAfterReplay(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "signals.jsonl")

	bus, err := signal.NewDurableBus(path)
	if err != nil {
		t.Fatal(err)
	}
	bus.Emit(&signal.Signal{Event: "start", Agent: "w1", CaseID: "C1"})
	bus.Close()

	bus2, err := signal.NewDurableBus(path)
	if err != nil {
		t.Fatal(err)
	}
	defer bus2.Close()

	if _, err = bus2.Replay(); err != nil {
		t.Fatalf("replay: %v", err)
	}
	bus2.Emit(&signal.Signal{Event: "continue", Agent: "w1", CaseID: "C1", Step: "triage"})

	if bus2.Len() != 2 {
		t.Errorf("expected 2 signals, got %d", bus2.Len())
	}

	// Close and re-replay to verify both signals persisted.
	bus2.Close()

	bus3, err := signal.NewDurableBus(path)
	if err != nil {
		t.Fatal(err)
	}
	defer bus3.Close()
	count, _ := bus3.Replay()
	if count != 2 {
		t.Errorf("replayed %d, want 2", count)
	}
}

func TestDurableBus_FilePersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "persist.jsonl")

	bus, err := signal.NewDurableBus(path)
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 5; i++ {
		bus.Emit(&signal.Signal{
			Event:        fmt.Sprintf("event-%d", i),
			Agent:        "test",
			Performative: signal.Inform,
		})
	}
	bus.Close()

	// Re-open and replay -- verify all 5 signals persist.
	bus2, err := signal.NewDurableBus(path)
	if err != nil {
		t.Fatal(err)
	}
	defer bus2.Close()

	count, err := bus2.Replay()
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if count != 5 {
		t.Errorf("replay count: got %d, want 5", count)
	}

	sigs := bus2.Since(0)
	for i, s := range sigs {
		want := fmt.Sprintf("event-%d", i)
		if s.Event != want {
			t.Errorf("signal[%d].Event: got %q, want %q", i, s.Event, want)
		}
	}
}

func TestDurableBus_Path(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")

	bus, err := signal.NewDurableBus(path)
	if err != nil {
		t.Fatal(err)
	}
	defer bus.Close()

	if bus.Path() != path {
		t.Errorf("Path: got %q, want %q", bus.Path(), path)
	}
}

func TestDurableBus_ImplementsBusInterface(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "iface.jsonl")

	bus, err := signal.NewDurableBus(path)
	if err != nil {
		t.Fatal(err)
	}
	defer bus.Close()

	var _ signal.Bus = bus
}

// ---------------------------------------------------------------------------
// Supervisor
// ---------------------------------------------------------------------------

func emitWorkerSignal(bus signal.Bus, event, workerID string) {
	bus.Emit(&signal.Signal{
		Event: event,
		Agent: signal.AgentWorker,
		Meta:  map[string]string{signal.MetaKeyWorkerID: workerID},
	})
}

func emitWorkerDone(bus signal.Bus, workerID, caseID, step string) {
	bus.Emit(&signal.Signal{
		Event:  signal.EventWorkerDone,
		Agent:  signal.AgentWorker,
		CaseID: caseID,
		Step:   step,
		Meta:   map[string]string{signal.MetaKeyWorkerID: workerID},
	})
}

func emitWorkerError(bus signal.Bus, workerID, caseID, step, errMsg string) {
	bus.Emit(&signal.Signal{
		Event:  signal.EventWorkerError,
		Agent:  signal.AgentWorker,
		CaseID: caseID,
		Step:   step,
		Meta: map[string]string{
			signal.MetaKeyWorkerID: workerID,
			signal.MetaKeyError:    errMsg,
		},
	})
}

func TestSupervisor_WorkerLifecycle(t *testing.T) {
	bus := signal.NewMemBus()
	sup := signal.NewSupervisor(signal.NewBusEventLog(bus))

	emitWorkerSignal(bus, signal.EventWorkerStarted, "w1")
	emitWorkerSignal(bus, signal.EventWorkerStarted, "w2")

	sup.Process()
	h := sup.Health()

	if h.TotalActive != 2 {
		t.Errorf("expected 2 active workers, got %d", h.TotalActive)
	}
	if len(h.Workers) != 2 {
		t.Errorf("expected 2 workers, got %d", len(h.Workers))
	}

	emitWorkerSignal(bus, signal.EventWorkerStopped, "w1")
	sup.Process()
	h = sup.Health()

	if h.TotalActive != 1 {
		t.Errorf("expected 1 active worker, got %d", h.TotalActive)
	}
	if h.TotalStopped != 1 {
		t.Errorf("expected 1 stopped worker, got %d", h.TotalStopped)
	}
}

func TestSupervisor_ErrorThreshold_FlagsReplacement(t *testing.T) {
	bus := signal.NewMemBus()
	sup := signal.NewSupervisor(signal.NewBusEventLog(bus), signal.WithErrorThreshold(2))

	emitWorkerSignal(bus, signal.EventWorkerStarted, "w1")
	emitWorkerError(bus, "w1", "C1", "F0", "first error")

	sup.Process()
	h := sup.Health()

	if len(h.ShouldReplace) != 0 {
		t.Errorf("1 error should not trigger replacement, got %v", h.ShouldReplace)
	}

	emitWorkerError(bus, "w1", "C2", "F1", "second error")

	sup.Process()
	h = sup.Health()

	if h.TotalErrored != 1 {
		t.Errorf("expected 1 errored worker, got %d", h.TotalErrored)
	}
	if len(h.ShouldReplace) != 1 || h.ShouldReplace[0] != "w1" {
		t.Errorf("expected [w1] in should_replace, got %v", h.ShouldReplace)
	}
}

func TestSupervisor_SilenceThreshold_FlagsReplacement(t *testing.T) {
	bus := signal.NewMemBus()
	sup := signal.NewSupervisor(signal.NewBusEventLog(bus), signal.WithSilenceThreshold(50*time.Millisecond))

	emitWorkerSignal(bus, signal.EventWorkerStarted, "w1")
	sup.Process()

	time.Sleep(100 * time.Millisecond)

	h := sup.Health()
	if len(h.ShouldReplace) != 1 || h.ShouldReplace[0] != "w1" {
		t.Errorf("expected [w1] flagged as silent, got %v", h.ShouldReplace)
	}
}

func TestSupervisor_StepCounting(t *testing.T) {
	bus := signal.NewMemBus()
	sup := signal.NewSupervisor(signal.NewBusEventLog(bus))

	emitWorkerSignal(bus, signal.EventWorkerStarted, "w1")
	for i := 0; i < 3; i++ {
		emitWorkerDone(bus, "w1", fmt.Sprintf("C%d", i+1), "F0")
	}

	sup.Process()
	h := sup.Health()

	for _, w := range h.Workers {
		if w.WorkerID == "w1" {
			if w.StepsComplete != 3 {
				t.Errorf("expected 3 steps complete, got %d", w.StepsComplete)
			}
			return
		}
	}
	t.Error("worker w1 not found in health summary")
}

func TestSupervisor_ShouldStop(t *testing.T) {
	bus := signal.NewMemBus()
	sup := signal.NewSupervisor(signal.NewBusEventLog(bus))

	if sup.ShouldStop() {
		t.Error("should_stop should be false initially")
	}

	sup.EmitShouldStop()
	sup.Process()

	if !sup.ShouldStop() {
		t.Error("should_stop should be true after EmitShouldStop")
	}
}

func TestSupervisor_BudgetTracking(t *testing.T) {
	bus := signal.NewMemBus()
	sup := signal.NewSupervisor(signal.NewBusEventLog(bus), signal.WithBudgetTotal(1000))

	bus.Emit(&signal.Signal{
		Event: signal.EventBudgetUpdate,
		Agent: "system",
		Meta:  map[string]string{signal.MetaKeyUsed: "500"},
	})
	sup.Process()

	h := sup.Health()
	if h.BudgetUsedPct < 49.9 || h.BudgetUsedPct > 50.1 {
		t.Errorf("expected ~50%% budget used, got %.1f%%", h.BudgetUsedPct)
	}
}

func TestSupervisor_IncrementalProcessing(t *testing.T) {
	bus := signal.NewMemBus()
	sup := signal.NewSupervisor(signal.NewBusEventLog(bus))

	emitWorkerSignal(bus, signal.EventWorkerStarted, "w1")
	sup.Process()

	h := sup.Health()
	if h.TotalActive != 1 {
		t.Fatalf("expected 1 active, got %d", h.TotalActive)
	}

	emitWorkerSignal(bus, signal.EventWorkerStarted, "w2")
	sup.Process()

	h = sup.Health()
	if h.TotalActive != 2 {
		t.Errorf("expected 2 active after incremental process, got %d", h.TotalActive)
	}
}

func TestSupervisor_ConcurrentProcess_Race(t *testing.T) {
	bus := signal.NewMemBus()
	sup := signal.NewSupervisor(signal.NewBusEventLog(bus))

	emitWorkerSignal(bus, signal.EventWorkerStarted, "w1")
	const doneSignals = 5
	for i := 0; i < doneSignals; i++ {
		emitWorkerDone(bus, "w1", fmt.Sprintf("C%d", i), "F0")
	}

	const goroutines = 50
	var barrier sync.WaitGroup
	barrier.Add(goroutines)
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			barrier.Done()
			barrier.Wait()
			sup.Process()
		}()
	}
	wg.Wait()

	h := sup.Health()
	for _, w := range h.Workers {
		if w.WorkerID == "w1" && w.StepsComplete != doneSignals {
			t.Errorf("double-counting: expected %d steps, got %d",
				doneSignals, w.StepsComplete)
		}
	}

	// Emit a new signal and verify Process() still sees it.
	emitWorkerDone(bus, "w1", "C_late", "F1")
	sup.Process()

	h = sup.Health()
	for _, w := range h.Workers {
		if w.WorkerID == "w1" {
			if w.StepsComplete != doneSignals+1 {
				t.Errorf("signal blindness: expected %d steps after late signal, got %d",
					doneSignals+1, w.StepsComplete)
			}
			return
		}
	}
	t.Error("w1 not found in health summary")
}

func TestSupervisor_MultipleWorkersIndependent(t *testing.T) {
	bus := signal.NewMemBus()
	sup := signal.NewSupervisor(signal.NewBusEventLog(bus), signal.WithErrorThreshold(2))

	emitWorkerSignal(bus, signal.EventWorkerStarted, "w1")
	emitWorkerSignal(bus, signal.EventWorkerStarted, "w2")
	emitWorkerError(bus, "w1", "C1", "F0", "e1")
	emitWorkerError(bus, "w1", "C2", "F0", "e2")
	emitWorkerError(bus, "w2", "C4", "F0", "e3") // below threshold
	emitWorkerDone(bus, "w2", "C3", "F0")

	sup.Process()
	h := sup.Health()

	if h.TotalErrored != 1 {
		t.Errorf("expected 1 errored, got %d", h.TotalErrored)
	}
	if h.TotalActive != 1 {
		t.Errorf("expected 1 active, got %d", h.TotalActive)
	}

	var w1found, w2found bool
	for _, w := range h.Workers {
		switch w.WorkerID {
		case "w1":
			w1found = true
			if w.Status != signal.WorkerStatusErrored {
				t.Errorf("w1 expected errored, got %s", w.Status)
			}
			if w.ErrorCount != 2 {
				t.Errorf("w1 expected 2 errors, got %d", w.ErrorCount)
			}
		case "w2":
			w2found = true
			if w.Status != signal.WorkerStatusActive {
				t.Errorf("w2 expected active, got %s", w.Status)
			}
			if w.StepsComplete != 1 {
				t.Errorf("w2 expected 1 step, got %d", w.StepsComplete)
			}
			if w.ErrorCount != 1 {
				t.Errorf("w2 expected 1 error, got %d", w.ErrorCount)
			}
		}
	}
	if !w1found || !w2found {
		t.Error("missing worker in health summary")
	}
}

// ---------------------------------------------------------------------------
// Performative
// ---------------------------------------------------------------------------

func TestSignal_Performative(t *testing.T) {
	s := signal.Signal{
		Event:        "test",
		Agent:        "agent1",
		Performative: signal.Handoff,
	}
	if s.Performative != signal.Handoff {
		t.Errorf("performative: got %q, want %q", s.Performative, signal.Handoff)
	}
}

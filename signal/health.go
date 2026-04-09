package signal

import (
	"sync"
	"time"

	"github.com/dpopsuev/battery/event"
)

// WorkerStatus describes the operational state of a worker.
type WorkerStatus string

const (
	WorkerStatusActive  WorkerStatus = "active"
	WorkerStatusIdle    WorkerStatus = "idle"
	WorkerStatusErrored WorkerStatus = "errored"
	WorkerStatusStopped WorkerStatus = "stopped"
)

// WorkerState tracks the health state of a single worker.
type WorkerState struct {
	WorkerID      string       `json:"worker_id"`
	Status        WorkerStatus `json:"status"`
	ErrorCount    int          `json:"error_count"`
	StepsComplete int          `json:"steps_complete"`
	LastSeen      time.Time    `json:"last_seen"`
	LastError     string       `json:"last_error,omitempty"`
}

// HealthSummary is a snapshot of all tracked workers and overall circuit health.
type HealthSummary struct {
	Workers       []WorkerState `json:"workers"`
	TotalActive   int           `json:"total_active"`
	TotalErrored  int           `json:"total_errored"`
	TotalStopped  int           `json:"total_stopped"`
	ShouldReplace []string      `json:"should_replace,omitempty"`
	ShouldStop    bool          `json:"should_stop"`
	QueueDepth    int           `json:"queue_depth,omitempty"`
	BudgetUsedPct float64       `json:"budget_used_pct,omitempty"`
}

// Supervisor watches an EventLog and maintains per-worker health state.
// The supervisor agent queries this for health summaries to make replacement
// and shutdown decisions.
type Supervisor struct {
	mu               sync.Mutex
	workers          map[string]*WorkerState
	lastProcessed    int
	log              event.EventLog
	silenceThreshold time.Duration
	errorThreshold   int
	shouldStop       bool
	budgetTotal      float64
	budgetUsed       float64
}

// SupervisorOption configures a Supervisor.
type SupervisorOption func(*Supervisor)

// WithSilenceThreshold sets how long a worker can be silent before being
// flagged for replacement. Default: 2 minutes.
func WithSilenceThreshold(d time.Duration) SupervisorOption {
	return func(s *Supervisor) { s.silenceThreshold = d }
}

// WithErrorThreshold sets how many errors a worker can accumulate before being
// flagged for replacement. Default: 3.
func WithErrorThreshold(n int) SupervisorOption {
	return func(s *Supervisor) { s.errorThreshold = n }
}

// WithBudgetTotal sets the total budget for budget tracking (arbitrary units).
func WithBudgetTotal(total float64) SupervisorOption {
	return func(s *Supervisor) { s.budgetTotal = total }
}

// NewSupervisor creates a health tracker that watches the given EventLog.
func NewSupervisor(log event.EventLog, opts ...SupervisorOption) *Supervisor {
	s := &Supervisor{
		workers:          make(map[string]*WorkerState),
		log:              log,
		silenceThreshold: 2 * time.Minute,
		errorThreshold:   3,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Process reads new events from the log and updates worker state.
// Safe for concurrent callers -- lastProcessed is read and written
// under the same lock to prevent double-counting and index overshoot.
func (s *Supervisor) Process() {
	s.mu.Lock()
	defer s.mu.Unlock()

	events := s.log.Since(s.lastProcessed)
	if len(events) == 0 {
		return
	}

	for _, evt := range events {
		s.lastProcessed++
		wid := evt.Meta[MetaKeyWorkerID]
		if wid == "" && evt.Kind != EventShouldStop && evt.Kind != EventBudgetUpdate {
			continue
		}

		switch evt.Kind {
		case EventWorkerStarted:
			s.workers[wid] = &WorkerState{
				WorkerID: wid,
				Status:   WorkerStatusActive,
				LastSeen: evt.Timestamp,
			}

		case EventWorkerStopped:
			if w, ok := s.workers[wid]; ok {
				w.Status = WorkerStatusStopped
				w.LastSeen = evt.Timestamp
			}

		case EventWorkerStart, EventWorkerDone:
			if w, ok := s.workers[wid]; ok {
				w.LastSeen = evt.Timestamp
				w.Status = WorkerStatusActive
				if evt.Kind == EventWorkerDone {
					w.StepsComplete++
				}
			}

		case EventWorkerError:
			if w, ok := s.workers[wid]; ok {
				w.ErrorCount++
				w.LastError = evt.Meta[MetaKeyError]
				w.LastSeen = evt.Timestamp
				if w.ErrorCount >= s.errorThreshold {
					w.Status = WorkerStatusErrored
				}
			}

		case EventShouldStop:
			s.shouldStop = true

		case EventBudgetUpdate:
			if v, ok := evt.Meta[MetaKeyUsed]; ok {
				n, _ := parseFloat(v)
				s.budgetUsed = n
			}
		}
	}
}

// Health returns a snapshot of the current worker health state.
// Callers should call Process() first to ensure state is up-to-date.
func (s *Supervisor) Health() HealthSummary {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	summary := HealthSummary{
		ShouldStop: s.shouldStop,
	}

	if s.budgetTotal > 0 {
		summary.BudgetUsedPct = (s.budgetUsed / s.budgetTotal) * 100
	}

	for _, w := range s.workers {
		summary.Workers = append(summary.Workers, *w)

		switch w.Status {
		case WorkerStatusActive:
			summary.TotalActive++
			if s.silenceThreshold > 0 && now.Sub(w.LastSeen) > s.silenceThreshold {
				summary.ShouldReplace = append(summary.ShouldReplace, w.WorkerID)
			}
		case WorkerStatusErrored:
			summary.TotalErrored++
			summary.ShouldReplace = append(summary.ShouldReplace, w.WorkerID)
		case WorkerStatusStopped:
			summary.TotalStopped++
		}
	}

	return summary
}

// EmitShouldStop emits a should_stop event on the log, instructing workers
// to finish their current step and exit.
func (s *Supervisor) EmitShouldStop() {
	s.log.Emit(event.Event{
		Source: AgentSupervisor,
		Kind:   EventShouldStop,
	})
}

// ShouldStop returns true if a should_stop signal has been processed.
func (s *Supervisor) ShouldStop() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.shouldStop
}


func parseFloat(s string) (float64, bool) {
	var result float64
	var decimal float64
	var inDecimal bool
	for _, c := range s {
		if c >= '0' && c <= '9' {
			if inDecimal {
				decimal /= 10
				result += float64(c-'0') * decimal
			} else {
				result = result*10 + float64(c-'0')
			}
		} else if c == '.' {
			inDecimal = true
			decimal = 1
		}
	}
	return result, true
}

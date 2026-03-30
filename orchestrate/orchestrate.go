package orchestrate

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
)

// Log key constants for sloglint compliance.
const (
	logKeySession = "session"
	logKeyCount   = "count"
)

// Sentinel errors.
var (
	ErrAlreadyRunning = errors.New("workers are already running; call stop first")
	ErrNotRunning     = errors.New("no workers running")
	ErrStepFailed     = errors.New("step failed")
)

// Manager manages a pool of agent workers that connect to an MCP endpoint.
type Manager struct {
	mu        sync.Mutex
	endpoint  string
	cancel    context.CancelFunc
	running   bool
	count     int
	agent     string
	session   string
	cfg       WorkerConfig
	completed atomic.Int64
	errored   atomic.Int64
}

// NewManager creates a manager that spawns workers connecting to the given endpoint.
func NewManager(endpoint string, cfg WorkerConfig) *Manager {
	cfg.defaults()
	return &Manager{endpoint: endpoint, cfg: cfg}
}

// Start spawns N agent workers as goroutines.
func (m *Manager) Start(ctx context.Context, session, agent string, count int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return ErrAlreadyRunning
	}

	if agent == "" {
		agent = "claude"
	}
	if count < 1 {
		count = 4
	}

	m.session = session
	m.agent = agent
	m.count = count
	m.running = true
	m.completed.Store(0)
	m.errored.Store(0)

	workerCtx, cancel := context.WithCancel(ctx)
	m.cancel = cancel

	var wg sync.WaitGroup
	for i := range count {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			name := fmt.Sprintf("worker-%d", id+1)
			if err := RunWorker(workerCtx, m.endpoint, agent, session, name, m.cfg); err != nil {
				m.errored.Add(1)
				slog.ErrorContext(workerCtx, "worker failed",
					slog.String(logKeyWorker, name),
					slog.Any(logKeyError, err))
			} else {
				m.completed.Add(1)
				slog.InfoContext(workerCtx, "worker done",
					slog.String(logKeyWorker, name))
			}
		}(i)
	}

	go func() {
		wg.Wait()
		m.mu.Lock()
		m.running = false
		m.mu.Unlock()
	}()

	slog.InfoContext(ctx, "workers started",
		slog.String(logKeySession, session),
		slog.String(logKeyAgent, agent),
		slog.Int(logKeyCount, count))

	return nil
}

// Stop kills all running workers.
func (m *Manager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return ErrNotRunning
	}
	m.cancel()
	return nil
}

// Health returns worker status.
func (m *Manager) Health() map[string]any {
	m.mu.Lock()
	running := m.running
	m.mu.Unlock()

	return map[string]any{
		"running":   running,
		"session":   m.session,
		"agent":     m.agent,
		"count":     m.count,
		"completed": m.completed.Load(),
		"errored":   m.errored.Load(),
	}
}

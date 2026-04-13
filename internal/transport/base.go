package transport

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
)

type taskEntry struct {
	task *Task
	subs []chan Event
	mu   sync.Mutex
}

// baseTransport is the shared task management core embedded by
// LocalTransport and HTTPTransport. Handles handler registration,
// task lifecycle, subscriptions, and role-based routing.
type baseTransport struct {
	mu          sync.RWMutex
	handlers    map[AgentID]MsgHandler
	tasks       map[string]*taskEntry
	nextID      uint64
	closed      bool
	roles       *RoleRegistry
	roleCounter map[string]int
}

func newBase() baseTransport {
	return baseTransport{
		handlers:    make(map[AgentID]MsgHandler),
		tasks:       make(map[string]*taskEntry),
		roles:       NewRoleRegistry(),
		roleCounter: make(map[string]int),
	}
}

func (b *baseTransport) Roles() *RoleRegistry { return b.roles }

func (b *baseTransport) Register(agentID AgentID, handler MsgHandler) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, exists := b.handlers[agentID]; exists {
		return fmt.Errorf("%w: %q", ErrAlreadyRegistered, agentID)
	}
	b.handlers[agentID] = handler
	return nil
}

func (b *baseTransport) Unregister(agentID AgentID) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.handlers, agentID)
}

func (b *baseTransport) SendMessage(ctx context.Context, to AgentID, msg Message) (*Task, error) {
	b.mu.RLock()
	handler, ok := b.handlers[to]
	closed := b.closed
	b.mu.RUnlock()

	if closed {
		return nil, ErrTransportClosed
	}
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrAgentNotFound, to)
	}

	taskID := fmt.Sprintf("task-%d", atomic.AddUint64(&b.nextID, 1))
	task := &Task{ID: taskID, State: TaskSubmitted}
	entry := &taskEntry{task: task}

	b.mu.Lock()
	b.tasks[taskID] = entry
	b.mu.Unlock()

	b.notify(entry, Event{TaskID: taskID, State: TaskSubmitted})
	go b.execute(ctx, handler, entry, msg)

	return task, nil
}

func (b *baseTransport) execute(ctx context.Context, handler MsgHandler, entry *taskEntry, msg Message) {
	entry.mu.Lock()
	entry.task.State = TaskWorking
	taskID := entry.task.ID
	entry.mu.Unlock()

	b.notify(entry, Event{TaskID: taskID, State: TaskWorking})

	result, err := handler(ctx, msg)

	entry.mu.Lock()
	defer entry.mu.Unlock()

	if err != nil {
		entry.task.State = TaskFailed
		entry.task.Error = err.Error()
		b.notifyLocked(entry, Event{TaskID: taskID, State: TaskFailed})
	} else {
		entry.task.State = TaskCompleted
		entry.task.Result = &result
		b.notifyLocked(entry, Event{TaskID: taskID, State: TaskCompleted, Data: &result})
	}

	for _, ch := range entry.subs {
		close(ch)
	}
	entry.subs = nil
}

func (b *baseTransport) Subscribe(_ context.Context, taskID string) (<-chan Event, error) {
	b.mu.RLock()
	entry, ok := b.tasks[taskID]
	b.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrTaskNotFound, taskID)
	}

	ch := make(chan Event, 8) //nolint:mnd // small buffer for expected events

	entry.mu.Lock()
	defer entry.mu.Unlock()

	if entry.task.State == TaskCompleted || entry.task.State == TaskFailed || entry.task.State == TaskCanceled {
		ch <- Event{TaskID: taskID, State: entry.task.State, Data: entry.task.Result}
		close(ch)
		return ch, nil
	}

	entry.subs = append(entry.subs, ch)
	return ch, nil
}

func (b *baseTransport) Ask(ctx context.Context, to AgentID, msg Message) (Message, error) {
	task, err := b.SendMessage(ctx, to, msg)
	if err != nil {
		return Message{}, err
	}

	ch, err := b.Subscribe(ctx, task.ID)
	if err != nil {
		return Message{}, err
	}

	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return Message{}, fmt.Errorf("%w: %s", ErrTaskChanClosed, task.ID)
			}
			switch ev.State {
			case TaskCompleted:
				if ev.Data != nil {
					return *ev.Data, nil
				}
				return Message{}, nil
			case TaskFailed:
				return Message{}, fmt.Errorf("%w: %s", ErrTaskFailed, task.ID)
			}
		case <-ctx.Done():
			return Message{}, ctx.Err()
		}
	}
}

func (b *baseTransport) SendToRole(ctx context.Context, role string, msg Message) (*Task, error) {
	agents := b.roles.AgentsForRole(role)
	if len(agents) == 0 {
		return nil, fmt.Errorf("%w: %q", ErrNoAgentsForRole, role)
	}
	b.mu.Lock()
	idx := b.roleCounter[role]
	b.roleCounter[role] = idx + 1
	b.mu.Unlock()
	return b.SendMessage(ctx, AgentID(agents[idx%len(agents)]), msg)
}

func (b *baseTransport) AskRole(ctx context.Context, role string, msg Message) (Message, error) {
	agents := b.roles.AgentsForRole(role)
	if len(agents) == 0 {
		return Message{}, fmt.Errorf("%w: %q", ErrNoAgentsForRole, role)
	}
	b.mu.Lock()
	idx := b.roleCounter[role]
	b.roleCounter[role] = idx + 1
	b.mu.Unlock()
	return b.Ask(ctx, AgentID(agents[idx%len(agents)]), msg)
}

func (b *baseTransport) Broadcast(ctx context.Context, role string, msg Message) ([]*Task, error) {
	agents := b.roles.AgentsForRole(role)
	if len(agents) == 0 {
		return nil, fmt.Errorf("%w: %q", ErrNoAgentsForRole, role)
	}
	tasks := make([]*Task, 0, len(agents))
	for _, aid := range agents {
		task, err := b.SendMessage(ctx, AgentID(aid), msg)
		if err != nil {
			return tasks, fmt.Errorf("transport: broadcast to %s: %w", aid, err)
		}
		tasks = append(tasks, task)
	}
	return tasks, nil
}

func (b *baseTransport) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers = make(map[AgentID]MsgHandler)
	b.closed = true
	return nil
}

func (b *baseTransport) notify(entry *taskEntry, ev Event) {
	entry.mu.Lock()
	defer entry.mu.Unlock()
	b.notifyLocked(entry, ev)
}

func (*baseTransport) notifyLocked(entry *taskEntry, ev Event) {
	for _, ch := range entry.subs {
		select {
		case ch <- ev:
		default:
		}
	}
}

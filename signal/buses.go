package signal

// ControlLog carries control-plane events: routing decisions, vetoes,
// hook executions. Producers: broker, director.
type ControlLog struct{ EventLog }

// WorkLog carries data-plane events: task start, done, error.
// Producers: hookedActor, transport handlers.
type WorkLog struct{ EventLog }

// StatusLog carries observability events: Andon health transitions,
// worker lifecycle, budget updates, perf metrics, and projections
// from Control and Data planes. Producers: warden, world, supervisor,
// plus projected events from control/data emitters.
type StatusLog struct{ EventLog }

// BusSet groups the three typed event buses. Passed through broker,
// warden, and testkit instead of a single EventLog.
type BusSet struct {
	Control ControlLog
	Work    WorkLog
	Status  StatusLog
}

// NewBusSet creates a BusSet backed by three independent MemLogs.
func NewBusSet() BusSet {
	return BusSet{
		Control: ControlLog{NewMemLog()},
		Work:    WorkLog{NewMemLog()},
		Status:  StatusLog{NewMemLog()},
	}
}

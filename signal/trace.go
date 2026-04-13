// trace.go — cognitive event helpers for agent-internal state transitions.
// These are NOT tool calls — they're agent thinking, deciding, retrying.
// Consumers call these from inside agent loops to emit trace events.
package signal

// Cognitive event kinds.
const (
	KindThink  = "agent.cognitive.think"
	KindDecide = "agent.cognitive.decide"
	KindRetry  = "agent.cognitive.retry"
	KindGiveUp = "agent.cognitive.give_up"
)

// Think creates an event for agent reasoning.
func Think(traceID, source, msg string) Event {
	return Event{TraceID: traceID, Source: source, Kind: KindThink, Data: msg}
}

// Decide creates an event for agent decision.
func Decide(traceID, source, msg string) Event {
	return Event{TraceID: traceID, Source: source, Kind: KindDecide, Data: msg}
}

// Retry creates an event for agent retry.
func Retry(traceID, source, msg string) Event {
	return Event{TraceID: traceID, Source: source, Kind: KindRetry, Data: msg}
}

// GiveUp creates an event for agent giving up.
func GiveUp(traceID, source, msg string) Event {
	return Event{TraceID: traceID, Source: source, Kind: KindGiveUp, Data: msg}
}

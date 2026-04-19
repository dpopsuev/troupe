# Claude Code Instructions for Troupe Development

## What is Troupe

Troupe is a dual-face AI agent platform:
1. **Server**: Authoritative ECS World for multi-agent orchestration — lifecycle, A2A, admission, buses, scheduling
2. **Library**: Exportable harness components for agent builders — LLM drivers, billing, resilience, model selection, scoring, predicates

Three core interfaces (Broker, Actor, Director) compose agents, strategies, and drivers into collectives.

- Repo: github.com/dpopsuev/troupe
- Scribe scope: troupe

## Platform Boundaries

Troupe is part of a three-project platform. Each project owns specific boundaries:

| Boundary | Owner | What |
|---|---|---|
| World ↔ Agent | **Troupe** | A2A, Admission, lifecycle, ECS, buses, resilience, billing, scoring |
| Agent ↔ Tools | **Origami** | Workbench instruments, normalized tool I/O, strict mode |
| Agent Orchestration | **Origami** | Circuits, graph-walking, Virtuoso harness |
| Human ↔ Agent | **Djinn** | Operator frontend, REPL, HITL, Cortex, context engineering |

Dependency direction: `Djinn -> Origami -> Troupe`

## Ecosystem Dependency Rules

**CRITICAL: Troupe is the bottom of the dependency stack.**

- Troupe NEVER imports origami/ or djinn/ or hegemony/
- Troupe defines interfaces (Actor, Broker, Director, Driver, Meter), consumers implement them
- Consumer-to-consumer communication goes through A2A protocol, not Go imports

## Architecture: Server + Library

### Server packages (authoritative multi-agent world)

```
Root package   — Broker, Actor, Director, Driver, Meter, Gate, Pick, Threshold, Admission, Admin, AgentCard interfaces
broker/        — Broker implementation, Lobby (admission), multi-driver adapter, hooked actors
                 broker.New() = bare, broker.Default() = batteries-included (Arsenal wired)
signal/        — Three-bus architecture (ControlLog, WorkLog, StatusLog), Andon health, EventStore
world/         — ECS entity-component store (Alive/Ready, ComponentType, hierarchy edges)
collective/    — Multi-agent primitives (Race, RoundRobin, Scatter, Scale, Dialectic, Arbiter, Fallback)
```

### Library packages (all wired into Broker via With* options)

```
providers/     — LLM provider abstraction (any-llm-go: Anthropic, OpenAI, Gemini, Vertex, OpenRouter)
                 Wired via WithProviderResolver()
billing/       — Token/cost tracking (CostBill, BudgetEnforcer) — wired via WithTracker()
referee/       — Event-driven scoring engine (YAML Scorecards) — wired via WithReferee()
arsenal/       — Embedded model catalog (trait-scored selection, TraitVector) — wired via WithArsenal()
resilience/    — Circuit breaker, retry, timeout, rate limiter (pure algorithms)
visual/        — Cosmetic identity (Color, Palette, Element, View)
testkit/       — Test fixtures (MockActor, MockBroker, toy agents, BusSet helpers)
```

### Internal packages

```
internal/agent/     — Solo agent implementation (Actor wrapper)
auth/               — Authentication abstraction (Bearer, Identity, Authenticator)
                      Wired into transport via NewA2ATransportWithAuth()
internal/transport/ — A2A messaging types, LocalTransport, HTTPTransport, A2A server
internal/warden/    — Agent process supervision (Fork/Kill/Wait, restart, zombie reaping)
```

## Protocol Decisions

- **A2A is the protocol**: A2A (HTTP JSON-RPC via a2a-go) is the sole wire protocol. No custom protocol/ package — types live in internal/transport/.
- **No ACP**: Subprocess launching removed. Agents connect to Troupe via A2A, not process spawning.
- **A2A roles, not Performatives**: Messages use A2A roles (user/agent) directly. FIPA-ACL Performatives removed.
- **Heartbeat vs Andon**: Heartbeat = control plane liveness (transport-level lastSeen). Andon = data plane readiness (StatusLog: Nominal/Degraded/Failure/Blocked/Dead).
- **Three buses**: ControlLog (routing), WorkLog (task lifecycle), StatusLog (health/observability). All durable.
- **Vertex env vars**: Use Google standard (GOOGLE_CLOUD_LOCATION, GOOGLE_CLOUD_PROJECT), not Anthropic convention.
- **Arsenal source provider mask**: vertex-ai.yaml `provider: anthropic` filters models to what the implementation can reach.

## Admin Control Plane

`Admin` interface (admin.go) — privileged operator API, separate from Broker (agent-facing):
- Query: Agents(), Inspect(), Tree()
- Lifecycle: Kill(), Drain(), Undrain()
- Policy: SetBudget(), SetQuota()
- Emergency: Cordon(), Uncordon(), KillAll()

Interface drafted, implementation pending (GOL-44).

## Naming Conventions

- **Core interfaces**: Actor (Perform/Ready/Kill), Broker, Director, Driver, Meter, Admin
- **Predicates**: Gate (allow/deny), Pick[T] (selection), Threshold (numeric condition)
- **Health signals**: Andon (IEC 60073 stack light). NOT horn.
- **Events**: EventKind (Started, Completed, Failed, Transition, Done)
- **Identity**: AgentCard (public interface), ActorConfig (input/job spec), Actor (running instance)
- **Visual**: Color, Palette, Element — cosmetic only, in visual/ package
- **Admission**: Admit (enter), Kick (forceful removal, replaces Dismiss), Ban (Kick + deny list)
- **Project name**: Troupe. NOT Jericho or Bugle.

## Go Conventions

- Go 1.25+
- golangci-lint enforced via pre-commit hook
- American English spelling (canceled, not cancelled)
- Sentinel errors with descriptive names
- slog for structured logging with constant key names
- broker.New() = bare, broker.Default() = batteries-included
- integrate-early: no package without a production caller

# Claude Code Instructions for Jericho Development

## What is Jericho

Jericho is the agent platform — ECS framework for managing autonomous AI agents at scale. The Bugle Protocol (jericho/bugle/) is the wire format for distributing work to agents.

- Repo: github.com/dpopsuev/jericho (renamed from bugle on 2026-03-31)
- Scribe scope: jericho (legacy artifacts use BGL- prefix)
- Campaigns: BGL-CMP-6 (v0.1.0 Bugle Protocol), BGL-CMP-7 (v0.2.0 Cloud Native)

## Ecosystem Dependency Rules (JRC-SPC-2)

**CRITICAL: Jericho is the bottom of the dependency stack.**

- Jericho NEVER imports origami/ or djinn/ or hegemony/
- Jericho defines interfaces (Responder, Server), consumers implement them
- Consumer-to-consumer communication goes through Bugle Protocol, not Go imports
- dispatch/ is being evicted to Origami (circuit-specific, not platform)

Dependency direction: `Origami -> Jericho <- Djinn`

## Package Map

```
bugle/        — Bugle Protocol types (LEAF, zero external deps)
orchestrate/  — Protocol client loop (uses Responder interface)
resilience/   — Circuit breaker, rate limiter, retry (pure algorithms)
acp/          — Agent Context Protocol launcher
pool/         — Agent process lifecycle (Fork/Kill/Wait)
facade/       — Staff, AgentHandle, Agent interface
collective/   — Multi-agent facades (Dialectic, Arbiter strategies)
transport/    — A2A messaging (LocalTransport, role-based routing)
signal/       — Event bus (Bus, DurableBus)
world/        — ECS entity-component store
palette/      — Color identity engine (56 colors, registry)
identity/     — Agent identity types (Persona, AgentIdentity)
persona/      — Default persona templates (Herald, Seeker, etc.)
billing/      — Token/cost tracking
worldview/    — Observable agent state (Snapshot, Subscribe)
testkit/      — Test fixtures (QuickWorld, handlers, assertions)
```

## Naming Conventions

- **Bugle Protocol verbs**: pull (get work), push (return results). NOT step/submit.
- **Health signals**: andon (IEC 60073 stack light). NOT horn.
- **Work item field**: `item`. NOT `step`.
- **Tool name**: `bugle`. NOT `circuit`.
- **Project name**: Jericho. NOT Bugle (Bugle = the protocol only).

## Go Conventions

- Go 1.25+
- golangci-lint enforced via pre-commit hook
- American English spelling (canceled, not cancelled)
- Sentinel errors with descriptive names
- slog for structured logging with constant key names

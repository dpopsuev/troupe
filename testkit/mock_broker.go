package testkit

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/dpopsuev/troupe"
)

// MockBroker is a test Broker that spawns MockActors.
type MockBroker struct {
	// Actors pre-configured for spawning. Pick returns configs for these.
	Actors []*MockActor

	spawned atomic.Int64
}

// NewMockBroker creates a broker with n echo actors.
func NewMockBroker(n int) *MockBroker {
	actors := make([]*MockActor, n)
	for i := range n {
		actors[i] = &MockActor{
			Name: fmt.Sprintf("actor-%d", i+1),
		}
	}
	return &MockBroker{Actors: actors}
}

// Pick returns configs for the pre-configured actors.
func (b *MockBroker) Pick(_ context.Context, prefs troupe.Preferences) ([]troupe.ActorConfig, error) {
	count := prefs.Count
	if count <= 0 {
		count = 1
	}
	if count > len(b.Actors) {
		count = len(b.Actors)
	}

	configs := make([]troupe.ActorConfig, count)
	model := prefs.Model
	if model == "" {
		model = "mock"
	}
	role := prefs.Role
	for i := range count {
		actorRole := role
		if actorRole == "" {
			actorRole = b.Actors[i].Name
		}
		configs[i] = troupe.ActorConfig{
			Model: model,
			Role:  actorRole,
		}
	}
	return configs, nil
}

// Discover returns agent cards for pre-configured actors.
func (b *MockBroker) Discover(role string) []troupe.AgentCard {
	cards := make([]troupe.AgentCard, 0, len(b.Actors))
	for _, a := range b.Actors {
		if role != "" && a.Name != role {
			continue
		}
		cards = append(cards, &mockCard{name: a.Name, role: a.Name})
	}
	return cards
}

type mockCard struct {
	name   string
	role   string
	skills []string
}

func (c *mockCard) Name() string     { return c.name }
func (c *mockCard) Role() string     { return c.role }
func (c *mockCard) Skills() []string { return c.skills }

// Spawn returns the next pre-configured MockActor.
func (b *MockBroker) Spawn(_ context.Context, config troupe.ActorConfig) (troupe.Actor, error) {
	idx := int(b.spawned.Add(1)) - 1
	if idx >= len(b.Actors) {
		return nil, fmt.Errorf("mock broker: no more actors (spawned %d, have %d)", idx+1, len(b.Actors))
	}
	b.Actors[idx].Name = config.Role
	return b.Actors[idx], nil
}

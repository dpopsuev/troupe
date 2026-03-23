package bugle

import "testing"

// Feature: ECS World Lifecycle

func TestAcceptance_CreateAgentIn2Lines(t *testing.T) {
	// Scenario: Create agent with color identity in 2 lines
	//   Given a new World
	//   When I spawn an agent and attach a ColorIdentity
	//   Then the agent is alive and the identity is queryable

	world := NewWorld()
	agent := world.Spawn()
	Attach(world, agent, ColorIdentity{
		Shade: "Indigo", Colour: "Denim", Role: "Writer", Collective: "Refactor",
	})

	if !world.Alive(agent) {
		t.Fatal("agent should be alive")
	}
	title := Get[ColorIdentity](world, agent).Title()
	if title != "Denim Writer of Indigo Refactor" {
		t.Errorf("Title = %q, want 'Denim Writer of Indigo Refactor'", title)
	}
}

func TestAcceptance_QueryActiveAgents(t *testing.T) {
	// Scenario: Query all active agents
	//   Given 3 agents, 2 with Health(Active), 1 with Health(Done)
	//   When I query for Health components
	//   Then all 3 are returned (query matches by component presence, not value)

	world := NewWorld()
	a := world.Spawn()
	b := world.Spawn()
	c := world.Spawn()
	Attach(world, a, Health{State: Active})
	Attach(world, b, Health{State: Active})
	Attach(world, c, Health{State: Done})

	ids := Query[Health](world)
	if len(ids) != 3 {
		t.Errorf("Query[Health] = %d entities, want 3", len(ids))
	}
}

func TestAcceptance_DespawnRemovesEverything(t *testing.T) {
	// Scenario: Despawn agent removes all components
	//   Given an agent with ColorIdentity + Health
	//   When despawned
	//   Then the agent is not alive and components are gone

	world := NewWorld()
	agent := world.Spawn()
	Attach(world, agent, ColorIdentity{Colour: "Denim"})
	Attach(world, agent, Health{State: Active})

	world.Despawn(agent)

	if world.Alive(agent) {
		t.Error("agent should not be alive after despawn")
	}
	if _, ok := TryGet[ColorIdentity](world, agent); ok {
		t.Error("ColorIdentity should be gone after despawn")
	}
}

// Feature: Color Identity

func TestAcceptance_HeraldicNaming(t *testing.T) {
	// Scenario: Heraldic naming — Title returns "Denim Writer of Indigo Refactor"

	c := ColorIdentity{
		Shade: "Indigo", Colour: "Denim", Role: "Writer", Collective: "Refactor",
	}
	if c.Title() != "Denim Writer of Indigo Refactor" {
		t.Errorf("Title = %q", c.Title())
	}
	if c.Label() != "[Indigo·Denim|Writer]" {
		t.Errorf("Label = %q", c.Label())
	}
	if c.Short() != "Denim" {
		t.Errorf("Short = %q", c.Short())
	}
}

func TestAcceptance_RegistryAssignsUniqueColours(t *testing.T) {
	// Scenario: 10 agents in same collective get unique colours

	reg := NewRegistry()
	seen := make(map[string]bool)

	for range 10 {
		id, err := reg.Assign("Worker", "TestCollective")
		if err != nil {
			t.Fatalf("Assign failed: %v", err)
		}
		key := id.Shade + "·" + id.Colour
		if seen[key] {
			t.Errorf("duplicate assignment: %s", key)
		}
		seen[key] = true
	}

	if reg.Active() != 10 {
		t.Errorf("Active = %d, want 10", reg.Active())
	}
}

func TestAcceptance_RegistryReleaseAndReuse(t *testing.T) {
	// Scenario: Released colour returns to pool

	reg := NewRegistry()
	id, _ := reg.Set("Azure", "Cerulean", "Coder", "TestTeam")
	reg.Release(id)

	// Should be able to assign Cerulean again
	id2, err := reg.Set("Azure", "Cerulean", "Reviewer", "TestTeam")
	if err != nil {
		t.Fatalf("re-assign after release failed: %v", err)
	}
	if id2.Colour != "Cerulean" {
		t.Errorf("expected Cerulean, got %s", id2.Colour)
	}
}

// Feature: Identity Strategy

func TestAcceptance_DefaultStrategyCreatesAgent(t *testing.T) {
	// Scenario: DefaultStrategy creates agent with identity + health

	world := NewWorld()
	reg := NewRegistry()
	strategy := NewDefaultStrategy(world, reg)

	id, err := strategy.Resolve("Coder", "Refactor")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if !world.Alive(id) {
		t.Fatal("agent should be alive")
	}

	color, ok := TryGet[ColorIdentity](world, id)
	if !ok {
		t.Fatal("agent should have ColorIdentity")
	}
	if color.Role != "Coder" {
		t.Errorf("Role = %q, want Coder", color.Role)
	}
	if color.Collective != "Refactor" {
		t.Errorf("Collective = %q, want Refactor", color.Collective)
	}

	health, ok := TryGet[Health](world, id)
	if !ok {
		t.Fatal("agent should have Health")
	}
	if health.State != Active {
		t.Errorf("Health.State = %s, want active", health.State)
	}
}

func TestAcceptance_MultipleResolvesProduceUniqueIdentities(t *testing.T) {
	// Scenario: Multiple resolves produce unique identities

	world := NewWorld()
	reg := NewRegistry()
	strategy := NewDefaultStrategy(world, reg)

	seen := make(map[string]bool)
	for range 5 {
		id, err := strategy.Resolve("Worker", "Team")
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		color := Get[ColorIdentity](world, id)
		key := color.Shade + "·" + color.Colour
		if seen[key] {
			t.Errorf("duplicate identity: %s", key)
		}
		seen[key] = true
	}
}

// Feature: README Test (from BGL-SPC-1 duality_architecture)

func TestAcceptance_READMEExample(t *testing.T) {
	// The README test: if this doesn't compile and run, the API is wrong.

	world := NewWorld()
	agent := world.Spawn()
	Attach(world, agent, ColorIdentity{
		Shade: "Indigo", Colour: "Denim", Role: "Writer", Collective: "Refactor",
	})
	Attach(world, agent, Health{State: Active})

	title := Get[ColorIdentity](world, agent).Title()
	if title != "Denim Writer of Indigo Refactor" {
		t.Errorf("README example broken: Title = %q", title)
	}
}

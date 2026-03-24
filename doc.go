// Package bugle is an ECS agent identity and coordination framework
// with A2A protocol support.
//
// Bugle provides:
//   - ECS World (world/ — entity registry, component storage, system queries)
//   - Agent identity (identity/ — AgentIdentity, ModelIdentity, Persona)
//   - Heraldic color system (palette/ — ColorIdentity, Registry, Shade/Colour)
//   - Behavioral profiles (element/ — Element, Approach)
//   - Signal bus (signal/ — Bus, DurableBus)
//   - Health tracking (root — Health, Hierarchy, Budget, Progress)
//   - A2A transport (transport/ — LocalTransport, HTTPTransport)
//   - Observable state (worldview/ — View, Snapshot, Minimap)
//
// Identity is the primitive. Protocol is the adapter.
//
// Usage:
//
//	w := world.NewWorld()
//	agent := w.Spawn()
//	world.Attach(w, agent, palette.ColorIdentity{
//	    Shade: "Indigo", Colour: "Denim", Role: "Writer", Collective: "Refactor",
//	})
//	fmt.Println(world.Get[palette.ColorIdentity](w, agent).Title())
//	// → "Denim Writer of Indigo Refactor"
package bugle

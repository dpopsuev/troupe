// Package bugle is an ECS agent identity and coordination framework
// with A2A protocol support.
//
// Bugle provides:
//   - ECS World (entity registry, component storage, system queries)
//   - Agent identity (ColorIdentity with heraldic naming)
//   - Behavioral profiles (Element, Approach)
//   - Signal bus (Bus, DurableBus)
//   - Health tracking (HealthSystem)
//   - A2A transport (LocalTransport, HTTPTransport)
//   - Observable state (WorldView, Minimap)
//
// Identity is the primitive. Protocol is the adapter.
//
// Usage:
//
//	world := bugle.NewWorld()
//	agent := world.Spawn()
//	bugle.Attach(world, agent, bugle.ColorIdentity{
//	    Shade: "Indigo", Colour: "Denim", Role: "Writer", Collective: "Refactor",
//	})
//	fmt.Println(bugle.Get[bugle.ColorIdentity](world, agent).Title())
//	// → "Denim Writer of Indigo Refactor"
package bugle

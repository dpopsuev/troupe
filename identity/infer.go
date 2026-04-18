package identity

// FromVector converts a TraitVector to a trait.Set ECS component.
func FromVector(v TraitVector) Set {
	return Set{
		{Name: Coding, Value: v.Coding},
		{Name: Reasoning, Value: v.Reasoning},
		{Name: Knowledge, Value: v.Knowledge},
		{Name: Math, Value: v.Math},
		{Name: Instruction, Value: v.Instruction},
		{Name: Agentic, Value: v.Agentic},
		{Name: Speed, Value: v.Speed},
		{Name: Cost, Value: v.Cost},
	}
}

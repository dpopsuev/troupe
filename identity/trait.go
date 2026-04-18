// Package trait provides behavioral trait primitives for agents.
// TraitVector axes map to industry LLM benchmarks — not personality types.
// Arsenal scores models by dot-product of preferences against model benchmarks.
package identity

import "github.com/dpopsuev/troupe/world"

// TraitType is the ComponentType for a trait Set.
const TraitType world.ComponentType = "trait"

// Trait is a single behavioral trait with a value between 0.0 and 1.0.
type Trait struct {
	Name  string  `json:"name"`
	Value float64 `json:"value"`
}

// Set is a collection of traits, attached as an ECS component.
type Set []Trait

// ComponentType implements world.Component.
func (Set) ComponentType() world.ComponentType { return TraitType }

// Get returns the value for a named trait, or 0.0 if not present.
func (s Set) Get(name string) float64 {
	for _, t := range s {
		if t.Name == name {
			return t.Value
		}
	}
	return 0.0
}

// Has returns true if the named trait is present.
func (s Set) Has(name string) bool {
	for _, t := range s {
		if t.Name == name {
			return true
		}
	}
	return false
}

// Benchmark-aligned trait axes.
const (
	Coding      = "coding"      // SWE-bench, HumanEval, LiveCodeBench
	Reasoning   = "reasoning"   // GPQA Diamond, BBH, MUSR
	Knowledge   = "knowledge"   // MMLU, SuperGPQA, SimpleQA
	Math        = "math"        // AIME, HMMT, MATH-500
	Instruction = "instruction" // IFEval, Chatbot Arena
	Agentic     = "agentic"     // TerminalBench, BrowseComp, tool-calling
	Speed       = "speed"       // tokens/sec throughput
	Cost        = "cost"        // inverse price (higher = cheaper)
)

// DefaultVocabulary returns the 8 benchmark-aligned trait names.
func DefaultVocabulary() []string {
	return []string{Coding, Reasoning, Knowledge, Math, Instruction, Agentic, Speed, Cost}
}

// TraitVector holds normalized trait scores (0.0-1.0) for model selection.
// Axes map to industry LLM benchmarks.
type TraitVector struct {
	Coding      float64 `yaml:"coding"      json:"coding"`
	Reasoning   float64 `yaml:"reasoning"   json:"reasoning"`
	Knowledge   float64 `yaml:"knowledge"   json:"knowledge"`
	Math        float64 `yaml:"math"        json:"math"`
	Instruction float64 `yaml:"instruction" json:"instruction"`
	Agentic     float64 `yaml:"agentic"     json:"agentic"`
	Speed       float64 `yaml:"speed"       json:"speed"`
	Cost        float64 `yaml:"cost"        json:"cost"`
}

// Score returns the dot product of this vector with a weight vector.
func (v TraitVector) Score(w TraitVector) float64 {
	return v.Coding*w.Coding +
		v.Reasoning*w.Reasoning +
		v.Knowledge*w.Knowledge +
		v.Math*w.Math +
		v.Instruction*w.Instruction +
		v.Agentic*w.Agentic +
		v.Speed*w.Speed +
		v.Cost*w.Cost
}

// MeetsMinimum returns true if every non-zero field in floor is <= the
// corresponding field in v.
func (v TraitVector) MeetsMinimum(floor TraitVector) bool {
	return (floor.Coding == 0 || v.Coding >= floor.Coding) &&
		(floor.Reasoning == 0 || v.Reasoning >= floor.Reasoning) &&
		(floor.Knowledge == 0 || v.Knowledge >= floor.Knowledge) &&
		(floor.Math == 0 || v.Math >= floor.Math) &&
		(floor.Instruction == 0 || v.Instruction >= floor.Instruction) &&
		(floor.Agentic == 0 || v.Agentic >= floor.Agentic) &&
		(floor.Speed == 0 || v.Speed >= floor.Speed) &&
		(floor.Cost == 0 || v.Cost >= floor.Cost)
}

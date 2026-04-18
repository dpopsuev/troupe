package arsenal

import (
	"errors"
	"math"
	"testing"
)

func TestTraitVector_Score(t *testing.T) {
	v := TraitVector{Speed: 0.5, Coding: 0.9}
	w := TraitVector{Speed: 2.0, Coding: 1.0}

	got := v.Score(w)
	want := 0.5*2.0 + 0.9*1.0
	if math.Abs(got-want) > 0.001 {
		t.Errorf("Score = %f, want %f", got, want)
	}
}

func TestTraitVector_MeetsMinimum(t *testing.T) {
	v := TraitVector{Coding: 0.8, Reasoning: 0.6}

	if !v.MeetsMinimum(TraitVector{Coding: 0.7}) {
		t.Error("0.8 should meet minimum 0.7")
	}
	if v.MeetsMinimum(TraitVector{Coding: 0.9}) {
		t.Error("0.8 should NOT meet minimum 0.9")
	}
	if !v.MeetsMinimum(TraitVector{}) {
		t.Error("zero minimum should always pass")
	}
}

func TestApplyMapping(t *testing.T) {
	benchmarks := map[string]float64{
		"swe_bench":  50.0,
		"human_eval": 90.0,
		"ifeval":     85.0,
	}
	mapping := TraitMapping{
		Coding:     map[string]float64{"swe_bench": 0.6, "human_eval": 0.4},
		Instruction: map[string]float64{"ifeval": 1.0},
	}

	v := ApplyMapping(benchmarks, mapping)

	wantCoding := 50.0*0.6 + 90.0*0.4 // 66.0
	if math.Abs(v.Coding-wantCoding) > 0.001 {
		t.Errorf("Coding = %f, want %f", v.Coding, wantCoding)
	}
	if math.Abs(v.Instruction-85.0) > 0.001 {
		t.Errorf("Instruction = %f, want 85.0", v.Instruction)
	}
}

func TestFilter_Matches(t *testing.T) {
	tests := []struct {
		name   string
		filter Filter
		value  string
		want   bool
	}{
		{"empty filter allows all", Filter{}, "anything", true},
		{"allow list includes", Filter{Allow: []string{"a", "b"}}, "a", true},
		{"allow list excludes", Filter{Allow: []string{"a", "b"}}, "c", false},
		{"block list blocks", Filter{Block: []string{"bad"}}, "bad", false},
		{"block list allows others", Filter{Block: []string{"bad"}}, "good", true},
		{"block wins over allow", Filter{Allow: []string{"x"}, Block: []string{"x"}}, "x", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.filter.matches(tt.value); got != tt.want {
				t.Errorf("matches(%q) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}

func TestNewArsenal_Latest(t *testing.T) {
	a, err := NewArsenal("latest")
	if err != nil {
		t.Fatalf("NewArsenal(latest): %v", err)
	}
	if a.Pin() != "2026-03" {
		t.Errorf("Pin = %q, want 2026-03", a.Pin())
	}
	if len(a.Available()) == 0 {
		t.Error("Available should not be empty")
	}
}

func TestNewArsenal_EmptyPin(t *testing.T) {
	a, err := NewArsenal("")
	if err != nil {
		t.Fatalf("NewArsenal(''): %v", err)
	}
	if a.Pin() != "2026-03" {
		t.Errorf("Pin = %q, want 2026-03 (default to latest)", a.Pin())
	}
}

func TestNewArsenal_ExplicitPin(t *testing.T) {
	a, err := NewArsenal("2026-03")
	if err != nil {
		t.Fatalf("NewArsenal(2026-03): %v", err)
	}
	if a.Pin() != "2026-03" {
		t.Errorf("Pin = %q, want 2026-03", a.Pin())
	}
}

func TestNewArsenal_BadPin(t *testing.T) {
	_, err := NewArsenal("1999-01")
	if !errors.Is(err, ErrBadPin) {
		t.Errorf("err = %v, want ErrBadPin", err)
	}
}

func TestPick_HappyPath(t *testing.T) {
	a, _ := NewArsenal("latest")

	agent, err := a.Pick("claude-opus-4-6", "claude")
	if err != nil {
		t.Fatalf("Pick: %v", err)
	}
	if agent.Model != "claude-opus-4-6" {
		t.Errorf("Model = %q", agent.Model)
	}
	if agent.Provider != "anthropic" {
		t.Errorf("Provider = %q", agent.Provider)
	}
	if agent.Source != "claude" {
		t.Errorf("Source = %q", agent.Source)
	}
	if agent.Pipeline != "direct" {
		t.Errorf("Pipeline = %q, want direct", agent.Pipeline)
	}
}

func TestPick_NotFound(t *testing.T) {
	a, _ := NewArsenal("latest")

	_, err := a.Pick("nonexistent", "claude")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestPick_SourceModifierCapsContext(t *testing.T) {
	a, _ := NewArsenal("latest")

	// Cursor caps context to 120000.
	agent, err := a.Pick("claude-opus-4-6", "cursor")
	if err != nil {
		t.Fatalf("Pick: %v", err)
	}
	if agent.EffContext != 120000 {
		t.Errorf("EffContext = %d, want 120000 (cursor cap)", agent.EffContext)
	}
	if agent.Overhead != 5.5 {
		t.Errorf("Overhead = %f, want 5.5", agent.Overhead)
	}
	if agent.Pipeline != "multi-model" {
		t.Errorf("Pipeline = %q, want multi-model", agent.Pipeline)
	}
}

func TestPick_DirectSourcePreservesContext(t *testing.T) {
	a, _ := NewArsenal("latest")

	agent, err := a.Pick("claude-opus-4-6", "claude")
	if err != nil {
		t.Fatalf("Pick: %v", err)
	}
	if agent.EffContext != 200000 {
		t.Errorf("EffContext = %d, want 200000 (model native)", agent.EffContext)
	}
}

func TestSelect_WeightsPickBest(t *testing.T) {
	a, _ := NewArsenal("latest")

	// Heavy coding weight should pick the best coding model.
	agent, err := a.Select("", &Preferences{
		Weights: TraitVector{Coding: 2.0},
	})
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	// Opus has highest raw coding benchmarks → highest normalized coding.
	if agent.Model != "claude-opus-4-6" {
		t.Errorf("Model = %q, want claude-opus-4-6 (best coding)", agent.Model)
	}
}

func TestSelect_MinTraitsGate(t *testing.T) {
	a, _ := NewArsenal("latest")

	// Set impossibly high min trait.
	_, err := a.Select("", &Preferences{
		Weights:   TraitVector{Coding: 1.0},
		MinTraits: TraitVector{Coding: 999.0},
	})
	if !errors.Is(err, ErrNoCandidate) {
		t.Errorf("err = %v, want ErrNoCandidate", err)
	}
}

func TestSelect_ProviderFilter(t *testing.T) {
	a, _ := NewArsenal("latest")

	agent, err := a.Select("", &Preferences{
		Weights:   TraitVector{Coding: 1.0},
		Providers: Filter{Allow: []string{"google"}},
	})
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if agent.Provider != "google" {
		t.Errorf("Provider = %q, want google", agent.Provider)
	}
}

func TestSelect_SourceFilter(t *testing.T) {
	a, _ := NewArsenal("latest")

	agent, err := a.Select("", &Preferences{
		Weights: TraitVector{Coding: 1.0},
		Sources: Filter{Allow: []string{"anthropic-api"}},
	})
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if agent.Source != "anthropic-api" {
		t.Errorf("Source = %q, want anthropic-api", agent.Source)
	}
}

func TestSelect_MaxCost(t *testing.T) {
	a, _ := NewArsenal("latest")

	agent, err := a.Select("", &Preferences{
		Weights: TraitVector{Coding: 1.0},
		MaxCost: 2.0, // only haiku ($0.80) and gemini ($1.25) pass
	})
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if agent.Cost.InputPerM > 2.0 {
		t.Errorf("Cost = %f, should be <= 2.0", agent.Cost.InputPerM)
	}
}

func TestSelect_BlockProvider(t *testing.T) {
	a, _ := NewArsenal("latest")

	agent, err := a.Select("", &Preferences{
		Weights:   TraitVector{Coding: 1.0},
		Providers: Filter{Block: []string{"anthropic"}},
	})
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if agent.Provider == "anthropic" {
		t.Error("anthropic should be blocked")
	}
}

func TestNormalize_BestIs1(t *testing.T) {
	a, _ := NewArsenal("latest")
	snap := a.snapshots[a.active]

	// Find max coding among all models.
	var maxCoding float64
	for _, m := range snap.Models {
		if m.Traits.Coding > maxCoding {
			maxCoding = m.Traits.Coding
		}
	}
	if math.Abs(maxCoding-1.0) > 0.001 {
		t.Errorf("max coding = %f, want 1.0 (best model should be 1.0)", maxCoding)
	}
}

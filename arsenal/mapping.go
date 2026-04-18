package arsenal

// TraitMapping defines how raw benchmark scores map to the 8 trait axes.
// Each trait has a map of benchmark_name → weight (weights should sum to 1.0).
type TraitMapping struct {
	Coding      map[string]float64 `yaml:"coding"`
	Reasoning   map[string]float64 `yaml:"reasoning"`
	Knowledge   map[string]float64 `yaml:"knowledge"`
	Math        map[string]float64 `yaml:"math"`
	Instruction map[string]float64 `yaml:"instruction"`
	Agentic     map[string]float64 `yaml:"agentic"`
	Speed       map[string]float64 `yaml:"speed"`
	Cost        map[string]float64 `yaml:"cost"`
}

// ApplyMapping converts raw benchmark scores to a TraitVector using weighted sums.
func ApplyMapping(benchmarks map[string]float64, m TraitMapping) TraitVector {
	return TraitVector{
		Coding:      weightedSum(benchmarks, m.Coding),
		Reasoning:   weightedSum(benchmarks, m.Reasoning),
		Knowledge:   weightedSum(benchmarks, m.Knowledge),
		Math:        weightedSum(benchmarks, m.Math),
		Instruction: weightedSum(benchmarks, m.Instruction),
		Agentic:     weightedSum(benchmarks, m.Agentic),
		Speed:       weightedSum(benchmarks, m.Speed),
		Cost:        weightedSum(benchmarks, m.Cost),
	}
}

func weightedSum(benchmarks, weights map[string]float64) float64 {
	var sum float64
	for name, weight := range weights {
		sum += benchmarks[name] * weight
	}
	return sum
}

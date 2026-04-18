package arsenal

import (
	"embed"
	"fmt"
	"io/fs"
	"math"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

//go:embed catalog
var catalogFS embed.FS

// providerFile is the YAML structure for a provider manifest.
type providerFile struct {
	Provider string       `yaml:"provider"`
	Shade    string       `yaml:"shade,omitempty"` // canonical shade family
	Models   []ModelEntry `yaml:"models"`
}

// Snapshot is a self-contained catalog at a point in time.
type Snapshot struct {
	Name         string
	Models       map[string]*ModelEntry  // by model ID
	Sources      map[string]*SourceEntry // by source name
	Mapping      TraitMapping
	discoveryRan bool // true after MergeDiscovery has been called
}

// loadSnapshot parses a snapshot directory from the embedded FS.
func loadSnapshot(name string) (*Snapshot, error) {
	snap := &Snapshot{
		Name:    name,
		Models:  make(map[string]*ModelEntry),
		Sources: make(map[string]*SourceEntry),
	}

	base := filepath.Join("catalog", name)

	// Load trait mapping.
	mappingData, err := catalogFS.ReadFile(filepath.Join(base, "trait_mapping.yaml"))
	if err != nil {
		return nil, fmt.Errorf("arsenal: read trait_mapping: %w", err)
	}
	if err := yaml.Unmarshal(mappingData, &snap.Mapping); err != nil {
		return nil, fmt.Errorf("arsenal: parse trait_mapping: %w", err)
	}

	// Load providers.
	providerDir := filepath.Join(base, "providers")
	providerEntries, err := catalogFS.ReadDir(providerDir)
	if err != nil {
		return nil, fmt.Errorf("arsenal: read providers dir: %w", err)
	}
	for _, entry := range providerEntries {
		if entry.IsDir() {
			continue
		}
		data, err := catalogFS.ReadFile(filepath.Join(providerDir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("arsenal: read provider %s: %w", entry.Name(), err)
		}
		var pf providerFile
		if err := yaml.Unmarshal(data, &pf); err != nil {
			return nil, fmt.Errorf("arsenal: parse provider %s: %w", entry.Name(), err)
		}
		for i := range pf.Models {
			pf.Models[i].Provider = pf.Provider
			model := pf.Models[i]
			snap.Models[model.ID] = &model
		}
	}

	// Load sources.
	sourceDir := filepath.Join(base, "sources")
	sourceEntries, err := catalogFS.ReadDir(sourceDir)
	if err != nil {
		return nil, fmt.Errorf("arsenal: read sources dir: %w", err)
	}
	for _, entry := range sourceEntries {
		if entry.IsDir() {
			continue
		}
		data, err := catalogFS.ReadFile(filepath.Join(sourceDir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("arsenal: read source %s: %w", entry.Name(), err)
		}
		var se SourceEntry
		if err := yaml.Unmarshal(data, &se); err != nil {
			return nil, fmt.Errorf("arsenal: parse source %s: %w", entry.Name(), err)
		}
		snap.Sources[se.Source] = &se

		// Source-owned models (e.g. Cursor → Composer).
		for i := range se.Models {
			model := se.Models[i]
			if _, exists := snap.Models[model.ID]; !exists {
				snap.Models[model.ID] = &model
			}
		}
	}

	// Apply trait mapping + normalize.
	for _, model := range snap.Models {
		model.Traits = ApplyMapping(model.Benchmarks, snap.Mapping)
	}
	snap.normalize()

	return snap, nil
}

// normalize applies min-max normalization per trait across all models.
// Best model = 1.0, worst = 0.0 for each trait independently.
func (s *Snapshot) normalize() {
	if len(s.Models) == 0 {
		return
	}

	// Find min/max per trait.
	minV := TraitVector{
		Speed: math.MaxFloat64, Reasoning: math.MaxFloat64, Knowledge: math.MaxFloat64,
		Coding: math.MaxFloat64, Instruction: math.MaxFloat64, Agentic: math.MaxFloat64,
		Math: math.MaxFloat64, Cost: math.MaxFloat64,
	}
	var maxV TraitVector

	for _, m := range s.Models {
		t := m.Traits
		minV.Speed = min(minV.Speed, t.Speed)
		minV.Reasoning = min(minV.Reasoning, t.Reasoning)
		minV.Knowledge = min(minV.Knowledge, t.Knowledge)
		minV.Coding = min(minV.Coding, t.Coding)
		minV.Instruction = min(minV.Instruction, t.Instruction)
		minV.Agentic = min(minV.Agentic, t.Agentic)
		minV.Math = min(minV.Math, t.Math)
		minV.Cost = min(minV.Cost, t.Cost)

		maxV.Speed = max(maxV.Speed, t.Speed)
		maxV.Reasoning = max(maxV.Reasoning, t.Reasoning)
		maxV.Knowledge = max(maxV.Knowledge, t.Knowledge)
		maxV.Coding = max(maxV.Coding, t.Coding)
		maxV.Instruction = max(maxV.Instruction, t.Instruction)
		maxV.Agentic = max(maxV.Agentic, t.Agentic)
		maxV.Math = max(maxV.Math, t.Math)
		maxV.Cost = max(maxV.Cost, t.Cost)
	}

	// Normalize each model's traits to 0.0-1.0.
	for _, m := range s.Models {
		m.Traits.Speed = normField(m.Traits.Speed, minV.Speed, maxV.Speed)
		m.Traits.Reasoning = normField(m.Traits.Reasoning, minV.Reasoning, maxV.Reasoning)
		m.Traits.Knowledge = normField(m.Traits.Knowledge, minV.Knowledge, maxV.Knowledge)
		m.Traits.Coding = normField(m.Traits.Coding, minV.Coding, maxV.Coding)
		m.Traits.Instruction = normField(m.Traits.Instruction, minV.Instruction, maxV.Instruction)
		m.Traits.Agentic = normField(m.Traits.Agentic, minV.Agentic, maxV.Agentic)
		m.Traits.Math = normField(m.Traits.Math, minV.Math, maxV.Math)
		m.Traits.Cost = normField(m.Traits.Cost, minV.Cost, maxV.Cost)
	}
}

func normField(val, minVal, maxVal float64) float64 {
	rng := maxVal - minVal
	if rng == 0 {
		return 1.0 // all models have the same value
	}
	return (val - minVal) / rng
}

// availableSnapshots returns snapshot names sorted alphabetically (latest last).
func availableSnapshots() ([]string, error) {
	entries, err := catalogFS.ReadDir("catalog")
	if err != nil {
		return nil, fmt.Errorf("arsenal: read catalog dir: %w", err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

// canAccess returns true if the source can reach the given model.
func canAccess(source *SourceEntry, modelID string) bool {
	// Source's own models.
	for i := range source.Models {
		if source.Models[i].ID == modelID {
			return true
		}
	}
	// Routed models.
	for _, id := range source.Access {
		if id == modelID {
			return true
		}
	}
	return false
}

// Compile-time check: catalogFS must satisfy fs.FS.
var _ fs.FS = catalogFS

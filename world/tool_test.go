package world

import "testing"

func TestToolEntity_E2E_RegisterAndQuery(t *testing.T) {
	w := NewWorld()
	RegisterTool(w, ToolCard{Name: "oculus", Capabilities: []string{"code-analysis", "symbol-graph"}})
	RegisterTool(w, ToolCard{Name: "bash", Capabilities: []string{"shell"}})
	RegisterTool(w, ToolCard{Name: "gotools", Capabilities: []string{"code-analysis", "go-specific"}})

	// Query by capability.
	codeTools := QueryToolsByCapability(w, "code-analysis")
	if len(codeTools) != 2 {
		t.Fatalf("code-analysis tools = %d, want 2", len(codeTools))
	}

	shellTools := QueryToolsByCapability(w, "shell")
	if len(shellTools) != 1 {
		t.Errorf("shell tools = %d, want 1", len(shellTools))
	}
	if shellTools[0].Name != "bash" {
		t.Errorf("shell tool name = %q, want bash", shellTools[0].Name)
	}

	// No match.
	none := QueryToolsByCapability(w, "nonexistent")
	if len(none) != 0 {
		t.Errorf("nonexistent = %d, want 0", len(none))
	}
}

func TestToolEntity_AllTools(t *testing.T) {
	w := NewWorld()
	RegisterTool(w, ToolCard{Name: "read"})
	RegisterTool(w, ToolCard{Name: "write"})

	all := AllTools(w)
	if len(all) != 2 {
		t.Fatalf("AllTools = %d, want 2", len(all))
	}
}

func TestToolEntity_EmptyWorld(t *testing.T) {
	w := NewWorld()
	if tools := AllTools(w); len(tools) != 0 {
		t.Errorf("empty world tools = %d, want 0", len(tools))
	}
}

func TestToolCard_ComponentType(t *testing.T) {
	tc := ToolCard{Name: "test"}
	if tc.ComponentType() != ToolCardType {
		t.Errorf("ComponentType = %q, want %q", tc.ComponentType(), ToolCardType)
	}
}

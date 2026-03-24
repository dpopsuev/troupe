package identity

import "testing"

func TestAgentIdentity_Tag(t *testing.T) {
	cases := []struct {
		name string
		id   AgentIdentity
		want string
	}{
		{
			name: "normal",
			id:   AgentIdentity{PersonaName: "Herald", Color: Color{Name: "Crimson"}},
			want: "[crimson/herald]",
		},
		{
			name: "empty color",
			id:   AgentIdentity{PersonaName: "Herald"},
			want: "[none/herald]",
		},
		{
			name: "empty name",
			id:   AgentIdentity{Color: Color{Name: "Crimson"}},
			want: "[crimson/anon]",
		},
		{
			name: "both empty",
			id:   AgentIdentity{},
			want: "[none/anon]",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.id.Tag()
			if got != tc.want {
				t.Errorf("Tag() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestHomeZoneFor(t *testing.T) {
	cases := []struct {
		pos  Position
		want MetaPhase
	}{
		{PositionPG, MetaPhaseBk},
		{PositionSG, MetaPhasePt},
		{PositionPF, MetaPhaseFc},
		{PositionC, MetaPhaseFc},
		{"unknown", ""},
	}
	for _, tc := range cases {
		got := HomeZoneFor(tc.pos)
		if got != tc.want {
			t.Errorf("HomeZoneFor(%q) = %q, want %q", tc.pos, got, tc.want)
		}
	}
}

func TestAgentIdentity_IsRole(t *testing.T) {
	id := AgentIdentity{Role: RoleWorker}
	if !id.IsRole(RoleWorker) {
		t.Error("IsRole(worker) should be true")
	}
	if id.IsRole(RoleManager) {
		t.Error("IsRole(manager) should be false")
	}
}

func TestAgentIdentity_HasRole(t *testing.T) {
	noRole := AgentIdentity{}
	if noRole.HasRole() {
		t.Error("HasRole() should be false for empty role")
	}

	withRole := AgentIdentity{Role: RoleEnforcer}
	if !withRole.HasRole() {
		t.Error("HasRole() should be true for assigned role")
	}
}

func TestModelIdentity_String(t *testing.T) {
	cases := []struct {
		name string
		m    ModelIdentity
		want string
	}{
		{
			name: "full",
			m:    ModelIdentity{ModelName: "claude-4", Provider: "anthropic", Version: "2026-03"},
			want: "claude-4@2026-03/anthropic",
		},
		{
			name: "no version",
			m:    ModelIdentity{ModelName: "gpt-5", Provider: "openai"},
			want: "gpt-5/openai",
		},
		{
			name: "with wrapper",
			m:    ModelIdentity{ModelName: "claude-4", Provider: "anthropic", Wrapper: "cursor"},
			want: "claude-4/anthropic (via cursor)",
		},
		{
			name: "empty",
			m:    ModelIdentity{},
			want: "unknown/unknown",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.m.String()
			if got != tc.want {
				t.Errorf("String() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestModelIdentity_Tag(t *testing.T) {
	m := ModelIdentity{ModelName: "claude-4-sonnet"}
	got := m.Tag()
	if got != "[claude-4-sonnet]" {
		t.Errorf("Tag() = %q, want [claude-4-sonnet]", got)
	}

	long := ModelIdentity{ModelName: "a-very-long-model-name-that-exceeds-twenty"}
	got = long.Tag()
	if len(got) > 22 { // "[" + 20 + "]"
		t.Errorf("Tag() too long: %q (%d chars)", got, len(got))
	}
}

func TestValidRoles(t *testing.T) {
	for _, r := range []Role{RoleWorker, RoleManager, RoleEnforcer, RoleBroker} {
		if !ValidRoles[r] {
			t.Errorf("ValidRoles[%q] should be true", r)
		}
	}
	if ValidRoles["invalid"] {
		t.Error("ValidRoles should not contain invalid roles")
	}
}

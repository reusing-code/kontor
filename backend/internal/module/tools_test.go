package module

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	"github.com/reusing-code/kontor/backend/internal/mcptool"
)

type plainModule struct {
	Base
	id string
}

func (m plainModule) ID() string                                  { return m.id }
func (m plainModule) Prefix(userID string) []byte                 { return Prefix(userID, m.id) }
func (plainModule) RegisterRoutes(*Router)                        {}
func (plainModule) IsEmpty(context.Context, string) (bool, error) { return true, nil }
func (plainModule) ExportSection(context.Context, string) (json.RawMessage, error) {
	return nil, nil
}
func (plainModule) ImportSection(context.Context, string, json.RawMessage, *ImportResult) error {
	return nil
}

type toolModule struct {
	plainModule
	tool string
}

func (m toolModule) RegisterTools(r *mcptool.Registrar) {
	mcptool.Add(r, mcptool.Spec{Name: m.tool, Scope: mcptool.ScopeRead, Module: m.id},
		func(context.Context, mcptool.Call, struct{}) (struct{}, error) { return struct{}{}, nil })
}

func TestRegistry_RegisterToolsKeepsOrderAndSkipsNonProviders(t *testing.T) {
	reg := NewRegistry(
		toolModule{plainModule{id: "a"}, "tool_a"},
		plainModule{id: "b"},
		toolModule{plainModule{id: "c"}, "tool_c"},
	)
	r := mcptool.NewRegistrar(mcptool.Env{})
	reg.RegisterTools(r)

	var names []string
	for _, s := range r.Specs() {
		names = append(names, s.Name)
	}
	if !slices.Equal(names, []string{"tool_a", "tool_c"}) {
		t.Fatalf("tools = %v", names)
	}
}

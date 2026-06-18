package engine

import (
	"context"
	"testing"
)

type fakeEngine struct{ ran bool }

func (f *fakeEngine) Spec() Spec {
	return Spec{
		ID: "fake", Name: "Fake", Description: "d",
		Triggers:   []Trigger{TriggerOnDemand},
		Config:     []ConfigField{{Key: "limiar", Label: "Limiar", Type: "number", Default: "5"}},
		OutputType: "suggestion", DefaultOn: false,
	}
}
func (f *fakeEngine) Run(_ context.Context, _ RunContext) error { f.ran = true; return nil }

func TestRegistryListGetDefaults(t *testing.T) {
	r := NewRegistry()
	fe := &fakeEngine{}
	r.Register(fe)

	if got := r.List(); len(got) != 1 || got[0].ID != "fake" {
		t.Fatalf("List=%+v", got)
	}
	e, ok := r.Get("fake")
	if !ok || e != fe {
		t.Fatalf("Get falhou: %v %v", e, ok)
	}
	if _, ok := r.Get("nope"); ok {
		t.Fatal("Get devolveu motor inexistente")
	}
	def := r.Defaults("fake")
	if def["limiar"] != "5" || def["__enabled"] != "false" || def["__trigger"] != "on_demand" {
		t.Fatalf("Defaults=%v", def)
	}
}

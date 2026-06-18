package store

import "testing"

func TestEngineConfigResolveLayers(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProject("App", "")
	defaults := map[string]string{"limiar": "5", "__enabled": "false"}

	// só defaults
	got, err := s.ResolveEngineConfig("m", p.ID, defaults)
	if err != nil {
		t.Fatal(err)
	}
	if got["limiar"] != "5" || got["__enabled"] != "false" {
		t.Fatalf("defaults: %v", got)
	}

	// global liga o motor e muda o limiar
	_ = s.SetEngineConfig("m", "__enabled", "true", "")
	_ = s.SetEngineConfig("m", "limiar", "8", "")
	got, _ = s.ResolveEngineConfig("m", p.ID, defaults)
	if got["__enabled"] != "true" || got["limiar"] != "8" {
		t.Fatalf("global: %v", got)
	}

	// override do projeto vence o global no limiar, mantém __enabled global
	_ = s.SetEngineConfig("m", "limiar", "20", p.ID)
	got, _ = s.ResolveEngineConfig("m", p.ID, defaults)
	if got["limiar"] != "20" || got["__enabled"] != "true" {
		t.Fatalf("override: %v", got)
	}

	// upsert: regravar a mesma chave atualiza, não duplica
	_ = s.SetEngineConfig("m", "limiar", "21", p.ID)
	raw, _ := s.GetEngineConfig("m", p.ID)
	if raw["limiar"] != "21" {
		t.Fatalf("upsert: %v", raw)
	}
}

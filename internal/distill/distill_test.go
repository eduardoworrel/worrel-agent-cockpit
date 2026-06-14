package distill

import (
	"strings"
	"testing"
)

func TestParseCandidates(t *testing.T) {
	raw := "```json\n[" +
		`{"type":"skill.learned","title":"Deploy","name":"Deploy","content":"# x","evidence":"trecho"},` +
		`{"type":"lixo","title":"ignora"},` +
		`{"type":"skill.correction","title":"sem id"}` +
		"]\n```"
	cands, dropped := ParseCandidates(raw)
	if len(cands) != 1 {
		t.Fatalf("válidos = %d, want 1: %+v", len(cands), cands)
	}
	if dropped != 2 {
		t.Fatalf("descartados = %d, want 2", dropped)
	}
	if cands[0].Title != "Deploy" {
		t.Fatalf("cand %+v", cands[0])
	}
}

func TestValidRequiresEvidence(t *testing.T) {
	// sem evidence → inválido
	c := Candidate{Type: "skill.learned", Title: "T", Name: "N", Content: "C", Evidence: ""}
	if c.valid() {
		t.Fatal("candidato sem evidence deve ser inválido")
	}
	// com evidence → válido
	c2 := Candidate{Type: "skill.learned", Title: "T", Name: "N", Content: "C", Evidence: "trecho de prova"}
	if !c2.valid() {
		t.Fatal("candidato com evidence deve ser válido")
	}
}

func TestAddMemoryValidation(t *testing.T) {
	// add_memory exige title + evidence + content (ou description)
	bad := Candidate{Type: "add_memory", Title: "", Evidence: "e", Content: "c"}
	if bad.valid() {
		t.Fatal("add_memory sem title deve ser inválido")
	}
	bad2 := Candidate{Type: "add_memory", Title: "T", Evidence: "", Content: "c"}
	if bad2.valid() {
		t.Fatal("add_memory sem evidence deve ser inválido")
	}
	bad3 := Candidate{Type: "add_memory", Title: "T", Evidence: "e", Content: "", Description: ""}
	if bad3.valid() {
		t.Fatal("add_memory sem content/description deve ser inválido")
	}
	ok := Candidate{Type: "add_memory", Title: "Convenção", Evidence: "repetida em 3 sessões", Content: "usar X"}
	if !ok.valid() {
		t.Fatal("add_memory com content deve ser válido")
	}
	okDesc := Candidate{Type: "add_memory", Title: "Convenção", Evidence: "e", Description: "usar X"}
	if !okDesc.valid() {
		t.Fatal("add_memory com description deve ser válido")
	}
}

func TestAddMemoryPayloadCarriesContent(t *testing.T) {
	// O applier lê p.Content; quando só há description, o payload deve preenchê-lo.
	c := Candidate{Type: "add_memory", Title: "T", Evidence: "e", Description: "decisão Y"}
	pl := candidatePayload(c)
	if !strings.Contains(pl, "decisão Y") {
		t.Fatalf("payload deve conter o conteúdo da memória: %s", pl)
	}
}

func TestParseHandlesPlainArray(t *testing.T) {
	cands, _ := ParseCandidates(`[{"type":"skill.variant","title":"V","name":"V","content":"c","evidence":"e"}]`)
	if len(cands) != 1 {
		t.Fatalf("got %d", len(cands))
	}
}

func TestCreateProjectRequiresDescription(t *testing.T) {
	// create_project sem description não-vazia → inválido
	c := Candidate{Type: "create_project", Title: "T", Evidence: "e", Description: ""}
	if c.valid() {
		t.Fatal("create_project sem description deve ser inválido")
	}
	// create_project com description preenchida → válido
	c2 := Candidate{Type: "create_project", Title: "T", Evidence: "e", Description: "Projeto de automação"}
	if !c2.valid() {
		t.Fatal("create_project com description deve ser válido")
	}
}

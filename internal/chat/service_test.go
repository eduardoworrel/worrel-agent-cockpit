package chat

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

// fakeHeadless captura o prompt recebido e devolve uma resposta fixa.
type fakeHeadless struct {
	gotPrompt string
	reply     string
}

func (f *fakeHeadless) RunHeadless(_ context.Context, prompt string, _ adapter.HeadlessOpts) (string, error) {
	f.gotPrompt = prompt
	return f.reply, nil
}

func TestSendMessageCreatesPipelineAndMemoryAndRedacts(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer st.Close()

	// Skill existente para as etapas do pipeline.
	proj, err := st.CreateProject("Proj", "")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	sk, err := st.CreateSkill(proj.ID, "Extrair PDF", "passos...")
	if err != nil {
		t.Fatalf("CreateSkill: %v", err)
	}

	// Sessão com transcript contendo um segredo cru.
	const secret = "ghp_0123456789abcdefghijABCDEF"
	sess, err := st.CreateSession(&store.Session{Adapter: "claudecode", Mode: "observed", Title: "s"})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if err := st.AppendTranscriptEvent(sess.ID, "user", "msg",
		"use o token "+secret+" para extrair o pdf da nota fiscal", 0, 0); err != nil {
		t.Fatalf("AppendTranscriptEvent: %v", err)
	}

	// Resposta do LLM: texto + array JSON com 1 pipeline (2 etapas válidas) + 1 memória.
	reply := fmt.Sprintf("Aqui está minha análise sobre o fluxo.\n[%s,%s]",
		fmt.Sprintf(`{"type":"pipeline","name":"Fluxo NF","title":"Fluxo NF","steps":[`+
			`{"skill_id":%q,"note":"extrai"},{"skill_id":%q,"note":"valida"}],"evidence":"sessão s"}`, sk.ID, sk.ID),
		`{"type":"add_memory","content":"sempre validar o CNPJ","title":"Validar CNPJ","evidence":"sessão s"}`)

	fh := &fakeHeadless{reply: reply}
	svc := NewService(st, nil, fh, nil)

	th, err := st.CreateChatThread("{}", "", "", "chat")
	if err != nil {
		t.Fatalf("CreateChatThread: %v", err)
	}

	assistant, sources, ids, err := svc.SendMessage(context.Background(), th.ID, "como extrair pdf?", "", "")
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	// Segredo NÃO pode aparecer cru no prompt.
	if strings.Contains(fh.gotPrompt, secret) {
		t.Fatalf("segredo cru vazou no prompt")
	}
	if !strings.Contains(fh.gotPrompt, "ghp_****CDEF") {
		t.Fatalf("máscara esperada não encontrada no prompt:\n%s", fh.gotPrompt)
	}

	// Texto do assistant não deve conter o array JSON.
	if strings.Contains(assistant, `"type":"pipeline"`) {
		t.Fatalf("array JSON vazou no texto do assistant: %q", assistant)
	}
	if !strings.Contains(assistant, "análise") {
		t.Fatalf("texto do assistant inesperado: %q", assistant)
	}

	// Fonte recuperada deve incluir a sessão.
	if len(sources) != 1 || sources[0].SessionID != sess.ID {
		t.Fatalf("sources inesperado: %+v", sources)
	}

	// Deve ter criado 2 sugestões (pipeline + memória), ambas origin=chat.
	if len(ids) != 2 {
		t.Fatalf("esperava 2 sugestões, veio %d (%v)", len(ids), ids)
	}
	var pipe, mem int
	for _, id := range ids {
		sg, err := st.GetSuggestion(id)
		if err != nil {
			t.Fatalf("GetSuggestion: %v", err)
		}
		if sg.Origin != "chat" {
			t.Fatalf("origin esperado chat, veio %q", sg.Origin)
		}
		switch sg.Type {
		case "pipeline":
			pipe++
			if !strings.Contains(sg.Payload, sk.ID) || !strings.Contains(sg.Payload, "Fluxo NF") {
				t.Fatalf("payload pipeline inesperado: %s", sg.Payload)
			}
		case "add_memory":
			mem++
		}
	}
	if pipe != 1 || mem != 1 {
		t.Fatalf("esperava 1 pipeline + 1 memória, veio pipe=%d mem=%d", pipe, mem)
	}
}

func TestPipelineDroppedWhenFewerThanTwoValidSteps(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer st.Close()
	proj, _ := st.CreateProject("P", "")
	sk, _ := st.CreateSkill(proj.ID, "S", "c")

	// pipeline com 1 etapa válida + 1 órfã → descartado.
	reply := fmt.Sprintf("texto\n[{\"type\":\"pipeline\",\"name\":\"P\",\"title\":\"P\","+
		"\"steps\":[{\"skill_id\":%q},{\"skill_id\":\"inexistente\"}],\"evidence\":\"e\"}]", sk.ID)
	fh := &fakeHeadless{reply: reply}
	svc := NewService(st, nil, fh, nil)
	th, _ := st.CreateChatThread("{}", "", "", "")
	_, _, ids, err := svc.SendMessage(context.Background(), th.ID, "x", "", "")
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("pipeline com <2 etapas válidas não deveria criar sugestão, veio %v", ids)
	}
}

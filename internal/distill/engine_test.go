package distill

import (
	"context"
	"testing"
	"time"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

type fakeAdapter struct {
	out    string
	events []adapter.TranscriptEvent
}

func (f *fakeAdapter) RunHeadless(_ context.Context, _ string, _ adapter.HeadlessOpts) (string, error) {
	return f.out, nil
}
func (f *fakeAdapter) ReadTranscript(_ adapter.SessionRef) ([]adapter.TranscriptEvent, error) {
	return f.events, nil
}

func TestSweepCreatesTypedSuggestion(t *testing.T) {
	s, _ := store.Open(t.TempDir() + "/t.db")
	defer s.Close()
	p, _ := s.CreateProject("App", "")
	sess, _ := s.CreateSession(&store.Session{ProjectID: p.ID, Adapter: "claude-code", Mode: "observed"})
	s.EndSession(sess.ID)
	s.AppendTranscriptEvent(sess.ID, "user", "text", "como faço deploy?", 0, 0)
	s.AppendTranscriptEvent(sess.ID, "assistant", "text", "houve um erro no build; repetimos os passos de deploy de staging ate ficar verde, vale virar skill", 0, 0)

	fake := &fakeAdapter{out: `[{"type":"skill.learned","title":"Deploy","name":"Deploy",` +
		`"content":"# passos","evidence":"como faço deploy?","project_id":"` + p.ID + `"}]`}
	b := bus.New()
	ch, cancel := b.Subscribe()
	defer cancel()
	eng := New(s, fake, b)

	res, err := eng.Sweep(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Created != 1 {
		t.Fatalf("criadas = %d", res.Created)
	}
	pend, _ := s.ListSuggestions("", "pending")
	if len(pend) != 1 || pend[0].Type != "skill.learned" || pend[0].Evidence == "" {
		t.Fatalf("sugestões %+v", pend)
	}
	// sessão marcada analisada
	got, _ := s.GetSession(sess.ID)
	if got.AnalyzedAt == nil {
		t.Fatal("sessão não marcada analyzed_at")
	}
	// eventos no bus
	assertEvent(t, ch, "sweep.started")
	assertEvent(t, ch, "sweep.finished")
}

func TestSweepDropsDanglingSkillRef(t *testing.T) {
	s, _ := store.Open(t.TempDir() + "/t.db")
	defer s.Close()
	p, _ := s.CreateProject("App", "")
	sess, _ := s.CreateSession(&store.Session{ProjectID: p.ID, Adapter: "claude-code", Mode: "observed"})
	s.EndSession(sess.ID)
	s.AppendTranscriptEvent(sess.ID, "user", "text", "como faço deploy?", 0, 0)
	s.AppendTranscriptEvent(sess.ID, "assistant", "text", "houve um erro no build; repetimos os passos de deploy de staging ate ficar verde, vale virar skill", 0, 0)

	// correction aponta para um skill_id inexistente (alucinação do LLM sobre
	// base recém-criada) → deve ser descartada, sem FK morta nem erro de insert.
	fake := &fakeAdapter{out: `[{"type":"skill.correction","title":"corrige deploy","content":"# fix",` +
		`"skill_id":"nao-existe-123","evidence":"como faço deploy?"}]`}
	eng := New(s, fake, bus.New())
	res, err := eng.Sweep(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Created != 0 || res.Dropped != 1 {
		t.Fatalf("esperava 0 criadas / 1 descartada, veio %+v", res)
	}
	if pend, _ := s.ListSuggestions("", "pending"); len(pend) != 0 {
		t.Fatalf("nenhuma sugestão deveria ser inserida, veio %+v", pend)
	}
}

func TestSweepDedupesAgainstPending(t *testing.T) {
	s, _ := store.Open(t.TempDir() + "/t.db")
	defer s.Close()
	p, _ := s.CreateProject("App", "")
	s.CreateSuggestion(&store.Suggestion{ProjectID: p.ID, Type: "skill.learned", Title: "Deploy staging"})
	sess, _ := s.CreateSession(&store.Session{ProjectID: p.ID, Adapter: "claude-code", Mode: "observed"})
	s.EndSession(sess.ID)
	s.AppendTranscriptEvent(sess.ID, "user", "text", "preciso automatizar o deploy de staging com varios passos repetidos toda vez", 0, 0)
	s.AppendTranscriptEvent(sess.ID, "assistant", "text", "erro encontrado no passo de build; vamos corrigir e tentar novamente ate ficar verde", 0, 0)
	fake := &fakeAdapter{out: `[{"type":"skill.learned","title":"deploy staging","name":"D","content":"c","evidence":"e","project_id":"` + p.ID + `"}]`}
	eng := New(s, fake, bus.New())
	res, _ := eng.Sweep(context.Background())
	if res.Created != 0 || res.Duplicates != 1 {
		t.Fatalf("res %+v", res)
	}
}

// TestSweepLearnedMatchingRecentSkillBecomesVariant: skill.learned que faz Jaccard match
// com skill recente deve ser convertido para skill.variant (não descartado).
func TestSweepLearnedMatchingRecentSkillBecomesVariant(t *testing.T) {
	s, _ := store.Open(t.TempDir() + "/t.db")
	defer s.Close()
	p, _ := s.CreateProject("App", "")
	// Skill recente com nome "Deploy staging"
	sk, _ := s.CreateSkill(p.ID, "Deploy staging", "# old")
	// Forçar updated_at recente (< 24h): store.RecentlyUpdatedSkills usa updated_at
	s.DB().Exec(`UPDATE skills SET updated_at=? WHERE id=?`, time.Now().UnixMilli(), sk.ID)

	sess, _ := s.CreateSession(&store.Session{ProjectID: p.ID, Adapter: "claude-code", Mode: "observed"})
	s.EndSession(sess.ID)
	s.AppendTranscriptEvent(sess.ID, "user", "text", "preciso automatizar o deploy de staging com varios passos repetidos toda vez", 0, 0)
	s.AppendTranscriptEvent(sess.ID, "assistant", "text", "erro encontrado no passo de build; vamos corrigir e tentar novamente ate ficar verde", 0, 0)

	// candidato skill.learned que faz match com "Deploy staging"
	fake := &fakeAdapter{out: `[{"type":"skill.learned","title":"deploy staging","name":"deploy staging","content":"# new","evidence":"e","project_id":"` + p.ID + `"}]`}
	eng := New(s, fake, bus.New())
	res, _ := eng.Sweep(context.Background())

	// Deve criar 1 sugestão (como skill.variant), não descartar
	if res.Created != 1 {
		t.Fatalf("res.Created=%d (want 1 — deve virar variant), Duplicates=%d", res.Created, res.Duplicates)
	}
	if res.Duplicates != 0 {
		t.Fatalf("Duplicates=%d, want 0", res.Duplicates)
	}
	pend, _ := s.ListSuggestions("", "pending")
	if len(pend) != 1 || pend[0].Type != "skill.variant" {
		t.Fatalf("sugestão type=%q, want skill.variant", pend[0].Type)
	}
}

// TestSweepVariantMatchingTargetIsNotDuplicate: skill.variant que faz match com sua skill-alvo
// NÃO deve ser descartado como duplicata.
func TestSweepVariantMatchingTargetIsNotDuplicate(t *testing.T) {
	s, _ := store.Open(t.TempDir() + "/t.db")
	defer s.Close()
	p, _ := s.CreateProject("App", "")
	sk, _ := s.CreateSkill(p.ID, "Deploy staging", "# old")
	s.DB().Exec(`UPDATE skills SET updated_at=? WHERE id=?`, time.Now().UnixMilli(), sk.ID)

	sess, _ := s.CreateSession(&store.Session{ProjectID: p.ID, Adapter: "claude-code", Mode: "observed"})
	s.EndSession(sess.ID)
	s.AppendTranscriptEvent(sess.ID, "user", "text", "preciso automatizar o deploy de staging com varios passos repetidos toda vez", 0, 0)
	s.AppendTranscriptEvent(sess.ID, "assistant", "text", "erro encontrado no passo de build; vamos corrigir e tentar novamente ate ficar verde", 0, 0)

	// candidato já é skill.variant com skill_id apontando para sk.ID
	fake := &fakeAdapter{out: `[{"type":"skill.variant","title":"deploy staging variante","name":"deploy staging variante","content":"# var","evidence":"e","skill_id":"` + sk.ID + `","project_id":"` + p.ID + `"}]`}
	eng := New(s, fake, bus.New())
	res, _ := eng.Sweep(context.Background())

	if res.Created != 1 {
		t.Fatalf("res.Created=%d, want 1 (variant não deve ser duplicata da skill alvo)", res.Created)
	}
	if res.Duplicates != 0 {
		t.Fatalf("Duplicates=%d, want 0", res.Duplicates)
	}
}

// TestSweepVariantDuplicatePendingIsDropped: skill.variant cujo título faz Jaccard match
// com sugestão PENDING já existente DEVE ser descartada.
func TestSweepVariantDuplicatePendingIsDropped(t *testing.T) {
	s, _ := store.Open(t.TempDir() + "/t.db")
	defer s.Close()
	p, _ := s.CreateProject("App", "")
	sk, _ := s.CreateSkill(p.ID, "Deploy staging", "# old")
	s.DB().Exec(`UPDATE skills SET updated_at=? WHERE id=?`, time.Now().UnixMilli(), sk.ID)
	// Pending suggestion with same title
	s.CreateSuggestion(&store.Suggestion{ProjectID: p.ID, Type: "skill.variant", Title: "deploy staging variante"})

	sess, _ := s.CreateSession(&store.Session{ProjectID: p.ID, Adapter: "claude-code", Mode: "observed"})
	s.EndSession(sess.ID)
	s.AppendTranscriptEvent(sess.ID, "user", "text", "preciso automatizar o deploy de staging com varios passos repetidos toda vez", 0, 0)
	s.AppendTranscriptEvent(sess.ID, "assistant", "text", "erro encontrado no passo de build; vamos corrigir e tentar novamente ate ficar verde", 0, 0)

	fake := &fakeAdapter{out: `[{"type":"skill.variant","title":"deploy staging variante","name":"deploy staging variante","content":"# var","evidence":"e","skill_id":"` + sk.ID + `","project_id":"` + p.ID + `"}]`}
	eng := New(s, fake, bus.New())
	res, _ := eng.Sweep(context.Background())

	if res.Duplicates != 1 {
		t.Fatalf("Duplicates=%d, want 1 (pending dup deve ser descartada)", res.Duplicates)
	}
	if res.Created != 0 {
		t.Fatalf("Created=%d, want 0", res.Created)
	}
}

func assertEvent(t *testing.T, ch <-chan bus.Event, typ string) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				t.Fatalf("canal fechado antes de receber evento %q", typ)
			}
			if ev.Type == typ {
				return
			}
		case <-deadline:
			t.Fatalf("evento %q não recebido em 2s", typ)
		}
	}
}

func TestAnalyzeBatchSuppressSkills(t *testing.T) {
	s, _ := store.Open(t.TempDir() + "/t.db")
	defer s.Close()
	p, _ := s.CreateProject("App", "")
	sess, _ := s.CreateSession(&store.Session{ProjectID: p.ID, Adapter: "claude-code", Mode: "observed"})
	s.EndSession(sess.ID)
	s.AppendTranscriptEvent(sess.ID, "user", "text", "como faço deploy?", 0, 0)
	s.AppendTranscriptEvent(sess.ID, "assistant", "text", "houve um erro no build; repetimos os passos de deploy de staging ate ficar verde, vale virar skill", 0, 0)

	fake := &fakeAdapter{out: `[{"type":"skill.learned","title":"Deploy","name":"Deploy",` +
		`"content":"# passos","evidence":"como faço deploy?","project_id":"` + p.ID + `"}]`}
	eng := New(s, fake, bus.New())

	res, err := eng.AnalyzeBatchDepth(context.Background(), p.ID, []string{sess.ID}, AnalyzeOpts{SuppressSkills: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Created != 0 {
		t.Fatalf("modo leve criou %d sugestões skill, esperado 0", res.Created)
	}
	pend, _ := s.ListSuggestions("", "pending")
	for _, x := range pend {
		if x.Type == "skill.learned" {
			t.Fatal("skill.learned não deveria existir no modo leve")
		}
	}
}

// recordingAdapter captura as HeadlessOpts recebidas para asserir override.
type recordingAdapter struct {
	out      string
	gotModel string
	called   bool
}

func (r *recordingAdapter) RunHeadless(_ context.Context, _ string, opts adapter.HeadlessOpts) (string, error) {
	r.called = true
	r.gotModel = opts.Model
	return r.out, nil
}
func (r *recordingAdapter) ReadTranscript(_ adapter.SessionRef) ([]adapter.TranscriptEvent, error) {
	return nil, nil
}

func TestAnalyzeBatchDepthUsesHeadlessAndModelOverride(t *testing.T) {
	s, _ := store.Open(t.TempDir() + "/t.db")
	defer s.Close()
	p, _ := s.CreateProject("App", "")
	sess, _ := s.CreateSession(&store.Session{ProjectID: p.ID, Adapter: "claude-code", Mode: "observed"})
	s.EndSession(sess.ID)
	s.AppendTranscriptEvent(sess.ID, "user", "text", "como faço deploy?", 0, 0)
	s.AppendTranscriptEvent(sess.ID, "assistant", "text", "houve um erro no build; repetimos os passos de deploy de staging ate ficar verde, vale virar skill", 0, 0)

	boot := &fakeAdapter{out: `[]`}
	override := &recordingAdapter{out: `[]`}
	eng := New(s, boot, bus.New())

	_, err := eng.AnalyzeBatchDepth(context.Background(), p.ID, []string{sess.ID},
		AnalyzeOpts{Headless: override, Model: "anthropic/claude-sonnet-4-6"})
	if err != nil {
		t.Fatal(err)
	}
	if !override.called {
		t.Fatal("override headless não foi usado")
	}
	if override.gotModel != "anthropic/claude-sonnet-4-6" {
		t.Fatalf("model = %q", override.gotModel)
	}
}

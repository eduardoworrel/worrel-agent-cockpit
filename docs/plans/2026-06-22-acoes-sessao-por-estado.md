# Ações de sessão por estado Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A coluna de ações da tabela de sessões da página de projeto passa a depender do estado: sessão `active` ganha "⨯ Encerrar" (com modal de confirmação) e perde "Recomeçar"; sessão `ended` mantém "Recomeçar" + "Arquivar".

**Architecture:** Mudança só de frontend em `web/src/pages/Project.tsx`. Reaproveita `killSession` (já em `api.ts`), o helper `run()` e o padrão do modal de arquivar já existentes. Adiciona estado `killTarget` + handler `handleKill` + um modal de confirmação clone do de arquivar, e condiciona os botões da coluna de ações ao `s.status`.

**Tech Stack:** React 19, TypeScript, Vite. Sem runner de teste de FE (padrão do repo) — verificação por `tsc -b`, `build` e Playwright.

## Global Constraints

- Comentários e copy em **português**; chaves i18n novas usam o 2º argumento de `t()` como default (padrão do repo).
- **Zero mudança de backend/API.** `killSession`, `postHandoff`, `archiveSession` já existem.
- Sem dependências novas.
- Estados de sessão: `active | ended | archived`; `archived` não aparece na lista (mantido).

---

### Task 1: Coluna de ações sensível ao estado + modal de Encerrar

**Files:**
- Modify: `web/src/pages/Project.tsx` (import da api ~`:28`; estado ~`:80`; handlers ~`:304`; coluna de ações `:618-644`; modais `:655`)

**Interfaces:**
- Consumes: `killSession(id: string): Promise<void>` (de `../api`); helper local `run(action)`; estado `busy`; `listSessions(id)`; `setSessions`.
- Produces: estado `killTarget: Session | null`; handler `handleKill(sessionId: string)`.

- [ ] **Step 1: Importar `killSession`**

Em `web/src/pages/Project.tsx`, no bloco de import de `../api` (que termina em `postHandoff,` na linha ~28), adicionar `killSession,`:

```tsx
  postHandoff,
  killSession,
} from '../api';
```

- [ ] **Step 2: Adicionar o estado `killTarget`**

Logo após a linha `const [archiveTarget, setArchiveTarget] = useState<Session | null>(null);` (~`:80`), adicionar:

```tsx
  const [killTarget, setKillTarget] = useState<Session | null>(null);
```

- [ ] **Step 3: Adicionar o handler `handleKill`**

Logo após a função `handleArchive` (~`:311`), adicionar:

```tsx
  // Encerra a sessão em andamento (só após confirmação): mata o processo do
  // agente; a sessão passa a 'ended' e ganha as ações de histórico na mesma linha.
  function handleKill(sessionId: string) {
    return run(async () => {
      await killSession(sessionId);
      setKillTarget(null);
      if (id) setSessions(await listSessions(id));
    });
  }
```

- [ ] **Step 4: Condicionar os botões da coluna de ações ao estado**

Substituir o bloco da coluna de ações (`web/src/pages/Project.tsx:618-644`, o `<td>` com o `<div style={{ display: 'flex', gap: '0.4rem', justifyContent: 'flex-end' }}>` contendo "Abrir terminal", "Recomeçar" e "Arquivar") por:

```tsx
                    <td>
                      <div style={{ display: 'flex', gap: '0.4rem', justifyContent: 'flex-end' }}>
                        {s.status === 'active' ? (
                          <>
                            <Link to={`/sessions/${s.id}`} className="btn btn-secondary" style={{ fontSize: '0.8rem' }}>
                              {t('sessions.openTerminal')}
                            </Link>
                            <button
                              className="btn btn-secondary"
                              style={{ fontSize: '0.8rem' }}
                              disabled={busy}
                              title={t('sessions.endHint', 'Encerra o processo do agente desta sessão') as string}
                              onClick={() => setKillTarget(s)}
                            >
                              ⨯ {t('sessions.end', 'Encerrar')}
                            </button>
                          </>
                        ) : (
                          <>
                            <button
                              className="btn btn-primary"
                              style={{ fontSize: '0.8rem' }}
                              disabled={busy}
                              title={t('sessions.resumeHint') as string}
                              onClick={() => handleResume(s.id)}
                            >
                              ↻ {t('sessions.resume')}
                            </button>
                            <button
                              className="btn btn-secondary row-archive"
                              style={{ fontSize: '0.8rem' }}
                              disabled={busy}
                              aria-label={t('sessions.archive') as string}
                              title={t('sessions.archive') as string}
                              onClick={() => setArchiveTarget(s)}
                            >
                              🗄 {t('sessions.archive')}
                            </button>
                          </>
                        )}
                      </div>
                    </td>
```

- [ ] **Step 5: Adicionar o modal de confirmação de Encerrar**

Logo após o bloco `{archiveTarget && ( ... )}` (que fecha em `:686`), adicionar o modal clone:

```tsx
      {killTarget && (
        <div className="modal-overlay" onClick={() => !busy && setKillTarget(null)}>
          <div
            className="modal"
            role="dialog"
            aria-modal="true"
            aria-labelledby="kill-confirm-title"
            onClick={(e) => e.stopPropagation()}
          >
            <h3 id="kill-confirm-title" style={{ marginTop: 0 }}>{t('sessions.endConfirmTitle', 'Encerrar sessão em andamento?')}</h3>
            <p>{t('sessions.endConfirmMsg', 'O processo do agente será finalizado. A sessão fica no histórico e pode ser recomeçada depois.')}</p>
            <div style={{ display: 'flex', gap: '1rem', marginTop: '1.5rem' }}>
              <button
                className="btn btn-secondary"
                style={{ flex: 1 }}
                disabled={busy}
                onClick={() => setKillTarget(null)}
              >
                {t('common.cancel')}
              </button>
              <button
                className="btn btn-primary"
                style={{ flex: 1 }}
                disabled={busy}
                onClick={() => handleKill(killTarget.id)}
              >
                {t('sessions.end', 'Encerrar')}
              </button>
            </div>
          </div>
        </div>
      )}
```

- [ ] **Step 6: Type-check, lint e build**

Run: `cd web && npx tsc -b && npm run build`
Expected: PASS (No errors; build conclui). Rodar também `npx eslint web/src/pages/Project.tsx` e confirmar ZERO erros novos nesse arquivo (o repo tem baseline de lint pré-existente em outros arquivos, que não conta).

- [ ] **Step 7: Commit**

```bash
git add web/src/pages/Project.tsx
git commit -m "feat(sessions): acoes por estado (Encerrar em andamento, Recomecar no historico)"
```

---

### Task 2: Verificação comportamental (Playwright)

**Files:**
- (Sem arquivos novos — verificação assistida por Playwright MCP, padrão do repo.)

**Interfaces:**
- Consumes: app rodando (`make build && ./bin/worrel -addr 127.0.0.1:7799 -no-open -data /tmp/worrel-e2e`), com um projeto e ao menos uma sessão.

> Nota: sem runner de teste de FE; a verificação é comportamental. Se não for
> possível ter uma sessão `active` real neste ambiente (precisa de um CLI de
> agente vivo), verificar ao menos o caminho `ended`/`archived` (que "Recomeçar"
> e "Arquivar" aparecem e "Encerrar" não) e registrar que o caminho `active` foi
> validado por inspeção do diff + `tsc`/`build`.

- [ ] **Step 1: Subir o app e abrir a página do projeto**

Build e run conforme acima; via Playwright `browser_navigate` para `/projects/<id>` e `browser_snapshot` da aba de sessões.

- [ ] **Step 2: Verificar ações por estado**

- Linha de sessão `active`: asserir presença de "Abrir terminal" e "⨯ Encerrar"; ausência de "↻ Recomeçar".
- Linha de sessão `ended`: asserir presença de "↻ Recomeçar" e "🗄 Arquivar"; ausência de "⨯ Encerrar".

- [ ] **Step 3: Fluxo de Encerrar (se houver sessão `active`)**

Clicar "⨯ Encerrar" → asserir que o modal de confirmação abre (título "Encerrar sessão em andamento?") → confirmar → asserir que a linha passa a `ended` e exibe "Recomeçar"/"Arquivar".

- [ ] **Step 4: Commit (registro, se houver screenshot)**

```bash
git add -A
git commit -m "test(sessions): verificacao Playwright das acoes por estado" --allow-empty
```

# Design: Ações de sessão por estado (Encerrar / Recomeçar / Arquivar)

Data: 2026-06-22
Status: Aprovado (brainstorming)

## Contexto

A tabela de sessões da página de projeto (`web/src/pages/Project.tsx`) lista as
sessões `active` e `ended` (as `archived` são omitidas por `Store.ListSessions`,
que filtra `status != 'archived'`). Hoje cada linha mostra, independentemente do
estado: "Abrir terminal" (só `active`), "↻ Recomeçar" e "🗄 Arquivar".

Faltam duas coisas do ponto de vista do usuário:
1. **Encerrar uma sessão em andamento** — não há ação para matar uma sessão `active`.
2. **Recomeçar a partir de uma sessão de histórico** — existe ("Recomeçar" =
   `postHandoff`), mas aparece em todo estado, inclusive em sessão viva, o que
   confunde o modelo "ações por estado".

## Backend já existente (zero mudança)

- `killSession(id)` → `POST /api/sessions/{id}/kill`: encerra o processo
  stream-json/PTY e faz `EndSession` (sessão vira `ended`).
- `postHandoff(id)` → `POST /api/sessions/{id}/handoff`: gera/reusa um **resumo**
  da sessão antiga, monta um primer (memória + resumo + skills) e **abre uma
  sessão nova semeada** com esse resumo, arquivando a antiga. Esta é a opção "(c)"
  acordada: nova sessão semeada com o resumo, não a conversa crua.
- `archiveSession(id)` → `POST /api/sessions/{id}/archive`: soft-hide.

Todos os três clientes já existem em `web/src/api.ts`. **Nenhuma mudança de
backend ou de API neste design.**

## Mudança proposta (apenas frontend)

Tornar a **coluna de ações** da tabela de sessões (`Project.tsx`, hoje em
`:618-644`) sensível ao `s.status`:

- **`active` (em andamento):**
  - "Abrir terminal" (mantido).
  - **"⨯ Encerrar"** — NOVO. Abre um **modal de confirmação** (mesmo padrão do
    modal de arquivar já existente em `Project.tsx:655`). Ao confirmar, chama
    `killSession(s.id)` e recarrega a lista. Sem "Recomeçar" aqui.
- **`ended` (histórico):**
  - **"↻ Recomeçar"** (mantido — `handleResume`/`postHandoff`).
  - **"🗄 Arquivar"** (mantido).
  - Sem "Encerrar" (já encerrada).
- **`archived`:** continuam omitidas da lista (sem mudança).

Fluxo resultante: encerrar uma sessão `active` → ela vira `ended` → na mesma
linha passam a aparecer "Recomeçar" e "Arquivar".

## Componentes e dados

- **Estado novo:** `killTarget: Session | null` (espelha o `archiveTarget`
  existente) para controlar o modal de confirmação de encerramento.
- **Modal de confirmação de encerrar:** clona a estrutura/classe do modal de
  arquivar (`modal-overlay`), com texto próprio ("Encerrar a sessão em
  andamento? O processo do agente será finalizado.") e botões Cancelar/Encerrar.
  Botão de confirmar desabilitado enquanto `busy`.
- **Handler:** `handleKill(id)` — `setBusy(true)`, `await killSession(id)`,
  recarrega as sessões (mesmo `reload`/`loadSessions` usado por archive/resume),
  fecha o modal, trata erro como os handlers vizinhos.
- **i18n:** novas chaves com default em português via 2º argumento de `t()`
  (`sessions.end`, `sessions.endConfirmTitle`, `sessions.endConfirmBody`,
  `sessions.endConfirm`), seguindo o padrão das chaves de arquivar.

## Tratamento de erro

- `killSession` falhando: manter o comportamento dos handlers vizinhos (capturar,
  não derrubar a UI; a lista é recarregada de qualquer forma). Encerrar uma
  sessão "órfã" (sem PTY vivo) já é tratado no backend (`EndSession` sempre roda),
  então o caminho feliz cobre o caso comum.

## Testes

- Sem runner de teste de FE no projeto (padrão do repo). Verificação por
  `npx tsc -b`, `npm run build` e checagem comportamental via Playwright:
  encerrar uma sessão `active` (com confirmação) → vira `ended` → "Recomeçar" e
  "Arquivar" aparecem na mesma linha; "Encerrar" some.

## Fora de escopo

- Qualquer mudança de backend/endpoints.
- Expor estas ações fora da tabela de sessões do projeto (card da Home, sidebar).
- "Resume" nativo do CLI (`--resume`) ou clonagem de setup — o "Recomeçar" é a
  opção (c) (nova sessão semeada com resumo), que já existe via handoff.

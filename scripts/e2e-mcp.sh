#!/usr/bin/env bash
# e2e-mcp.sh — Validação E2E do MCP server do worrel com Claude Code e OpenCode
# Uso: bash scripts/e2e-mcp.sh
# Requer: claude (2.1+), opencode (1.16+), sqlite3, curl

set -euo pipefail

ADDR="127.0.0.1:7717"
DATA_DIR="/tmp/worrel-e2e-f2"
LOG_FILE="/tmp/worrel-e2e-f2.log"
MCP_URL="http://${ADDR}/mcp"
API_URL="http://${ADDR}/api"
PASS=0
FAIL=0
SERVER_PID=""

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

pass() { echo -e "${GREEN}[PASS]${NC} $1"; PASS=$((PASS+1)); }
fail() { echo -e "${RED}[FAIL]${NC} $1"; FAIL=$((FAIL+1)); }
info() { echo -e "${YELLOW}[INFO]${NC} $1"; }

cleanup() {
  if [[ -n "$SERVER_PID" ]]; then
    info "Encerrando servidor (PID $SERVER_PID)..."
    kill "$SERVER_PID" 2>/dev/null || true
  fi
}
trap cleanup EXIT

# ---------------------------------------------------------------------------
# STEP 1: Build + servidor
# ---------------------------------------------------------------------------
info "Compilando..."
make build

info "Limpando diretório de dados..."
rm -rf "$DATA_DIR"

info "Iniciando servidor..."
./bin/worrel -addr "$ADDR" -data "$DATA_DIR" > "$LOG_FILE" 2>&1 &
SERVER_PID=$!
sleep 2

if ! curl -sf "${API_URL}/projects" > /dev/null; then
  fail "Servidor não respondeu em ${API_URL}"
  cat "$LOG_FILE"
  exit 1
fi
info "Servidor respondendo (PID $SERVER_PID)"

# ---------------------------------------------------------------------------
# STEP 2: Seed
# ---------------------------------------------------------------------------
info "Criando projeto E2E Worrel..."
PROJECT=$(curl -s -X POST "${API_URL}/projects" \
  -H 'Content-Type: application/json' \
  -d '{"name":"E2E Worrel","description":"Projeto para teste end-to-end do MCP"}')
PROJECT_ID=$(echo "$PROJECT" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
info "project_id: $PROJECT_ID"

info "Inserindo memória..."
curl -s -X PUT "${API_URL}/projects/${PROJECT_ID}/memory" \
  -H 'Content-Type: application/json' \
  -d '{"content":"# Convenções do Projeto\n\n- Convenção: commits em português\n- Usar make build antes de testar"}' > /dev/null

info "Criando skill Deploy Exemplo..."
SKILL=$(curl -s -X POST "${API_URL}/projects/${PROJECT_ID}/skills" \
  -H 'Content-Type: application/json' \
  -d '{"name":"Deploy Exemplo","content":"# Objetivo\nFazer deploy da aplicação\n\n## Passos\n1. Rodar make build\n2. Copiar binário\n3. Reiniciar serviço"}')
SKILL_ID=$(echo "$SKILL" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
info "skill_id: $SKILL_ID"

MCP_CONFIG_JSON='{"mcpServers":{"worrel":{"type":"http","url":"'"${MCP_URL}"'"}}}'

# ---------------------------------------------------------------------------
# STEP 3a: Claude Code — list_projects
# ---------------------------------------------------------------------------
info "=== Teste 3a: Claude Code list_projects ==="
OUT_3A=$(claude -p "Use a tool list_projects do servidor MCP worrel e responda só com os nomes dos projetos." \
  --mcp-config "${MCP_CONFIG_JSON}" \
  --strict-mcp-config \
  --allowedTools "mcp__worrel__list_projects" \
  --output-format text 2>&1)
echo "$OUT_3A"
if echo "$OUT_3A" | grep -q "E2E Worrel"; then
  pass "3a: list_projects contém 'E2E Worrel'"
else
  fail "3a: list_projects — 'E2E Worrel' não encontrado na saída"
fi

# ---------------------------------------------------------------------------
# STEP 3b: Claude Code — load_skill
# ---------------------------------------------------------------------------
info "=== Teste 3b: Claude Code load_skill ==="
OUT_3B=$(claude -p "Encontre a skill 'Deploy Exemplo' via MCP worrel (list_skills + load_skill) e responda apenas com o primeiro passo do roteiro." \
  --mcp-config "${MCP_CONFIG_JSON}" \
  --strict-mcp-config \
  --allowedTools "mcp__worrel__list_projects,mcp__worrel__list_skills,mcp__worrel__load_skill" \
  --output-format text 2>&1)
echo "$OUT_3B"
if echo "$OUT_3B" | grep -qi "make build"; then
  pass "3b: load_skill menciona 'make build'"
else
  fail "3b: load_skill — 'make build' não encontrado na saída"
fi

# ---------------------------------------------------------------------------
# STEP 3c: Claude Code — attribution / report_task_completed
# ---------------------------------------------------------------------------
info "=== Teste 3c: Claude Code report_task_completed (atribuição) ==="

# Inserir sessão no sqlite com token de teste
sqlite3 "${DATA_DIR}/worrel.db" "INSERT INTO sessions (id, project_id, adapter, mode, title, status, mcp_token, started_at) VALUES ('e2e-sess','${PROJECT_ID}','claude-code','wrapper','e2e','active','tok-e2e-real', CAST(strftime('%s','now') AS INTEGER)*1000);"
info "Sessão e2e-sess inserida no sqlite"

MCP_CONFIG_TOKEN='{"mcpServers":{"worrel":{"type":"http","url":"'"${MCP_URL}?s=tok-e2e-real"'"}}}'
OUT_3C=$(claude -p "Você concluiu a tarefa 'configurar CI'. Reporte via report_task_completed do worrel: resumo 'configurar CI', evidência 'pipeline verde'." \
  --mcp-config "${MCP_CONFIG_TOKEN}" \
  --strict-mcp-config \
  --allowedTools "mcp__worrel__report_task_completed" \
  --output-format text 2>&1)
echo "$OUT_3C"

SUGGESTIONS=$(curl -s "${API_URL}/suggestions?status=pending")
echo "Sugestões pendentes: $SUGGESTIONS"
if echo "$SUGGESTIONS" | grep -q '"session_id":"e2e-sess"'; then
  pass "3c: sugestão com session_id='e2e-sess' encontrada"
else
  fail "3c: sugestão com session_id='e2e-sess' não encontrada"
fi

# ---------------------------------------------------------------------------
# STEP 4: OpenCode — list_projects
# ---------------------------------------------------------------------------
info "=== Teste 4: OpenCode list_projects ==="
OPENCODE_CFG="/tmp/worrel-opencode.json"
cat > "$OPENCODE_CFG" << 'EOFJSON'
{"$schema":"https://opencode.ai/config.json","mcp":{"worrel":{"type":"remote","url":"PLACEHOLDER","enabled":true}}}
EOFJSON
# Substituir placeholder pela URL real (evitar escaping complexo)
sed -i '' "s|PLACEHOLDER|${MCP_URL}|" "$OPENCODE_CFG"

OUT_4=$(OPENCODE_CONFIG="$OPENCODE_CFG" opencode run \
  "Use the list_projects tool from the worrel MCP server and reply ONLY with the project names." 2>&1)
echo "$OUT_4"
if echo "$OUT_4" | grep -q "E2E Worrel"; then
  pass "4: OpenCode list_projects contém 'E2E Worrel'"
else
  fail "4: OpenCode list_projects — 'E2E Worrel' não encontrado (pode ser DEFERRED se requer permissão interativa)"
fi

# ---------------------------------------------------------------------------
# Sumário
# ---------------------------------------------------------------------------
echo ""
echo "======================================"
echo "  Testes: $((PASS+FAIL)) | PASS: $PASS | FAIL: $FAIL"
echo "======================================"

if [[ $FAIL -gt 0 ]]; then
  exit 1
fi

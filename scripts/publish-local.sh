#!/usr/bin/env bash
# Publica o worrel no npm a partir desta máquina (fallback quando o GitHub
# Actions está indisponível). Builda a UI, carimba a versão, cross-compila os 4
# binários de plataforma e publica os 5 pacotes (@worrel/* + worrel).
#
# Uso:   scripts/publish-local.sh <versão>      (ex.: scripts/publish-local.sh 0.2.0)
#
# 2FA: a conta npm exige segundo fator nos publishes. Sem app autenticador, gere
# um TOKEN DE AUTOMAÇÃO (npmjs.com -> Access Tokens -> Classic -> Automation, que
# ignora 2FA) e exporte antes de rodar:
#   npm config set //registry.npmjs.org/:_authToken=SEU_TOKEN
#
# Preferível: deixar o CI publicar via push de tag `v*` (.github/workflows/release.yml)
# quando o GitHub Actions estiver ativo — lá o NPM_TOKEN já dispensa OTP.
set -euo pipefail

VERSION="${1:?uso: scripts/publish-local.sh <versão>  (ex.: 0.2.0)}"
cd "$(dirname "$0")/.."

PLATFORMS=("darwin arm64 darwin-arm64" "darwin amd64 darwin-x64" "linux amd64 linux-x64" "linux arm64 linux-arm64")

echo "==> build da UI (embarcada no binário)"
make web

echo "==> carimba versão $VERSION nos package.json"
node scripts/stamp-version.mjs "$VERSION"

echo "==> cross-compila os 4 binários"
for entry in "${PLATFORMS[@]}"; do
  read -r goos goarch pkg <<<"$entry"
  GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 \
    go build -ldflags "-X main.version=$VERSION" -o "npm/platforms/$pkg/bin/worrel" ./cmd/worrel
  echo "    ok $goos/$goarch -> npm/platforms/$pkg/bin/worrel"
done

echo "==> publica os pacotes de plataforma"
for entry in "${PLATFORMS[@]}"; do
  read -r _ _ pkg <<<"$entry"
  ( cd "npm/platforms/$pkg" && npm publish --access public )
done

echo "==> publica o pacote principal"
( cd npm/worrel && npm publish --access public )

echo "==> OK: worrel@$VERSION publicado. Teste com: npx worrel@latest"
echo "==> dica: reverta os carimbos locais com 'git checkout npm/ internal/httpapi/dist/index.html'"

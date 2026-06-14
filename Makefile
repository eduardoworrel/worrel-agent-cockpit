.PHONY: build web test run publish

# publish: gera e publica a nova versão no npm num comando só.
#   make publish v=0.3.0
# (precisa de login npm ou de um token de automação configurado — ver scripts/publish-local.sh)
publish:
	@test -n "$(v)" || (echo "uso: make publish v=0.3.0"; exit 1)
	./scripts/publish-local.sh $(v)

# web: builds the React UI and copies dist to internal/httpapi/dist.
# After running `make web`, internal/httpapi/dist will have real assets.
# Only internal/httpapi/dist/index.html (placeholder) is tracked by git.
# The assets/ subdir is gitignored. To restore placeholder after a web build:
#   git checkout internal/httpapi/dist/index.html
web:
	cd web && npm run build
	rm -rf internal/httpapi/dist && cp -r web/dist internal/httpapi/dist

build: web
	go build -o bin/worrel ./cmd/worrel

test:
	go test ./...

run: build
	./bin/worrel

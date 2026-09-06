.PHONY: dev run build-web build test image deploy

# Frontend dev server on :5173. It proxies /api to the Go server from `make run`.
dev:
	cd web && npm run dev

# The Go server on its own. Reads .env when there is one, so a local run
# needs no exports.
run:
	@set -a; [ -f .env ] && . ./.env; set +a; go run ./cmd/server

# Build the frontend into web/dist, where the binary embeds it. Vite
# empties that directory, so the keep file that makes the embed pattern
# valid on a fresh clone is put back afterwards.
build-web:
	cd web && npm ci && npm run build && touch dist/.gitkeep

# One binary with the whole site inside it.
build: build-web
	go build -o bin/kindness-chain ./cmd/server

test:
	go vet ./... && go test ./...

# The same image Fly builds, for a local check before a deploy.
image:
	docker build -t kindness-chain .

deploy:
	fly deploy

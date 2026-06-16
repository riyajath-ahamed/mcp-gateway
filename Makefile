.PHONY: build test lint docker run clean generate-key version-sync release

# ─── Version ────────────────────────────────────────────────────
VERSION  := $(shell cat VERSION)
LDFLAGS  := -s -w -X github.com/configkits/mcp-gateway/internal/version.String=$(VERSION)

# ─── Go ─────────────────────────────────────────────────────────
build:
	go build -trimpath -ldflags="$(LDFLAGS)" -o bin/mcp-gateway ./cmd/gateway

test:
	go test -race -cover -count=1 ./...

test-verbose:
	go test -race -cover -count=1 -v ./...

lint:
	go vet ./...
	@which staticcheck > /dev/null 2>&1 && staticcheck ./... || echo "install staticcheck: go install honnef.co/go/tools/cmd/staticcheck@latest"

# ─── Run locally ────────────────────────────────────────────────
run: build
	./bin/mcp-gateway --config gateway.yaml

run-dev:
	go run -ldflags="$(LDFLAGS)" ./cmd/gateway --config gateway.yaml

# ─── API key generation ─────────────────────────────────────────
generate-key:
	@go run -mod=mod ./tools/genkey/main.go

# ─── TypeScript SDK ─────────────────────────────────────────────
sdk-build:
	cd pkg/typeScript-sdk && npm ci && npm run build

sdk-test:
	cd pkg/typeScript-sdk && npm run test

sdk-publish: version-sync
	cd pkg/typeScript-sdk && npm publish --access public

# ─── Version sync ──────────────────────────────────────────────
version-sync:
	@echo "Syncing VERSION $(VERSION) to pkg/typeScript-sdk/package.json"
	@cd pkg/typeScript-sdk && npm version $(VERSION) --no-git-tag-version --allow-same-version

# ─── Docker ─────────────────────────────────────────────────────
docker-build:
	docker build -t ghcr.io/configkits/mcp-gateway:latest \
		-t ghcr.io/configkits/mcp-gateway:$(VERSION) .

docker-run: docker-build
	docker run --rm \
		-p 8080:8080 \
		-v $(PWD)/gateway.yaml:/etc/mcp-gateway/gateway.yaml:ro \
		-e GATEWAY_API_KEY=$(GATEWAY_API_KEY) \
		ghcr.io/configkits/mcp-gateway:latest

docker-compose-up:
	docker compose -f deploy/docker-compose.yml up --build

# ─── Observability stack (local dev) ────────────────────────────
otel-up:
	docker compose -f deploy/docker-compose.yml --profile observability up -d jaeger
	@echo "Jaeger UI: http://localhost:16686"

# ─── Release ────────────────────────────────────────────────────
release: test version-sync
	@echo "Releasing v$(VERSION)..."
	@git diff-index --quiet HEAD -- || (echo "ERROR: uncommitted changes" && exit 1)
	git tag -a "v$(VERSION)" -m "Release v$(VERSION)"
	git push origin "v$(VERSION)"
	@echo "Tag v$(VERSION) pushed. GitHub Actions will handle the rest."

# ─── Cleanup ────────────────────────────────────────────────────
clean:
	rm -rf bin/ pkg/typeScript-sdk/dist pkg/typeScript-sdk/node_modules

# ─── Deploy ─────────────────────────────────────────────────────
deploy-fly:
	fly deploy --config deploy/fly.toml

deploy-railway:
	railway up

.PHONY: web-deps web build test test-linux

web-deps:
	cd web && npm ci

web: web-deps
	cd web && npm run build
	rm -rf internal/web/dist
	mkdir -p internal/web/dist
	cp -R web/build/. internal/web/dist/
	touch internal/web/dist/.gitkeep

build: web
	CGO_ENABLED=0 go build -ldflags "-X main.version=$$(git describe --tags --always --dirty 2>/dev/null || echo dev)" -o bin/hublot ./cmd/hublot

test: web-deps
	scripts/test-release-lib.sh
	go vet ./cmd/... ./internal/...
	go test -race ./cmd/... ./internal/...
	cd web && npm run check && npm run lint && npx vitest run

test-linux:
	scripts/test-linux.sh -race -tags integration ./cmd/... ./internal/...

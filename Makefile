.PHONY: web build test

web:
	cd web && npm ci && npm run build
	rm -rf internal/web/dist
	mkdir -p internal/web/dist
	cp -R web/build/. internal/web/dist/
	touch internal/web/dist/.gitkeep

build: web
	CGO_ENABLED=0 go build -ldflags "-X main.version=$$(git describe --tags --always --dirty)" -o bin/hublot ./cmd/hublot

test:
	go vet ./...
	go test -race ./...
	cd web && npm run check && npm run lint && npx vitest run

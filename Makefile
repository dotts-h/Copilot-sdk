BINARY := bin/my-orchestra
DESKTOP_BINARY := bin/my-orchestra-desktop
PKG := ./...
VERSION ?= dev
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: all build run desktop run-desktop test bench cover lint fmt vet fuzz tidy clean e2e e2e-install codemap

all: lint test build

build:
	@mkdir -p bin
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/my-orchestra

run: build
	$(BINARY)

# Desktop shell (Wails v3). Requires CGO + the OS webview toolchain: on Linux
# install libgtk-3-dev + libwebkit2gtk-4.1-dev; macOS/Windows ship the webview.
# Isolated behind the `desktop` build tag so the pure-Go targets never need CGO.
desktop:
	@mkdir -p bin
	CGO_ENABLED=1 go build -tags desktop -ldflags "$(LDFLAGS)" -o $(DESKTOP_BINARY) ./cmd/my-orchestra-desktop

run-desktop: desktop
	$(DESKTOP_BINARY) -demo

test:
	go test $(PKG) -race -count=1 -timeout 180s -cover

bench:
	go test ./internal/web -run x -bench . -benchmem

# Browser suite (e2e · api · a11y · ux · perf). Builds the binary, launches the
# demo server, and drives Chromium via Playwright. See e2e/README.md.
e2e-install:
	cd e2e && npm ci && npx playwright install --with-deps chromium

e2e: build
	cd e2e && npx playwright test

cover:
	go test $(PKG) -count=1 -coverprofile=coverage.txt -covermode=atomic
	go tool cover -func=coverage.txt | tail -1
	@echo "open coverage.html for the HTML report"
	go tool cover -html=coverage.txt -o coverage.html

fuzz:
	go test ./internal/telemetry -run x -fuzz FuzzPriceNeverNegativeOrNaN -fuzztime 20s

codemap:
	bash scripts/codemap.sh

fmt:
	gofmt -w ./cmd ./internal

vet:
	go vet $(PKG)

lint: vet
	@unformatted=$$(gofmt -l ./cmd ./internal); \
	if [ -n "$$unformatted" ]; then echo "gofmt needed:"; echo "$$unformatted"; exit 1; fi
	@command -v golangci-lint >/dev/null 2>&1 && golangci-lint run --timeout=5m || echo "golangci-lint not installed; skipping"

tidy:
	go mod tidy

clean:
	rm -rf bin dist coverage.txt coverage.html

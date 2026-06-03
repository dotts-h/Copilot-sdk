BINARY := bin/my-orchestra
PKG := ./...
VERSION ?= dev
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: all build run test bench cover lint fmt vet fuzz tidy clean e2e e2e-install

all: lint test build

build:
	@mkdir -p bin
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/my-orchestra

run: build
	$(BINARY)

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

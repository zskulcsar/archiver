.PHONY: \
	help \
	fmt \
	fmt-check \
	deps-verify \
	deps-vuln \
	deps-outdated \
	vet \
	lint \
	test-unit \
	test-race \
	docs-check \
	verify \
	build \
	build-cross \
	clean

VERSION ?= dev
REVISION ?=
BINARY := bin/archiver

help: ## List available make targets with descriptions.
	@printf "Available targets:\n"
	@grep -hE '^[a-zA-Z_-]+:.*## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*## "} {printf "  %-16s %s\n", $$1, $$2}'

fmt: ## Format Go source files.
	go fmt ./...

fmt-check: ## Check Go source formatting.
	@test -z "$$(gofmt -l $$(git ls-files '*.go'))"

deps-verify: ## Verify module integrity and that go.mod and go.sum are tidy.
	go mod verify
	go mod tidy -diff

deps-vuln: ## Scan reachable code for known vulnerabilities with govulncheck.
	govulncheck ./...

deps-outdated: ## Report available module updates.
	go list -m -u all

vet: ## Run Go static analysis.
	go vet ./...

lint: ## Run golangci-lint.
	golangci-lint run ./...

test-unit: ## Run unit tests.
	go test ./...

test-race: ## Run unit tests with the race detector.
	go test -race ./...

docs-check: ## Check Markdown files for trailing whitespace.
	@git ls-files --cached --others --exclude-standard '*.md' | awk '$$0 == "README.md" || $$0 ~ "^docs/"' | xargs -r awk '/[[:blank:]]$$/ { printf "%s:%d: trailing whitespace\n", FILENAME, FNR; failed=1 } END { exit failed }'

verify: fmt-check deps-verify vet lint test-unit docs-check ## Run the required local checks.

build: ## Build the archiver CLI.
	go build -trimpath -ldflags "-X main.version=$(VERSION) -X main.revision=$(REVISION)" -o $(BINARY) ./cmd/archiver

build-cross: ## Cross-compile the portable CLI for Linux, Windows, and macOS.
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-X main.version=$(VERSION) -X main.revision=$(REVISION)" -o bin/archiver-linux-amd64 ./cmd/archiver
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-X main.version=$(VERSION) -X main.revision=$(REVISION)" -o bin/archiver-windows-amd64.exe ./cmd/archiver
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags "-X main.version=$(VERSION) -X main.revision=$(REVISION)" -o bin/archiver-darwin-arm64 ./cmd/archiver

clean: ## Remove generated build and test artifacts.
	rm -rf bin coverage.out

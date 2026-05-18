# paths
makefile := $(realpath $(lastword $(MAKEFILE_LIST)))
cmd_dir  := ./cmd/freeplay
dist_dir := ./dist

# EmulatorJS vendored asset release (fetched at build time, not committed)
# Upstream tag and asset filename differ (tag has a leading `v`, asset
# does not), so they're separate variables rather than one composed from
# the other.
EMULATORJS_TAG      := v4.3.0-pre
EMULATORJS_ASSET    := 4.3.0-pre.7z
EMULATORJS_URL      := https://github.com/EmulatorJS/EmulatorJS/releases/download/$(EMULATORJS_TAG)/$(EMULATORJS_ASSET)
EMULATORJS_SHA256   := 0949d75fa5cff05c47e0431443dad6b65e2ebc5f1517cbb09f3d671236d3effd
EMULATORJS_SENTINEL := emulatorjs/data/version.json

# executables
GO   := go
FMT  := gofumpt
LINT := golangci-lint
MKDIR := mkdir -p

# build flags
VERSION     := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
BUILD_FLAGS := -ldflags="-s -w -X github.com/chrisallenlane/freeplay.Version=$(VERSION)" -mod vendor -trimpath

## build: build the freeplay binary
.PHONY: build
build: $(EMULATORJS_SENTINEL) | $(dist_dir)
	$(GO) build $(BUILD_FLAGS) -o $(dist_dir)/freeplay $(cmd_dir)

## build-debug: build with pprof exposed on 127.0.0.1:6060
.PHONY: build-debug
build-debug: $(EMULATORJS_SENTINEL) | $(dist_dir)
	$(GO) build -tags debug -ldflags="-X github.com/chrisallenlane/freeplay.Version=$(VERSION)-debug" -mod vendor -o $(dist_dir)/freeplay-debug $(cmd_dir)

## run: build and run with test data
.PHONY: run
run: build
	$(dist_dir)/freeplay -data ./testdata

# ./dist
$(dist_dir):
	$(MKDIR) $(dist_dir)

## fmt: format source files
.PHONY: fmt
fmt:
	$(FMT) -w .
	npx --yes @biomejs/biome check --write frontend/*.js

## lint: lint source files
.PHONY: lint
lint: $(EMULATORJS_SENTINEL)
	$(LINT) run ./...
	npx --yes @biomejs/biome check frontend/*.js
	npx --yes html-validate frontend/*.html

## vet: vet go source files
.PHONY: vet
vet: $(EMULATORJS_SENTINEL)
	$(GO) vet ./...

## test: run unit tests
.PHONY: test
test: build
	$(GO) test ./...
	node --test frontend/*_test.js

## coverage: generate a test coverage report
.PHONY: coverage
coverage: $(EMULATORJS_SENTINEL) .tmp
	$(GO) test ./... -coverprofile=.tmp/coverage.out && \
	$(GO) tool cover -html=.tmp/coverage.out -o .tmp/coverage.html && \
	echo "Coverage report generated: .tmp/coverage.html" && \
	(sensible-browser .tmp/coverage.html 2>/dev/null || \
	 xdg-open .tmp/coverage.html 2>/dev/null || \
	 open .tmp/coverage.html 2>/dev/null || \
	 echo "Please open .tmp/coverage.html in your browser")

## coverage-text: show test coverage by function in terminal
.PHONY: coverage-text
coverage-text: $(EMULATORJS_SENTINEL) .tmp
	$(GO) test ./... -coverprofile=.tmp/coverage.out && \
	$(GO) tool cover -func=.tmp/coverage.out | sort -k3 -n

## integration: run integration tests (boots the real server)
.PHONY: integration
integration: $(EMULATORJS_SENTINEL)
	$(GO) test -tags=integration -count=1 ./internal/integration/...

## fuzz: run quick fuzz tests (15s each)
.PHONY: fuzz
fuzz: $(EMULATORJS_SENTINEL)
	@./test/fuzz.sh 15s

## fuzz-long: run extended fuzz tests (10m each)
.PHONY: fuzz-long
fuzz-long: $(EMULATORJS_SENTINEL)
	@./test/fuzz.sh 10m

## bench: run Go benchmarks (count=5 for benchstat-friendly output)
.PHONY: bench
bench: $(EMULATORJS_SENTINEL)
	$(GO) test -run=^$$ -bench=. -benchmem -count=5 ./...

## a11y: run accessibility audit against live server
.PHONY: a11y
a11y: build
	@./test/a11y.sh $(dist_dir)/freeplay

## check: format, lint, vet, and run unit tests
.PHONY: check
check: fmt lint vet test

## audit: scan dependencies (govulncheck) and source (gosec) for security issues
.PHONY: audit
audit: $(EMULATORJS_SENTINEL)
	govulncheck ./...
	gosec -quiet ./...

## clean: remove compiled binary and temporary files
.PHONY: clean
clean:
	rm -f $(dist_dir)/*
	rm -rf .tmp

## vendor: download, tidy, and verify dependencies
.PHONY: vendor
vendor:
	$(GO) mod vendor && $(GO) mod tidy && $(GO) mod verify

## vendor-update: update vendored dependencies
.PHONY: vendor-update
vendor-update:
	$(GO) get -t -u ./... && $(GO) mod vendor && $(GO) mod tidy && $(GO) mod verify

## setup: install dev dependencies (gofumpt, golangci-lint, govulncheck, gosec)
.PHONY: setup
setup:
	$(GO) install mvdan.cc/gofumpt@v0.9.2
	$(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.11.3
	$(GO) install golang.org/x/vuln/cmd/govulncheck@latest
	$(GO) install github.com/securego/gosec/v2/cmd/gosec@latest

## docker: build docker image
.PHONY: docker
docker: $(EMULATORJS_SENTINEL)
	docker build --build-arg VERSION=$(VERSION) -t freeplay .

## fetch-emulatorjs: download and verify pinned EmulatorJS assets
.PHONY: fetch-emulatorjs
fetch-emulatorjs: $(EMULATORJS_SENTINEL)

$(EMULATORJS_SENTINEL): | .tmp
	@echo "Fetching EmulatorJS assets $(EMULATORJS_TAG)..."
	@curl -fsSL -o .tmp/$(EMULATORJS_ASSET) $(EMULATORJS_URL)
	@echo "$(EMULATORJS_SHA256)  .tmp/$(EMULATORJS_ASSET)" | sha256sum -c -
	@rm -rf emulatorjs
	@# `-xr!*Zone.Identifier` skips NTFS alternate-data-stream marker
	@# files that ship in the upstream archive (the release is packaged
	@# on Windows). Go's //go:embed rejects them as invalid filenames.
	@7z x -y -o./emulatorjs '-xr!*Zone.Identifier' .tmp/$(EMULATORJS_ASSET) data > /dev/null
	@rm -f .tmp/$(EMULATORJS_ASSET)
	@echo "EmulatorJS assets ready at ./emulatorjs/"

# .tmp
.tmp:
	mkdir -p .tmp

## help: display this help text
.PHONY: help
help:
	@cat $(makefile) | \
	sort             | \
	grep "^##"       | \
	sed 's/## //g'   | \
	column -t -s ':'

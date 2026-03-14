# paths
makefile := $(realpath $(lastword $(MAKEFILE_LIST)))

# executables
GO   := go
FMT  := gofumpt
LINT := revive

# build flags
BUILD_FLAGS := -ldflags="-s -w" -trimpath

## build: build the freeplay binary
.PHONY: build
build: emulatorjs
	$(GO) build $(BUILD_FLAGS) -o freeplay .

## run: build and run with test data
.PHONY: run
run: build
	./freeplay -data ./testdata

## fmt: format go source files
.PHONY: fmt
fmt:
	$(FMT) -w .

## lint: lint go source files
.PHONY: lint
lint:
	$(LINT) ./...

## vet: vet go source files
.PHONY: vet
vet:
	$(GO) vet ./...

## test: run unit tests
.PHONY: test
test:
	$(GO) test ./...

## coverage: generate a test coverage report
.PHONY: coverage
coverage: .tmp
	$(GO) test ./... -coverprofile=.tmp/coverage.out && \
	$(GO) tool cover -html=.tmp/coverage.out -o .tmp/coverage.html && \
	echo "Coverage report generated: .tmp/coverage.html" && \
	(sensible-browser .tmp/coverage.html 2>/dev/null || \
	 xdg-open .tmp/coverage.html 2>/dev/null || \
	 open .tmp/coverage.html 2>/dev/null || \
	 echo "Please open .tmp/coverage.html in your browser")

## coverage-text: show test coverage by function in terminal
.PHONY: coverage-text
coverage-text: .tmp
	$(GO) test ./... -coverprofile=.tmp/coverage.out && \
	$(GO) tool cover -func=.tmp/coverage.out | sort -k3 -n

## check: format, lint, vet, and run unit tests
.PHONY: check
check: fmt lint vet test

## clean: remove compiled binary and temporary files
.PHONY: clean
clean:
	rm -f freeplay
	rm -rf .tmp

## setup: install dev dependencies (gofumpt, revive)
.PHONY: setup
setup:
	$(GO) install mvdan.cc/gofumpt@latest
	$(GO) install github.com/mgechev/revive@v1.9.0

## docker: build docker image
.PHONY: docker
docker:
	docker build -t freeplay .

# Download EmulatorJS for local dev
emulatorjs:
	@if [ ! -f emulatorjs/data/loader.js ]; then ./scripts/download-emulatorjs.sh; fi

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

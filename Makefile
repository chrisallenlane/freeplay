# paths
makefile := $(realpath $(lastword $(MAKEFILE_LIST)))
cmd_dir  := ./cmd/freeplay
dist_dir := ./dist

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
build: | $(dist_dir)
	$(GO) build $(BUILD_FLAGS) -o $(dist_dir)/freeplay $(cmd_dir)

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
lint:
	$(LINT) run ./...
	npx --yes @biomejs/biome check frontend/*.js
	npx --yes html-validate frontend/*.html

## vet: vet go source files
.PHONY: vet
vet:
	$(GO) vet ./...

## test: run unit tests
.PHONY: test
test:
	$(GO) test ./...
	node --test frontend/utils_test.js

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

## setup: install dev dependencies (gofumpt, golangci-lint)
.PHONY: setup
setup:
	$(GO) install mvdan.cc/gofumpt@latest
	$(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest

## docker: build docker image
.PHONY: docker
docker:
	docker build --build-arg VERSION=$(VERSION) -t freeplay .

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

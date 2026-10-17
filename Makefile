GO ?= go

GOEXE := $(shell $(GO) env GOEXE)
BINARY := bin/gbay$(GOEXE)
VERSION_PKG := github.com/JoaoVictorVM/gopherbay/internal/version

VERSION ?= $(shell git describe --tags --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE ?= $(shell date -u +%Y-%m-%d)
LDFLAGS := -s -w -X $(VERSION_PKG).Version=$(VERSION) -X $(VERSION_PKG).Commit=$(COMMIT) -X $(VERSION_PKG).Date=$(DATE)

FUZZTIME ?= 30s

GOLANGCI_LINT_VERSION ?= v2.14.0
GOVULNCHECK_VERSION ?= v1.9.0

# go env prints backslashes on Windows, which Git Bash would treat as escapes.
TOOLS_DIR := $(subst \,/,$(shell $(GO) env GOBIN))
ifeq ($(TOOLS_DIR),)
TOOLS_DIR := $(subst \,/,$(firstword $(subst ;, ,$(shell $(GO) env GOPATH))))/bin
endif
GOLANGCI_LINT := $(TOOLS_DIR)/golangci-lint$(GOEXE)
GOVULNCHECK := $(TOOLS_DIR)/govulncheck$(GOEXE)

# The race detector needs cgo, so it is only enabled when a C compiler exists.
# Disable it with: make test RACE=
RACE ?= $(if $(shell command -v "$$($(GO) env CC)" 2>/dev/null),-race,)
TEST_CGO := $(if $(RACE),1,0)

.DEFAULT_GOAL := build
.PHONY: build run test cover fuzz lint vuln fmt tools clean

build:
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/gbay

run: build
	./$(BINARY) $(ARGS)

test:
	CGO_ENABLED=$(TEST_CGO) $(GO) test $(RACE) -shuffle=on ./...

cover:
	CGO_ENABLED=$(TEST_CGO) $(GO) test $(RACE) -shuffle=on -coverprofile=coverage.out -covermode=atomic ./...
	$(GO) tool cover -func=coverage.out | tail -n 1
	$(GO) tool cover -html=coverage.out -o coverage.html

fuzz:
	$(GO) test -run='^$$' -fuzz='^FuzzParse$$' -fuzztime=$(FUZZTIME) ./internal/config

lint:
	@test -x "$(GOLANGCI_LINT)" || { echo "golangci-lint não encontrado em $(TOOLS_DIR): rode make tools primeiro" >&2; exit 1; }
	"$(GOLANGCI_LINT)" run

vuln:
	@test -x "$(GOVULNCHECK)" || { echo "govulncheck não encontrado em $(TOOLS_DIR): rode make tools primeiro" >&2; exit 1; }
	"$(GOVULNCHECK)" ./...

fmt:
	@test -x "$(GOLANGCI_LINT)" || { echo "golangci-lint não encontrado em $(TOOLS_DIR): rode make tools primeiro" >&2; exit 1; }
	"$(GOLANGCI_LINT)" fmt

# go install picks the toolchain from each tool's own go.mod, and golangci-lint
# refuses to lint a module newer than the Go that built it, so both tools are
# built with the toolchain this module requires.
tools:
	GOTOOLCHAIN=$(shell $(GO) env GOVERSION) $(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	GOTOOLCHAIN=$(shell $(GO) env GOVERSION) $(GO) install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)

clean:
	rm -rf bin dist coverage.out coverage.html

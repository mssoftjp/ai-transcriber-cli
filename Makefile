.PHONY: build install fmt test cover lint deps ci hooks release-archive package verify

GO ?= go
PREFIX ?= $(HOME)/.local
BINDIR ?= $(PREFIX)/bin
BIN ?= $(CURDIR)/transcriber
DIST_DIR ?= $(CURDIR)/dist
VERSION ?=
COMMIT ?=
DATE ?=
GOOS ?=
GOARCH ?=

build:
	GO="$(GO)" GOOS="$(GOOS)" GOARCH="$(GOARCH)" VERSION="$(VERSION)" COMMIT="$(COMMIT)" DATE="$(DATE)" \
		./scripts/build.sh --output "$(BIN)"

install:
	GO="$(GO)" GOOS="$(GOOS)" GOARCH="$(GOARCH)" VERSION="$(VERSION)" COMMIT="$(COMMIT)" DATE="$(DATE)" \
		./scripts/install.sh --bindir "$(BINDIR)"

fmt:
	$(GO) fmt ./...

test:
	$(GO) test ./... -count=1 -timeout=240s

cover:
	$(GO) test -p 1 ./... -coverprofile=coverage.out -count=1 -timeout=240s

lint:
	@command -v golangci-lint >/dev/null 2>&1 || (echo "golangci-lint is required for 'make lint'" >&2; exit 1)
	golangci-lint run --timeout=5m ./...

deps:
	$(GO) mod verify
	@cp go.mod go.mod.bak && cp go.sum go.sum.bak
	@$(GO) mod tidy
	@if ! diff -q go.mod go.mod.bak >/dev/null 2>&1 || ! diff -q go.sum go.sum.bak >/dev/null 2>&1; then \
		mv go.mod.bak go.mod; mv go.sum.bak go.sum; \
		echo "go.mod/go.sum would change after 'go mod tidy' — run it and commit"; \
		exit 1; \
	fi
	@rm -f go.mod.bak go.sum.bak

ci: lint test deps

verify: fmt test

hooks:
	git config core.hooksPath .githooks

release-archive:
	@[ -n "$(VERSION)" ] || (echo "VERSION is required, e.g. make release-archive VERSION=v0.2.0 GOOS=darwin GOARCH=arm64" >&2; exit 1)
	@[ -n "$(GOOS)" ] || (echo "GOOS is required" >&2; exit 1)
	@[ -n "$(GOARCH)" ] || (echo "GOARCH is required" >&2; exit 1)
	mkdir -p "$(DIST_DIR)"
	GO="$(GO)" GOOS="$(GOOS)" GOARCH="$(GOARCH)" VERSION="$(VERSION)" COMMIT="$(COMMIT)" DATE="$(DATE)" \
		./scripts/package.sh --version "$(VERSION)" --output-dir "$(DIST_DIR)"

package:
	@[ -n "$(VERSION)" ] || (echo "VERSION is required, e.g. make package VERSION=v0.2.0" >&2; exit 1)
	mkdir -p "$(DIST_DIR)"
	GO="$(GO)" GOOS="$(GOOS)" GOARCH="$(GOARCH)" VERSION="$(VERSION)" COMMIT="$(COMMIT)" DATE="$(DATE)" \
		./scripts/package.sh --version "$(VERSION)" --output-dir "$(DIST_DIR)"

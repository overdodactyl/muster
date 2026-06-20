APP_NAME = muster
BIN_DIR  = bin
PREFIX  ?= $(HOME)/.local
GO      ?= go

# /tmp on the cluster can be noexec, which breaks `go test`. Use a repo-local tmpdir.
export GOTMPDIR := $(CURDIR)/.gotmp

VERSION = $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS = -X main.version=$(VERSION)

.PHONY: all build clean rebuild tidy vendor test install fmt vet

all: build

build: tidy
	mkdir -p $(BIN_DIR)
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(APP_NAME) .

tidy:
	$(GO) mod tidy

vendor:
	$(GO) mod vendor

test:
	mkdir -p $(GOTMPDIR)
	$(GO) test ./...

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

clean:
	rm -rf $(BIN_DIR)

rebuild: clean build

install: build
	install -m 0755 $(BIN_DIR)/$(APP_NAME) $(PREFIX)/bin/$(APP_NAME)
	@echo "Installed to $(PREFIX)/bin/$(APP_NAME)"

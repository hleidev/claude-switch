# claude-switch — build & install
#
# Local install (no Homebrew needed):
#   make install      build, install to ~/.local/bin, wire up shell integration
#   make uninstall    remove shell integration + config prompt, then the binary
#
# Override the install prefix like any autotools-style project:
#   make install PREFIX=/usr/local

BINARY  := cs
PREFIX  ?= $(HOME)/.local
BINDIR  := $(PREFIX)/bin
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: all build install uninstall test fmt vet clean

all: build

build:
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) .

install: build
	@mkdir -p "$(BINDIR)"
	install -m 0755 bin/$(BINARY) "$(BINDIR)/$(BINARY)"
	@echo "✓ installed $(BINDIR)/$(BINARY) ($(VERSION))"
	@case ":$$PATH:" in *":$(BINDIR):"*) ;; *) echo "⚠ $(BINDIR) is not on your PATH — add it." ;; esac
	@"$(BINDIR)/$(BINARY)" setup || true

uninstall:
	-@"$(BINDIR)/$(BINARY)" uninstall || true
	rm -f "$(BINDIR)/$(BINARY)"
	@echo "✓ removed $(BINDIR)/$(BINARY)"

test:
	go test ./...

fmt:
	gofmt -w .

vet:
	go vet ./...

clean:
	rm -rf bin

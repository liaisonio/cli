BINARY  := liaison
MODULE  := github.com/liaisonio/cli
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X $(MODULE)/internal/cli.Version=$(VERSION) -s -w

# Platforms we publish prebuilt binaries for. Format: GOOS/GOARCH.
PLATFORMS := \
	darwin/amd64 \
	darwin/arm64 \
	linux/amd64 \
	linux/arm64 \
	windows/amd64

DIST := dist

.PHONY: build install test clean tidy fmt vet release release-clean checksums

build:
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/liaison

install:
	go install -ldflags "$(LDFLAGS)" ./cmd/liaison

test:
	go test ./...

tidy:
	go mod tidy

fmt:
	gofmt -w .

vet:
	go vet ./...

clean:
	rm -rf bin/

# release builds one stripped binary per target platform under dist/
# Naming: dist/liaison-<version>-<os>-<arch>[.exe]
# This is what the install.sh script and npm wrapper download.
release: release-clean
	@mkdir -p $(DIST)
	@for platform in $(PLATFORMS); do \
		os=$${platform%/*}; arch=$${platform#*/}; \
		ext=""; if [ "$$os" = "windows" ]; then ext=".exe"; fi; \
		out="$(DIST)/$(BINARY)-$(VERSION)-$$os-$$arch$$ext"; \
		echo "→ $$out"; \
		GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 \
			go build -trimpath -ldflags "$(LDFLAGS)" -o $$out ./cmd/liaison || exit 1; \
	done
	@$(MAKE) checksums

checksums:
	@cd $(DIST) && shasum -a 256 $(BINARY)-* > SHA256SUMS && cat SHA256SUMS

release-clean:
	rm -rf $(DIST)

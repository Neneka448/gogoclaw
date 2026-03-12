VERSION   := $(shell git describe --tags --always --dirty)
BUILD_TIME := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
GIT_COMMIT := $(shell git rev-parse --short HEAD)
UNAME_S := $(shell uname -s)
UNAME_M := $(shell uname -m)
SQLITE_VEC_VERSION ?= v0.1.6
WORKSPACE ?= $(HOME)/.gogoclaw/workspace
SQLITE_VEC_PREFIX ?= $(WORKSPACE)/sqlite-vec

ifeq ($(UNAME_S),Darwin)
SQLITE_VEC_OS := macos
else ifeq ($(UNAME_S),Linux)
SQLITE_VEC_OS := linux
else
$(error unsupported OS $(UNAME_S) for sqlite-vec automation)
endif

ifeq ($(UNAME_M),arm64)
SQLITE_VEC_ARCH := aarch64
else ifeq ($(UNAME_M),aarch64)
SQLITE_VEC_ARCH := aarch64
else ifeq ($(UNAME_M),x86_64)
SQLITE_VEC_ARCH := x86_64
else
$(error unsupported architecture $(UNAME_M) for sqlite-vec automation)
endif

SQLITE_VEC_TARGET := $(SQLITE_VEC_OS)-$(SQLITE_VEC_ARCH)
SQLITE_VEC_ARCHIVE := sqlite-vec-$(patsubst v%,%,$(SQLITE_VEC_VERSION))-loadable-$(SQLITE_VEC_TARGET).tar.gz
SQLITE_VEC_DOWNLOAD_URL := https://github.com/asg017/sqlite-vec/releases/download/$(SQLITE_VEC_VERSION)/$(SQLITE_VEC_ARCHIVE)

ifeq ($(SQLITE_VEC_TARGET),macos-aarch64)
SQLITE_VEC_SHA256 := 142e195b654092632fecfadbad2825f3140026257a70842778637597f6b8c827
else ifeq ($(SQLITE_VEC_TARGET),macos-x86_64)
SQLITE_VEC_SHA256 := 35d014e5f7bcac52645a97f1f1ca34fdb51dcd61d81ac6e6ba1c712393fbf8fd
else ifeq ($(SQLITE_VEC_TARGET),linux-x86_64)
SQLITE_VEC_SHA256 := 438e0df29f3f8db3525b3aa0dcc0a199869c0bcec9d7abc5b51850469caf867f
else ifeq ($(SQLITE_VEC_TARGET),linux-aarch64)
SQLITE_VEC_SHA256 := d6e4ba12c5c0186eaab42fb4449b311008d86ffd943e6377d7d88018cffab3aa
else
$(error unsupported sqlite-vec target $(SQLITE_VEC_TARGET))
endif

ifeq ($(UNAME_S),Darwin)
SQLITE_VEC_SHA256_CMD := shasum -a 256
else
SQLITE_VEC_SHA256_CMD := sha256sum
endif

LDFLAGS := -X github.com/Neneka448/gogoclaw/internal/version.Version=$(VERSION) \
           -X github.com/Neneka448/gogoclaw/internal/version.BuildTime=$(BUILD_TIME) \
           -X github.com/Neneka448/gogoclaw/internal/version.GitCommit=$(GIT_COMMIT)

.PHONY: build test build-lite test-lite sqlite-vec-install sqlite-vec-clean

build: build-lite

test: test-lite

build-lite:
	go build -ldflags "$(LDFLAGS)" -o gogoclaw .

test-lite:
	go test ./...

sqlite-vec-install:
	@mkdir -p "$(SQLITE_VEC_PREFIX)"
	@tmpfile="$(SQLITE_VEC_PREFIX)/$(SQLITE_VEC_ARCHIVE)"; \
	command curl -fL --progress-bar "$(SQLITE_VEC_DOWNLOAD_URL)" -o "$$tmpfile"; \
	printf '%s  %s\n' "$(SQLITE_VEC_SHA256)" "$$tmpfile" | $(SQLITE_VEC_SHA256_CMD) -c -; \
	tar -xzf "$$tmpfile" -C "$(SQLITE_VEC_PREFIX)"; \
	rm -f "$$tmpfile"

sqlite-vec-clean:
	rm -rf "$(SQLITE_VEC_PREFIX)"
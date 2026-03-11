VERSION   := $(shell git describe --tags --always --dirty)
BUILD_TIME := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
GIT_COMMIT := $(shell git rev-parse --short HEAD)

LDFLAGS := -X github.com/Neneka448/gogoclaw/internal/version.Version=$(VERSION) \
           -X github.com/Neneka448/gogoclaw/internal/version.BuildTime=$(BUILD_TIME) \
           -X github.com/Neneka448/gogoclaw/internal/version.GitCommit=$(GIT_COMMIT)

build:
    go build -ldflags "$(LDFLAGS)" -o gogoclaw .
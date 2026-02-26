APP_NAME := orbit
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE    := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS := -X github.com/humanetools/orbit/internal/version.Version=$(VERSION) \
           -X github.com/humanetools/orbit/internal/version.GitCommit=$(COMMIT) \
           -X github.com/humanetools/orbit/internal/version.BuildDate=$(DATE)

.PHONY: build run test clean

build:
	go build -ldflags "$(LDFLAGS)" -o $(APP_NAME) .

run: build
	./$(APP_NAME)

test:
	go test ./... -count=1

clean:
	rm -f $(APP_NAME)

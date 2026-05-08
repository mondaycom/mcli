VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: build test lint vet fmt fmt-check tidy

build:
	go build -ldflags "-X github.com/mondaycom/mcli/internal/cli.version=$(VERSION)" -o bin/mcli ./cmd/mcli

test:
	go test -race ./...

lint:
	golangci-lint run

vet:
	go vet ./...

fmt:
	gofmt -w .

fmt-check:
	test -z "$$(gofmt -l .)"

tidy:
	go mod tidy

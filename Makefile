.PHONY: all build test lint run clean

all: lint test build

build:
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/gateway ./cmd/gateway

test:
	go test -v -race ./...

lint:
	go vet ./...

run:
	go run ./cmd/gateway

clean:
	rm -rf bin/

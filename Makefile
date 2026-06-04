.PHONY: build test lint clean

build:
	go build -ldflags "-X main.version=$(shell git describe --tags --always --dirty 2>/dev/null || echo dev)" -o bin/thenn ./cmd/thenn

test:
	go test ./...

lint:
	which golangci-lint 2>/dev/null || curl -sSfL https://golangci-lint.run/install.sh | sh -s -- -b $(go env GOPATH)/bin
	golangci-lint run ./...

clean:
	rm -rf bin/

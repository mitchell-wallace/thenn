version := shell("git describe --tags --always --dirty 2>/dev/null || echo dev")
gopath := shell("go env GOPATH")

default: build

# Build the thenn binary
build:
	go build -ldflags "-X main.version={{version}}" -o bin/thenn ./cmd/thenn

# Run all tests
test:
	go test ./...

# Run the linter
lint:
	which golangci-lint 2>/dev/null || curl -sSfL https://golangci-lint.run/install.sh | sh -s -- -b {{gopath}}/bin
	{{gopath}}/bin/golangci-lint run ./...

# Clean build artifacts
clean:
	rm -rf bin/

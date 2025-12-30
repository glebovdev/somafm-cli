BINARY_NAME=somafm
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null | sed 's/^v//' || echo "dev")
LDFLAGS=-ldflags "-X github.com/glebovdev/somafm-cli/internal/config.AppVersion=$(VERSION)"

build:
	go build $(LDFLAGS) -o $(BINARY_NAME) cmd/somafm/main.go

# Linux cross-compilation requires CGO for ALSA audio. Use GitHub Actions for Linux builds.
build-all:
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(BINARY_NAME)-darwin-amd64 cmd/somafm/main.go
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(BINARY_NAME)-darwin-arm64 cmd/somafm/main.go
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(BINARY_NAME)-windows-amd64.exe cmd/somafm/main.go

test:
	go test ./...

clean:
	go clean
	rm -f $(BINARY_NAME) $(BINARY_NAME)-*

.PHONY: build build-all test clean
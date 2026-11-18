VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
PREFIX ?= /usr/local

.PHONY: build build-darwin test lint install clean

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/watch ./cmd/watch

# Universal macOS binary. lipo ships with the Xcode command line tools.
build-darwin:
	GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/watch-arm64 ./cmd/watch
	GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/watch-amd64 ./cmd/watch
	lipo -create -output bin/watch bin/watch-arm64 bin/watch-amd64

test:
	go test -race -count=1 ./...

lint:
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi
	go vet ./...
	GOOS=darwin go vet ./...

install: build
	install -d $(PREFIX)/bin
	install -m 0755 bin/watch $(PREFIX)/bin/watch

clean:
	rm -rf bin

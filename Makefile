VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
PREFIX ?= /usr/local

.PHONY: build build-darwin dist test lint install clean

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/watch ./cmd/watch

# Universal macOS binary. lipo ships with the Xcode command line tools.
build-darwin:
	GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/watch-arm64 ./cmd/watch
	GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/watch-amd64 ./cmd/watch
	lipo -create -output bin/watch bin/watch-arm64 bin/watch-amd64

# One tarball per macOS architecture, and their checksums, for a release.
dist:
	rm -rf dist && mkdir -p dist
	for arch in arm64 amd64; do \
		name=watch_$(VERSION)_darwin_$$arch; \
		mkdir -p dist/$$name; \
		CGO_ENABLED=0 GOOS=darwin GOARCH=$$arch go build -trimpath -ldflags "$(LDFLAGS)" -o dist/$$name/watch ./cmd/watch; \
		cp README.md dist/$$name/; \
		tar -C dist -czf dist/$$name.tar.gz $$name; \
		rm -rf dist/$$name; \
	done
	cd dist && shasum -a 256 *.tar.gz > checksums.txt

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
	rm -rf bin dist

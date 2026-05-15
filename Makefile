BIN     := lntab
VERSION := $(shell git describe --tags --always --dirty)
LDFLAGS := -ldflags="-s -w -X main.version=$(VERSION)"
TARGETS := \
	linux-amd64 \
	linux-arm64

.PHONY: build install test clean release

build:
	go build $(LDFLAGS) -o $(BIN) .

install:
	go install $(LDFLAGS) .

test:
	go test ./...

clean:
	rm -f $(BIN)
	rm -rf dist/

release: $(TARGETS:%=dist/$(BIN)-$(VERSION)-%)

dist/$(BIN)-$(VERSION)-linux-%: | dist
	GOOS=linux GOARCH=$* go build $(LDFLAGS) -o $@ .

dist/$(BIN)-darwin-%: | dist
	GOOS=darwin GOARCH=$* go build $(LDFLAGS) -o $@ .

dist/$(BIN)-windows-%: | dist
	GOOS=windows GOARCH=$* go build $(LDFLAGS) -o $@.exe .

dist:
	mkdir -p dist

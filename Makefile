GO      ?= go
BIN     := bin/yarmouth
PKG     := ./cmd/yarmouth

.PHONY: all build vet test clean

all: build

build:
	$(GO) build -o $(BIN) $(PKG)

vet:
	$(GO) vet ./...

test:
	$(GO) test ./...

clean:
	rm -rf bin
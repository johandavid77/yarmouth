GO      ?= go
CGO     ?= 0
BIN     := bin/yarmouth
PKG     := ./cmd/yarmouth

.PHONY: all build vet test clean cross

all: build

build:
	CGO_ENABLED=$(CGO) $(GO) build -o $(BIN) $(PKG)

# binario estatico para correr dentro del chroot LFS / sistema desnudo
cross:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build -trimpath -o $(BIN) $(PKG)

vet:
	$(GO) vet ./...

test:
	$(GO) test ./...

clean:
	rm -rf bin
# paginas de manual: misma fuente unica que -h y -h con -V, asi que nunca se
# desincronizan con el binario. Para instalarlas en el LFS:
#   make man && sudo install -Dm644 share/man/man1/yarmouth.1 /usr/share/man/man1/
#             && sudo install -Dm644 share/man/man5/yarmouth.conf.5 /usr/share/man/man5/
.PHONY: man
man: build
	mkdir -p share/man/man1 share/man/man5
	$(BIN) man 1 > share/man/man1/yarmouth.1
	$(BIN) man 5 > share/man/man5/yarmouth.conf.5

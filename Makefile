APP_NAME ?= YealinkNotifier
BIN_DIR  := bin
PKG      := .
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS  := -s -w -X main.version=$(VERSION)

.PHONY: all build test vet fmt run clean

all: build

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(APP_NAME) $(PKG)

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

run:
	go run .

clean:
	rm -rf $(BIN_DIR) dist

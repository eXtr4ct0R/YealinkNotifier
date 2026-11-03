APP_NAME ?= YealinkNotifier
BIN_DIR  := bin
DIST_DIR := dist
PKG      := .
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS  := -s -w -X main.version=$(VERSION)

.PHONY: all build build-windows build-linux build-darwin test vet fmt run clean

# systray (and partially beeep) require cgo, so builds run with CGO_ENABLED=1

all: build

build:
	CGO_ENABLED=1 go build -trimpath -ldflags "$(LDFLAGS) -H windowsgui" -o $(BIN_DIR)/$(APP_NAME) $(PKG)

# Cross-build notes:
#   build-windows from a non-windows host requires MinGW-w64 (CC=x86_64-w64-mingw32-gcc)
#   build-darwin is realistically built on a macOS host/CI, not cross-compiled from Linux
build-windows:
	GOOS=windows GOARCH=amd64 CGO_ENABLED=1 go build -trimpath -ldflags "$(LDFLAGS) -H windowsgui" -o $(DIST_DIR)/$(APP_NAME).exe $(PKG)

build-linux:
	GOOS=linux CGO_ENABLED=1 go build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(APP_NAME) $(PKG)

build-darwin:
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=1 go build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(APP_NAME) $(PKG)

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

run:
	go run .

clean:
	rm -rf $(BIN_DIR) $(DIST_DIR)

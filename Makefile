.PHONY: build release clean

BINARY_SERVER := zaip-server
BINARY_CLIENT := zaip
LDFLAGS := -s -w
GOFLAGS := -trimpath

build:
	go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o dist/$(BINARY_SERVER) ./cmd/server
	go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o dist/$(BINARY_CLIENT) ./cmd/client

release:
	goreleaser release --clean

snapshot:
	goreleaser build --snapshot --clean

clean:
	rm -rf dist/

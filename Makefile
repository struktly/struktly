BIN := $(CURDIR)/.bin
GOLANGCI := $(BIN)/golangci-lint

.PHONY: build test lint fmt tools clean

build:
	go build ./...

test:
	go test ./...

lint: $(GOLANGCI)
	$(GOLANGCI) run

fmt: $(GOLANGCI)
	$(GOLANGCI) fmt

tools: $(GOLANGCI)

$(GOLANGCI): tools/go.mod tools/go.sum
	GOBIN=$(BIN) go -C tools install tool

clean:
	rm -rf $(BIN)

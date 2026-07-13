.PHONY: build test lint

build:
	go build ./...

test:
	go test ./...

lint:
	test -z "$$(gofmt -l .)"
	go vet ./...

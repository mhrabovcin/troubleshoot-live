.PHONY: tests
tests:
	go test ./...

.PHONY: lint
lint:
	golangci-lint run --fix

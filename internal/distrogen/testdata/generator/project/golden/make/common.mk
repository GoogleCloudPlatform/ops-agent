# TODO: This could be better
.PHONY: test
test:
	go test ./...

.PHONY: tidy
tidy:
	go mod tidy

.PHONY: generate
generate:
	go generate ./...

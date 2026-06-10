# TODO: This could be better
# In gpu receivers, we run test twice. Once with gpu support and once without.
# Both need to pass for the component to be "passing tests".
.PHONY: test
test:
	go test ./...
	go test -tags=gpu ./...

.PHONY: tidy
tidy:
	go mod tidy

# We don't want to use the `generated_component_test.go` from mdatagen
# because it doesn't account for the expected error when gpu support
# is turned off.
.PHONY: generate
generate:
	go generate -tags=gpu ./...
	rm generated_component_test.go
.PHONY: local-container-goreleaser
local-container-goreleaser:
	docker buildx build \
		--progress=plain \
		-t otelcol-basic-build \
		-f Dockerfile.goreleaser_releaser \
		..
	CONTAINER_ID=$$(docker create otelcol-basic-build) && \
		docker cp $$CONTAINER_ID:/basic-distro/dist . &&\
		docker rm --force $$CONTAINER_ID

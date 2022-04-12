# IMPORTANT: This Makefile is intended for development use only. Its targets
# are meant for convenience in common development tasks. Please do not build
# any dependencies on this file as everything is subject to change.

test:
	go test ./...

test_confgenerator:
	go test -mod=mod github.com/GoogleCloudPlatform/ops-agent/confgenerator

update_goldens:
	go test -mod=mod github.com/GoogleCloudPlatform/ops-agent/confgenerator -update_golden

update_goldens_v:
	go test -mod=mod github.com/GoogleCloudPlatform/ops-agent/confgenerator -update_golden -v

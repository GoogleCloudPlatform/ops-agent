test:
	go test ./...

test_confgenerator:
	go test -mod=mod github.com/GoogleCloudPlatform/ops-agent/confgenerator

test_metadata:
	go test -mod=mod github.com/GoogleCloudPlatform/ops-agent/integration_test/metadata

update_golden:
	go test -mod=mod github.com/GoogleCloudPlatform/ops-agent/confgenerator -update_golden
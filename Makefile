test:
	go test ./...

test_confgenerator:
	go test -mod=mod github.com/GoogleCloudPlatform/ops-agent/confgenerator

update_goldens:
	go test -mod=mod github.com/GoogleCloudPlatform/ops-agent/confgenerator -update_golden

update_goldens_v:
	go test -mod=mod github.com/GoogleCloudPlatform/ops-agent/confgenerator -update_golden -v

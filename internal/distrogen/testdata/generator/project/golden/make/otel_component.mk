DISTROGEN_BIN ?= distrogen

OTEL_VERSION ?= latest
OTEL_CONTRIB_VERSION ?= latest

STABLE_COMPONENTS_PATTERN = -e "^go.opentelemetry.io/collector/pdata" \
							-e "^go.opentelemetry.io/collector/featuregate" \
							-e "^go.opentelemetry.io/collector/client" \
							-e "^go.opentelemetry.io/collector/featuregate" \
							-e "^go.opentelemetry.io/collector/pdata" \
							-e "^go.opentelemetry.io/collector/confmap" \
							-e "^go.opentelemetry.io/collector/confmap/provider/envprovider" \
							-e "^go.opentelemetry.io/collector/confmap/provider/fileprovider" \
							-e "^go.opentelemetry.io/collector/confmap/provider/httpprovider" \
							-e "^go.opentelemetry.io/collector/confmap/provider/httpsprovider" \
							-e "^go.opentelemetry.io/collector/confmap/provider/yamlprovider" \
							-e "^go.opentelemetry.io/collector/config/configopaque" \
							-e "^go.opentelemetry.io/collector/config/configcompression" \
							-e "^go.opentelemetry.io/collector/config/configretry" \
							-e "^go.opentelemetry.io/collector/config/configtls" \
							-e "^go.opentelemetry.io/collector/config/confignet" \
							-e "^go.opentelemetry.io/collector/consumer"

LIST_DIRECT_MODULES = go list -m -f '{{if not (or .Indirect .Main)}}{{.Path}}{{end}}' all
INCLUDE_COLLECTOR_CORE_COMPONENTS = grep "^go.opentelemetry.io" | grep -v "^go.opentelemetry.io/otel"
INCLUDE_COLLECTOR_STABLE_CORE_COMPONENTS = grep $(STABLE_COMPONENTS_PATTERN)
EXCLUDE_COLLECTOR_STABLE_CORE_COMPONENTS = grep -v $(STABLE_COMPONENTS_PATTERN)
INCLUDE_CONTRIB_COMPONENTS = grep "^github.com/open-telemetry/opentelemetry-collector-contrib"
GO_GET_ALL = xargs --no-run-if-empty -t -I '{}' go get -tags=gpu {}

.PHONY: update-components
update-components: core-components contrib-components

.PHONY: core-components
core-components:
	$(LIST_DIRECT_MODULES) | \
		$(INCLUDE_COLLECTOR_CORE_COMPONENTS) | \
		$(DISTROGEN_BIN) otel_component_versions -otel_version $(OTEL_VERSION) | \
		$(GO_GET_ALL)

.PHONY: contrib-components
contrib-components:
	$(LIST_DIRECT_MODULES) | \
		$(INCLUDE_CONTRIB_COMPONENTS) | \
		$(GO_GET_ALL)@$(OTEL_CONTRIB_VERSION)

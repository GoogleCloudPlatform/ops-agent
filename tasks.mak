# Copyright 2022 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# NOTICE: This file is purely for automation of development tasks.
# No guarantee is made for these commands, and no dependencies should be made on any
# targets. Building the Ops Agent should be done through the Dockerfile.

############
# Setup
############

.PHONY: makefile_symlink
makefile_symlink:
	ln -s tasks.mak Makefile

.PHONY: install_tools
install_tools:
	go install github.com/google/yamlfmt/cmd/yamlfmt@latest
	go install github.com/google/addlicense@master

############
# Build
############

.PHONY: build
build:
	cd submodules/fluent-bit/build && find . ! -name ".empty" -type f -delete
	mkdir -p /tmp/google-cloud-ops-agent
	DOCKER_BUILDKIT=1 docker build -o /tmp/google-cloud-ops-agent . $(ARGS)

.PHONY: fluent_bit_local
fluent_bit_local:
	bash ./builds/fluent_bit.sh $(PWD)/dist

SKIP_JAVA ?= true
.PHONY: otelopscol_local
otelopscol_local:
	SKIP_OTEL_JAVA=${SKIP_JAVA} bash ./builds/otel.sh $(PWD)/dist

############
# Tools
############

.PHONY: addlicense
addlicense:
	addlicense -ignore "submodules/**" -ignore "**/testdata/**" -ignore "**/built-in-config*" -c "Google LLC" -l apache .

.PHONY: addlicense_check
addlicense_check:
	addlicense -check -ignore "submodules/**" -ignore "**/testdata/**" -ignore "**/built-in-config*" -c "Google LLC" -l apache .

.PHONY: addlicense_to
addlicense_to:
	addlicense -c "Google LLC" -l apache $(FILE)

.PHONY: yaml_format
yaml_format:
	yamlfmt

.PHONY: yaml_lint
yaml_lint:
	yamlfmt -lint

.PHONY: compile_dockerfile
compile_dockerfile:
	go run ./dockerfiles

############
# Unit Tests
############

.PHONY: test
test:
	go test ./...

.PHONY: test_confgenerator
test_confgenerator:
	go test ./confgenerator $(if $(UPDATE),-update,)

.PHONY: test_confgenerator_update
test_confgenerator_update: UPDATE=true
test_confgenerator_update: test_confgenerator

.PHONY: test_metadata
test_metadata:
	go test ./integration_test/metadata $(if $(UPDATE),-update,)

.PHONY: test_metadata_update
test_metadata_update: UPDATE=true
test_metadata_update: test_metadata

.PHONY: new_confgenerator_test
new_confgenerator_test:
ifndef TEST_NAME
	$(error "Please provide a TEST_NAME argument")
endif
	mkdir -p ./confgenerator/testdata/goldens/$(TEST_NAME)
	mkdir -p ./confgenerator/testdata/goldens/$(TEST_NAME)/golden
	touch ./confgenerator/testdata/goldens/$(TEST_NAME)/input.yaml

dist/opt/google-cloud-ops-agent/subagents/fluent-bit/bin/fluent-bit:
	$(MAKE) fluent_bit_local

dist/opt/google-cloud-ops-agent/subagents/opentelemetry-collector/otelopscol:
	$(MAKE) otelopscol_local

.PHONY: transformation_test
transformation_test: dist/opt/google-cloud-ops-agent/subagents/fluent-bit/bin/fluent-bit dist/opt/google-cloud-ops-agent/subagents/opentelemetry-collector/otelopscol
	FLB=$(PWD)/dist/opt/google-cloud-ops-agent/subagents/fluent-bit/bin/fluent-bit \
	OTELOPSCOL=$(PWD)/dist/opt/google-cloud-ops-agent/subagents/opentelemetry-collector/otelopscol \
	go test ./transformation_test $(if $(UPDATE),-update,)

.PHONY: transformation_test_update
transformation_test_update: UPDATE=true
transformation_test_update: transformation_test

############
# Integration Tests
############

# NOTE: You will need to set/export PROJECT and TRANSFERS_BUCKET manually for
# these targets. The Makefile will not ascribe defaults for these, since they
# are specific to different user environments.
#
# Defaults are provided for ZONES and PLATFORMS.
#
# If you would like to use a custom build you can provide a REPO_SUFFIX or
# AGENT_PACKAGES_IN_GCS. These targets function fine without either specified.
ZONES ?= us-central1-b
PLATFORMS ?= debian-cloud:debian-11

.PHONY: integration_tests
integration_tests:
	ZONES="${ZONES}" \
	PLATFORMS="${PLATFORMS}" \
	go test -v ./integration_test/ops_agent_test/main_test.go \
	-test.parallel=1000 \
	-tags=integration_test \
	-timeout=4h

.PHONY: third_party_apps_test
third_party_apps_test:
	ZONES="${ZONES}" \
	PLATFORMS="${PLATFORMS}" \
	go test -v ./integration_test/third_party_apps_test.go \
	-test.parallel=1000 \
	-tags=integration_test \
	-timeout=4h

############
# Precommit
############

.PHONY: precommit
precommit: addlicense_check go_vet test transformation_test

.PHONY: precommit_update
precommit_update: addlicense go_vet test_confgenerator_update test_metadata_update test transformation_test_update

############
# Convenience
############

.PHONY: reset_submodules
reset_submodules:
	git submodule foreach --recursive git clean -xfd
	git submodule foreach --recursive git reset --hard
	git submodule update --init --recursive

.PHONY: sync_fork
sync_fork:
	git fetch upstream
	git rebase upstream/main
	git push -f

.PHONY: update_go_modules
update_go_modules:
	go get -t -u ./...

.PHONY: go_vet
go_vet:
	go list ./... | grep -v "generated" | grep -v "/vendor/" | xargs go vet
	GOOS=windows go list ./... | grep -v "generated" | grep -v "/vendor/" | GOOS=windows xargs go vet

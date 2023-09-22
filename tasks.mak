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

makefile_symlink:
	ln -s tasks.mak Makefile

install_tools:
	go install github.com/google/yamlfmt/cmd/yamlfmt@latest
	go install github.com/google/addlicense@master

############
# Build
############

build:
	mkdir -p /tmp/google-cloud-ops-agent	
	DOCKER_BUILDKIT=1 docker build -o /tmp/google-cloud-ops-agent . $(ARGS)

############
# Tools
############

addlicense:
	addlicense -ignore "submodules/**" -ignore "**/testdata/**" -ignore "**/built-in-config*" -c "Google LLC" -l apache . 

addlicense_check:
	addlicense -check -ignore "submodules/**" -ignore "**/testdata/**" -ignore "**/built-in-config*" -c "Google LLC" -l apache . 

# Originally I made the argument to this "PATH" but of course that messes with
# the PATH environment variable. :D
addlicense_to:
	addlicense -c "Google LLC" -l apache $(P)

yaml_format:
	yamlfmt

yaml_lint:
	yamlfmt -lint

compile_dockerfile:
	go run ./dockerfiles

############
# Unit Tests
############

test:
	go test ./...

test_confgenerator_update:
	go test ./confgenerator -update

test_metadata_update:
	go test ./integration_test/metadata -update

new_confgenerator_test:
ifndef TEST_NAME
	$(error "Please provide a TEST_NAME argument")
endif
	mkdir -p ./confgenerator/testdata/goldens/$(TEST_NAME) 
	mkdir -p ./confgenerator/testdata/goldens/$(TEST_NAME)/golden
	touch ./confgenerator/testdata/goldens/$(TEST_NAME)/input.yaml

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
PLATFORMS ?= debian-11

integration_tests:
	ZONES="${ZONES}" \
	PLATFORMS="${PLATFORMS}" \
	go test -v ./integration_test/ops_agent_test.go \
	-test.parallel=1000 \
	-tags=integration_test \
	-timeout=4h

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

precommit: addlicense_check test go_vet

precommit_update: addlicense test_confgenerator_update test_metadata_update test go_vet

############
# Convenience
############

reset_submodules:
	git submodule foreach --recursive git clean -xfd
	git submodule foreach --recursive git reset --hard
	git submodule update --init --recursive

sync_fork:
	git fetch upstream
	git rebase upstream/main
	git push -f

update_go_modules:
	go get -t -u ./...

go_vet:
	go list ./... | grep -v "generated" | grep -v "/vendor/" | xargs go vet
	GOOS=windows go list ./... | grep -v "generated" | grep -v "/vendor/" | GOOS=windows xargs go vet

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
# Tools
############
install_tools:
	go install github.com/google/yamlfmt/cmd/yamlfmt@latest
	go install github.com/google/addlicense@master

addlicense:
	addlicense -ignore "submodules/**" -ignore "**/testdata/**" -ignore "**/built-in-config*" -c "Google LLC" -l apache . 

addlicense_check:
	addlicense -check -ignore "submodules/**" -ignore "**/testdata/**" -ignore "**/built-in-config*" -c "Google LLC" -l apache . 

yaml_format:
	yamlfmt

yaml_lint:
	yamlfmt -lint

############
# Unit Tests
############

test:
	go test ./...

test_confgenerator:
	go test ./confgenerator

test_confgenerator_update:
	go test ./confgenerator -update

test_metadata:
	go test ./integration_test/metadata

test_metadata_update:
	go test ./integration_test/metadata -update_golden

test_metadata_validate:
	go test ./integration_test/validate_metadata_test.go

############
# Precommit
############

precommit: addlicense_check yaml_lint test_confgenerator test_metadata validate_metadata

precommit_update: addlicense yaml_format test_confgenerator_update test_metadata_update validate_metadata
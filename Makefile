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

test:
	go test ./...

test_confgenerator:
	go test -mod=mod github.com/GoogleCloudPlatform/ops-agent/confgenerator

test_metadata:
	go test -mod=mod github.com/GoogleCloudPlatform/ops-agent/integration_test/metadata

update_golden:
	go test -mod=mod github.com/GoogleCloudPlatform/ops-agent/confgenerator -update_golden
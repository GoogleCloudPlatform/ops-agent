# Copyright 2020 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

go_library(
    name = "confgenerator",
    srcs = ["confgenerator.go"],
    deps = [
        "//cloud/monitoring/agents/unified_agents/fluentbit/conf",
        "//third_party/golang/yaml",
    ],
)

go_test(
    name = "confgenerator_test",
    size = "small",
    srcs = ["confgenerator_test.go"],
    data = glob(["testdata/**"]),
    library = ":confgenerator",
    deps = [
        "//base/go:runfiles",
        "//third_party/golang/cmp",
    ],
)

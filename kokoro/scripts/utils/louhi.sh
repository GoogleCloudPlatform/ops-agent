# Copyright 2022 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# A helper for parsing YAML files.
# Ex: VALUE=$(yaml ~/my_yaml_file.yaml "['a_key']")
function yaml() {
  python3 -c "import yaml
data=yaml.safe_load(open('$1'))$2
if type(data)==list:
  print(','.join(str(elem) for elem in data))
else:
 print(data)"
}

function parse_louhi_tag() {
  # if _LOUHI_TAG_NAME is defined, set TARGET and ARCH env vars by parsing it.
  # Example value: louhi/2.46.0/shortref/windows/x86_64/start
  if [[ -n "${_LOUHI_TAG_NAME:-}" ]]; then
    local -a _LOUHI_TAG_COMPONENTS=(${_LOUHI_TAG_NAME//\// })  
    export REPO_SUFFIX="${_LOUHI_TAG_COMPONENTS[2]}"  # the shortref is the repo suffix
    TARGET="${_LOUHI_TAG_COMPONENTS[3]}"
    ARCH="${_LOUHI_TAG_COMPONENTS[4]}"
    export ARTIFACT_REGISTRY_PROJECT="${_STAGING_ARTIFACTS_PROJECT_ID}"  # Louhi is responsible for passing this.
  fi

  EXT=$(yaml project.yaml "['targets']['${TARGET}']['package_extension']")
  if [[ "${EXT}" == "deb" ]]; then
    export REPO_CODENAME="${TARGET}-${ARCH}"
  fi
}
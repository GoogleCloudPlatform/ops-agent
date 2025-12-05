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

function populate_env_vars_from_louhi_tag_if_present() {
  # Populate TARGET, ARCH, and REPO_SUFFIX using the environment variables
  # provided directly by Louhi.
  if [[ -n "${_LOUHI_REF_SHA:-}" ]]; then
    TARGET="${_DISTRO}"
    ARCH="${_ARCH}"

    if [[ "${INSTALL_AGENT_UNDER_TEST:-1}" == "1" ]]; then
      export REPO_SUFFIX="${_RELEASE_ID}"
      export ARTIFACT_REGISTRY_PROJECT="${_TESTING_ARTIFACTS_PROJECT_ID}"  # Louhi is responsible for passing this.

      EXT=$(yaml project.yaml "['targets']['${TARGET}']['package_extension']")
      if [[ "${EXT}" == "deb" ]]; then
        export REPO_CODENAME="${TARGET}-${ARCH//_/-}"
      fi
    fi
  fi
}

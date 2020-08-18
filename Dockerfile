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

# Build as DOCKER_BUILDKIT=1 docker build -o /tmp/out .
# Generated tarball(s) will end up in /tmp/out

FROM debian:buster AS debian

RUN apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get -y install golang git systemd \
    build-essential cmake bison flex file libsystemd-dev

COPY . /work
WORKDIR /work
RUN ./build.sh

FROM scratch
COPY --from=debian /tmp/google-ops-agent.tgz /out/google-ops-agent-debian-buster.tgz

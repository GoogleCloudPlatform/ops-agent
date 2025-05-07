#!/bin/bash
# Copyright 2024 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -x -e

otel_dir=/opt/google-cloud-ops-agent/subagents/opentelemetry-collector
DESTDIR="${1}${otel_dir}"

mkdir -p $DESTDIR

LDFLAGS=""

# If this is not a local build, then we build otel java and add it to
# the ldflags.
if [ "$SKIP_OTEL_JAVA" != "true" ]; then
    cd submodules/opentelemetry-java-contrib

    mkdir -p "$DESTDIR"
    ./gradlew --no-daemon -Djdk.lang.Process.launchMechanism=vfork :jmx-metrics:build
    cp jmx-metrics/build/libs/opentelemetry-jmx-metrics-*-alpha-SNAPSHOT.jar "$DESTDIR/opentelemetry-java-contrib-jmx-metrics.jar"

    # Rename LICENSE file because it causes issues with file hash consistency due to an unknown
    # issue with the debuild/rpmbuild processes. Something is unzipping the jar in a case-insensitive
    # environment and having a conflict between the LICENSE file and license/ directory, leading to a changed jar file
    mkdir ./META-INF
    unzip -j "$DESTDIR/opentelemetry-java-contrib-jmx-metrics.jar" "META-INF/LICENSE" -d ./META-INF
    zip -d "$DESTDIR/opentelemetry-java-contrib-jmx-metrics.jar" "META-INF/LICENSE"
    mv ./META-INF/LICENSE ./META-INF/LICENSE.renamed
    zip -u "$DESTDIR/opentelemetry-java-contrib-jmx-metrics.jar" "META-INF/LICENSE.renamed"

    # Using array assignment to drop the filename from the sha256sum output
    JAR_SHA_256=($(sha256sum "$DESTDIR/opentelemetry-java-contrib-jmx-metrics.jar"))
    LDFLAGS="-X github.com/open-telemetry/opentelemetry-collector-contrib/receiver/jmxreceiver.MetricsGathererHash=$JAR_SHA_256"

    cd ../..
fi

cd submodules/opentelemetry-operations-collector/otelopscol
GO_BIN=/usr/local/go/bin/go \
TARGETARCH=$(/usr/local/go/bin/go env GOARCH) \
COLLECTOR_LD_FLAGS="$LDFLAGS" \
COLLECTOR_BUILDVCS="false" \
COLLECTOR_BUILD_TAGS="gpu" \
    make build
cp ./otelopscol "$DESTDIR/otelopscol"

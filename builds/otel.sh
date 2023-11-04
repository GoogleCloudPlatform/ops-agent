#!/bin/sh
set -x -e
DESTDIR=$1
otel_dir=/opt/google-cloud-ops-agent/subagents/opentelemetry-collector
DESTDIR="${DESTDIR}${otel_dir}"

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

cd ../opentelemetry-operations-collector
# Using array assignment to drop the filename from the sha256sum output
JAR_SHA_256=($(sha256sum "$DESTDIR/opentelemetry-java-contrib-jmx-metrics.jar"))
go build -tags=gpu -buildvcs=false -o "$DESTDIR/otelopscol" \
-ldflags "-X github.com/open-telemetry/opentelemetry-collector-contrib/receiver/jmxreceiver.MetricsGathererHash=$JAR_SHA_256" \
./cmd/otelopscol

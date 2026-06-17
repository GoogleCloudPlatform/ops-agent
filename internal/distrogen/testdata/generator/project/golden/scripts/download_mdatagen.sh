#!/bin/bash

# This script will download mdatagen at the otel version specified in the first argument.
# This script is needed to circumvent this issue:
# https://github.com/open-telemetry/opentelemetry-collector/issues/9281

set -e

OTEL_VERSION=$1

if [[ -z "$OTEL_VERSION" ]]; then
  echo "Please specify the OpenTelemetry Collector version to use."
  exit 1
fi

TMP_DIR=$(mktemp -d)

cd $TMP_DIR
go mod init tempmod
go get -u go.opentelemetry.io/collector/cmd/mdatagen@$OTEL_VERSION
go install go.opentelemetry.io/collector/cmd/mdatagen

rm -rf $TMP_DIR

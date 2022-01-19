#!/bin/bash
set -e
set -u
set -x
set -o pipefail

GOOGLE_APPLICATION_CREDENTIALS="${HOME}/credentials.json" \
  PROJECT=asdf \
  ZONE=us-central1-b \
  PLATFORMS=debian-10 \
  AGENTS_TO_TEST=ops-agent \
  go test -v ops_agent_test.go \
  -test.parallel=1000 \
  -tags=integration_test \
  -timeout=4h


#!/bin/bash
#ops-agents-public-buckets-test-logs/prod/stackdriver_agents/testing/consumer/ops_agent/build/bullseye_x86_64/6195/20250814-084737
#
#
#

#export AGENT_PACKAGES_IN_GCS=gs://ops-agents-public-buckets-test-logs/prod/stackdriver_agents/testing/consumer/ops_agent/build/bullseye_x86_64/6195/20250814-084737/result
export AGENT_PACKAGES_IN_GCS=gs://ops-agents-public-buckets-test-logs/prod/stackdriver_agents/testing/consumer/ops_agent/build/bullseye_x86_64/6247/20250818-104743/result
TRANSFERS_BUCKET=westphalrafael_ops_agent PROJECT=westphalrafael-dev ZONES=us-central1-b IMAGE_SPECS=debian-cloud:debian-11 go test -v ./integration_test/ops_agent_test/main_test.go -tags=integration_test -run ^TestPrometheusMetricsWithJSONExporter$ -timeout=4h

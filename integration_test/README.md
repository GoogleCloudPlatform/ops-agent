# Integration Testing

Integration tests are implemented as Kokoko builds that run on each PR. The
builds first build the Ops Agent and then run tests on that agent. The Kokoro
builds are split up by distro.

## Setup

You will need:

1.  A GCP project to run VMs in. This is referred to as `${PROJECT}` in the
    following instructions.
2.  A GCS bucket that is used to transfer files onto the testing VMs. This is
    referred to as `${TRANSFERS_BUCKET}`.
3.  `gcloud` to be installed. Run `gcloud auth login` to set up `gcloud`
    authentication (if you haven't done that already).
4.  To give the tests credentials to be able to access Google APIs as you,
    run the following command and do what it says (it may ask you to run
    a command on a separate machine if your main machine doesn't have the
    ability to open a browser window):

    ```
    gcloud --billing-project="${PROJECT}" auth application-default login
    ```

Once these steps are complete, you should be able to run the below commands.

## Ops Agent Test

This test exercises "core" features of the Ops Agent such as watching syslog or
a custom log file. It is implemented in ops_agent_test.go. It can be run outside
of Kokoro with some setup (see above).

### Testing Command

When the setup steps are complete, you can run ops_agent_test (for Linux)
like this:

```
PROJECT="${PROJECT}" \
TRANSFERS_BUCKET="${TRANSFERS_BUCKET}" \
ZONE=us-central1-b \
PLATFORMS=debian-10 \
go test -v ops_agent_test.go \
 -test.parallel=1000 \
 -tags=integration_test \
 -timeout=4h
```

Testing on Windows is tricky because it requires a suitable value of
WINRM_PAR_PATH, and for now only Googlers can build winrm.par to supply it at
runtime.

The above command will run the tests against the stable Ops Agent. To test
against a pre-built but unreleased agent, you can use add the
AGENT_PACKAGES_IN_GCS environment variable onto your command like this:

```
AGENT_PACKAGES_IN_GCS=gs://ops-agents-public-buckets-test-logs/prod/stackdriver_agents/testing/consumer/ops_agent/presubmit_github/debian/166/20220215-095636/agent_packages \
```

You can obtain such a URI by:

1.  take a previous Kokoro run with a successful build and get the
    "gsutil URI" to `+build_and_test.txt` from the Google Cloud Storage browser
    page. For example:
    `gs://ops-agents-public-buckets-test-logs/prod/stackdriver_agents/testing/consumer/ops_agent/presubmit_github/debian/166/20220215-095636/logs/+build_and_test.txt`
2.  Replace `logs/+build_and_test.txt` at the end of the URI with
    `agent_packages` and pass that as `AGENT_PACKAGES_IN_GCS`.

## Third Party Apps Test

This test attempts to verify, for each application in `supported_applications.txt`,
that the application can be installed on a real GCE VM and that a single
representative metric is successfully uploaded to Google Cloud Monitoring.

The test is designed to be highly parameterizable. It reads various files from
`third_party_apps_data` and decides what to do based on their contents. First
it reads `test_config.yaml` and uses that to set some testing options. See the
"test_config.yaml" section below. Then it reads
`agent/<platform>/supported_applications.txt` to determine
which applications to test. Each application is tested in parallel. For each,
the test will:

1.  Bring up a GCE VM
1.  Install the application on the VM by running
    `applications/<application>/<platform>/install` on the VM
1.  Install the Ops Agent (built from the contents of the PR) on the VM
1.  Configure the the Ops Agent to look for the application's logs/metrics by
    running `applications/<application>/enable` on the VM.
1.  Run `applications/<application>/exercise` script to send some load to
    the application, so that we can get it to generate some logs/metrics
1.  Wait for up to 7 minutes for logs matching the expectations in
    `applications/<application>/expected_logs.yaml` to appear in the Google
    Cloud Logging backend.
1.  Wait up to 7 minutes for metrics matching the expectations in expected_metrics of
    `applications/<application>/metadata.yaml` to appear in the Google Cloud
    Monitoring backend.

The test is designed so that simply modifying files in the
`third_party_apps_data` directory is sufficient to get the test runner to do the
right thing. But we do expect that we will need to make big changes to both the
data directory and the test runner before it is really meeting our needs.

### Adding a new third-party application

You will need to add and modify a few files. Start by adding your new
application to `agent/<linux_or_windows>/supported_applications.txt`

Then, inside `applications/<application>/`:

1.  `<platform>/install` to install the application,
1.  `enable` to configure the Ops Agent to read the application's metrics
    exposed in the previous step.
1.  (if necessary) `exercise`. This is only needed
    sometimes, e.g. to get the application to log to a particular file.
1.  Inside `metadata.yaml`, add `short_name`, e.g. `solr` and `long_name`, e.g.
    `Apache Solr`.
1.  Some integration will have steps for configuring instance, e.g. [Apache Hadoop](https://cloud.google.com/stackdriver/docs/solutions/agents/ops-agent/third-party/hadoop#configure-instance).
1.  (if you want to test logging) add `expected_logs` in metadata.yaml
1.  (if you want to test metrics) add `expected_metrics` in metadata.yaml

### expected_logs

We use `expected_logs` inside `metadata.yaml` file both as a test artifact and as a source for documentation, e.g. [Apache(httpd) public doc](https://cloud.google.com/stackdriver/docs/solutions/agents/ops-agent/third-party/apache#monitored-logs). All logs ingested from the integration should be documented here.

A sample `expected_logs` snippet looks like:

```yaml
expected_logs:
- log_name: apache_access
  fields:
  - name: httpRequest.requestMethod
    value_regex: GET
    type: string
    description: HTTP method
  - name: jsonPayload.host
    type: string
    description: Contents of the Host header
  - name: jsonPayload.user
    type: string
    description: Authenticated username for the request
```

- `name`: required, it will be used in e2e searching for the matching logs
- `type`: required, informational
- `description`: required, informational
- `value_regex`: optional, the value of the LogEntry field will be used in e2e searching for the matching logs.

### expected_metrics

We use `expected_metrics` inside `metadata.yaml` file both as a test artifact and as a source for documentation. All metrics ingested from the integration should be documented here.

A sample `expected_metrics` snippet looks like:

```yaml
expected_metrics:
- type: workload.googleapis.com/apache.current_connections
  value_type: INT64
  kind: GAUGE
  monitored_resource: gce_instance
  labels:
    server_name: .*
  representative: true
```

`type`, `value_type` and `kind` come directly from the metric descriptor for that metric. `monitored_resource` should always be `gce_instance`.

`labels` is an exhaustive list of labels associated with the metric. Each key in `labels` is the label name, and its value is a regular expression. During the test, each label returned by the time series for that metric is checked against `labels`: every label in the time series must be present in `labels`, and its value must match the regular expression.

For example, if a metric defines a label `operation` whose values can only be `read` or `write`, then an appropriate `labels` map in `expected_metrics` would be as follows:

```yaml
  labels:
    operation: read|write
```

Exactly one metric from each integration's `expected_metrics` must have `representative: true`. This metric can be used to detect when the integration is enabled. A representative metric cannot be optional.

With `optional: true`, the metric will be skipped during the test. This can be useful for metrics that are not guaranteed to be present during the test, for example due to platform differences or unimplemented test setup procedures. An optional metric cannot be representative.

`expected_metrics` can be generated or updated using `generate_expected_metrics.go`:

```
PROJECT="${PROJECT}" \
SCRIPTS_DIR=third_party_apps_data \
go run -tags=integration_test \
./cmd/generate_expected_metrics
```

This queries all metric descriptors under `workload.googleapis.com/`, `agent.googleapis.com/iis/`, and `agent.googleapis.com/mssql/`. The optional variable `FILTER` is also provided to make it quicker to test individual integrations. For example:

```
PROJECT="${PROJECT}" \
SCRIPTS_DIR=third_party_apps_data \
FILTER='metric.type=starts_with("workload.googleapis.com/apache")' \
go run -tags=integration_test \
./cmd/generate_expected_metrics
```

Existing `expected_metrics` files are updated with any new metrics that are retrieved. Any existing metrics within the file will be overwritten with newly retrieved ones, except that existing `labels` patterns are preserved.

### Testing Command

This needs the same setup steps as the Ops Agent test (see above). The command
is nearly the same, just replace `ops_agent_test.go` with
`third_party_apps_test.go`:

```
PROJECT="${PROJECT}" \
TRANSFERS_BUCKET="${TRANSFERS_BUCKET}" \
ZONE=us-central1-b \
PLATFORMS=debian-10 \
go test -v third_party_apps_test.go \
 -test.parallel=1000 \
 -tags=integration_test \
 -timeout=4h
```

As above, you can supply `AGENT_PACKAGES_IN_GCS` to test a pre-built agent.

# Test Logs

The Kokoro presubmit will have a "Details" link next to it. Clicking there
will take you to a publicly-visible GCS bucket that contains various log files.
It's a little tricky to figure out which one(s) to look at first, so here's a
guide for that.

TLDR: start in `build_and_test.txt` to see what failed, then drill down to
the corresponding `main_log.txt` from there.

Here is the full contents uploaded to the GCS bucket for a single test run.
The "Details" link takes you directly to the "logs" subdirectory to save you
a hop. The following is sorted roughly in descending order of usefulness.

```
├── logs
|   ├── build_and_test.txt
|   ├── sponge_log.xml
|   └── TestThirdPartyApps_ops-agent_debian-10_nginx
|       ├── main_log.txt
|       ├── syslog.txt
|       ├── logging-module.log.txt
|       ├── journalctl_output.txt
|       ├── systemctl_status_for_ops_agent.txt
|       ├── otel.yaml.txt
|       ├── VM_initialization.txt
|       ├── fluent_bit_main.conf.txt
|       └── fluent_bit_parser.conf.txt
└── agent_packages
    ├── google-cloud-ops-agent_2.0.5~debian10_amd64.deb
    └── google-cloud-ops-agent-dbgsym_2.0.5~debian10_amd64.deb
```

Let's go through each of these files and discuss what they are.

TODO: Document log files for a Windows VM.

*   `build_and_test.txt`: The top-level log that holds the stdout/stderr for
    the Kokoro job. Near the bottom is a summary of which tests passed and
    which ones failed.
*   `sponge_log.xml`: Structured data about which tests
    passed/failed, but not very human readable.
*   `main_log.txt`: The main log for the particular test shard (e.g.
    `TestThirdPartyApps_ops-agent_debian-10_nginx`) that ran. This is the place
    to start if you are wondering what happened to a particular shard.
*   `syslog.txt`: The system's `/var/log/{syslog,messages}`. Highly useful.
    OTel collector logs can be found here by searching for `otelopscol`.
*   `logging-module.log.txt`: The Fluent-Bit log file.
*   `journalctl_output.txt`: The output of running `journalctl -xe`. Useful
    when the Ops Agent can't start/restart properly, often due to malformed
    config files.
*   `otel.yaml.txt`: The generated config file used to start the OTel collector.
*   `VM_initialization.txt`: Only useful to look at when we can't bring up a
    fresh VM properly.
*   `fluent_bit_main.conf.txt`, `fluent_bit_parser.conf.txt`: Fluent-Bit config
    files.

The `agent_packages` directory contains the package files built from the PR
and installed on the VM for testing.

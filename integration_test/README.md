# Integration Testing

Integration tests are implemented as Kokoko builds that run on each PR. The
builds first build the Ops Agent and then run tests on that agent. The Kokoro
builds are split up by distro.

## Setup

You will need a GCP project to run VMs in and a GCS bucket that is used to
transfer files onto the testing VMs. These are referred to as `${PROJECT}`
and `${TRANSFERS_BUCKET}` in the following instructions.

You will need gcloud installed. Run `gcloud auth login` if you haven't
done so already.

Next, follow the setup instructions for either User Credentials or
Service Account Credentials.

### Setup (User Credentials)

To give the tests credentials to be able to access Google APIs as you,
run the following command and do what it says (it may ask you to run
a command on a separate machine if your main machine doesn't have the
ability to open a browser window):

```
gcloud --billing-project="${PROJECT}" auth application-default login
```

That's it! Now the test commands should be able to authenticate as you.

NOTE: this way of using user credentials is new and may have unforeseen
problems.

### Setup (Service Account Credentials)

You will need a service account with permissions to run various
operations in the project. You can set up such a service account with
a few commands:

```
gcloud iam service-accounts create "service-account-$(whoami)" \
  --project "${PROJECT}" \
  --display-name "service-account-$(whoami)"  
```

```
for ROLE in \
    logging.logWriter \
    monitoring.metricWriter \
    stackdriver.resourceMetadata.writer \
    logging.viewer \
    monitoring.viewer \
    stackdriver.resourceMetadata.viewer \
  ; do
      gcloud projects add-iam-policy-binding "${PROJECT}" \
          --member "serviceAccount:service-account-$(whoami)@${PROJECT}.iam.gserviceaccount.com" \
          --role "roles/${ROLE}" > /dev/null
  done
```

You will also need to give your service account read/write permissions on
`${TRANSFERS_BUCKET}`. To do this, register your service account as principal
with role "Storage Admin" to the bucket:

```
gsutil iam ch \
  "serviceAccount:service-account-$(whoami)@${PROJECT}.iam.gserviceaccount.com:roles/storage.admin" \
  "gs://${TRANSFERS_BUCKET}"
```

Finally, download your credentials as a JSON file:

```
gcloud iam service-accounts keys create $HOME/credentials.json \
  --project "${PROJECT}" \
  --iam-account "service-account-$(whoami)@${PROJECT}.iam.gserviceaccount.com"
```

When runninng the test commands below, you must also pass the
environment variable
`GOOGLE_APPLICATION_CREDENTIALS="${HOME}/credentials.json"` to the
test command.

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
WINRM_PAR_PATH, and for now only Goooglers can build winrm.par to supply it at
runtime.

The above command will run the tests against the stable Ops Agent. To test
against a pre-built but unreleased agent, you can use add the
AGENT_PACKAGES_IN_GCS environment variable onto your command like this:

```
AGENT_PACKAGES_IN_GCS=gs://ops-agents-public-buckets-test-logs/prod/stackdriver_agents/testing/consumer/ops_agent/presubmit_github/debian/166/20220215-095636/agent_packages \
```

You can obtain such a URI by taking a previous Kokoro run with
a successful build and getting the "gsutil URI" to `+build_and_test.txt`
from Pantheon. For example:

```
gs://ops-agents-public-buckets-test-logs/prod/stackdriver_agents/testing/consumer/ops_agent/presubmit_github/debian/166/20220215-095636/logs/+build_and_test.txt
```

Then replace `logs/+build_and_test.txt` at the end of the URI with
`agent_packages` and pass that as `AGENT_PACKAGES_IN_GCS`.

## Third Party Apps Test

This test attempts to verify, for each application in `supported_applications.txt`,
that the application can be installed on a real GCE VM and that a single
representative metric is successfully uploaded to Google Cloud Monitoring.

The test is designed to be highly parameterizable. It reads various files from
`third_party_apps_data` and decides what to do based on their contents. First
it reads `test_config.yaml` and uses that to set some testing options. See the
"test_config.yaml" section below. Then it reads
`agent/ops-agent/<platform>/supported_applications.txt` to determine
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
1.  Wait up to 7 minutes for the metric from
    `applications/<application>/metric_name.txt` to appear in the Google Cloud
    Monitoring backend.

The test is designed so that simply modifying files in the
`third_party_apps_data` directory is sufficient to get the test runner to do the
right thing. But we do expect that we will need to make big changes to both the
data directory and the test runner before it is really meeting our needs.

### Adding a new third-party application

You will need to add and modify a few files. Start by adding your new
application to `agent/ops-agent/<linux_or_windows>/supported_applications.txt`

Then, inside `applications/<application>/`:

1.  `<platform>/install` to install the application,
1.  `enable` to configure the Ops Agent to read the application's metrics
    exposed in the previous step.
1.  (if necessary) `exercise`. This is only needed
    sometimes, e.g. to get the application to log to a particular file.
1.  (if you want to test logging) `expected_logs.yaml`
1.  (if you want to test metrics) `metric_name.txt`

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
    └── ops-agent
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

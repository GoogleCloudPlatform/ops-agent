# Integration Testing asdf

Currently the only thing that is implemented is a test for third-party
application support. This test is currently implemented as a Kokoro build
that is run on all PRs. The test builds the Ops Agent on one distro (currently
Debian 10) and attempts to verify, for each application in
`supported_applications.txt`, that the application can be installed on a real GCE
VM and that a single representative metric is successfully uploaded to Google Cloud
Monitoring.

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
1.  Configure the application to expose its metrics by running
    `applications/<application>/<platform>/post` on the VM. This might
    be a no-op for some applications.
1.  Configure the the Ops Agent to look for the application's logs/metrics by
    running `agent/ops-agent/<platform>/enable_<application>` on the VM.
1.  Run `applications/<application>/exercise` script to send some load to
    the application, so that we can get it to generate some logs/metrics
1.  Wait for up to 7 minutes for logs matching the expectations in 
    `applications/<application>/expected_logs.yaml` to appear in the Google
    Cloud Logging backend.
1.  Wait up to 7 minutes for the metric from
    `applications/<application>/metric_name.txt` to appear in the Google Cloud
    Monitoring backend.

The code for the test runner is not open source yet, unfortunately.

The test is designed so that simply modifying files in the
`third_party_apps_data` directory is sufficient to get the test runner to do the
right thing. But we do expect that we will need to make big changes to both the
data directory and the test runner before it is really meeting our needs.

NOTE: Currently there are various directories that are included in
`third_party_apps_data` that are unused, such as everything in the
`agent/metrics/` directory. These are provided for a few reasons:

1.  To serve as an example for how the directory structure is expected to look
    as the number of platforms and applications increases,
1.  because I uploaded our existing data directory mostly as-is, and it is
    currently being used to test agents besides the Ops Agent, and
1.  to be a starting point so that nobody has to rewrite our logic for
    installing, e.g. redis on CentOS 7.
    
# Adding a new third-party application

You will need to add a few files, and possibly change what's there currently,
since much of it is there only as a starting point and example.

For now, the test only runs on debian-10, so the list of files to edit can be
simplified to:

1.  `agent/ops-agent/linux/supported_applications.txt`
1.  `applications/<application>/debian_ubuntu/install` (may already exist) to
    install the application,
1.  `applications/<application>/debian_ubuntu/post` (may already exist) to
    configure the application to expose metrics somewhere. This might be a
    no-op for some applications. If so, just leave the file empty.
1.  `agent/ops-agent/linux/enable_<application>` to configure the Ops Agent to
    read the application's metrics exposed in the previous step.
1.  (if necessary) `applications/<application>/exercise`. This is only needed
    sometimes, e.g. to get the application to log to a particular file.
1.  (if you want to test logging) `applications/<application>/expected_logs.yaml`

# test_config.yaml

This file sets some options that alter how the test runs. An example is:

```
platforms_override:
  - centos-7
  - sles-12
retries: 0
```

`platforms_override` is a list of strings (default is just debian-10 for now),
and that will tell the test to run on those platforms (which are really image
families). Note: the way the Kokoro build is set up, it will *build* all Linux
distros no matter what, but it only run the *test* on the platforms in
`platforms_override`.

`retries` configures the number of retries to do for certain errors. These
retries only apply to errors running the per-application `install` or `post`
scripts. Note that a value of 0 means to attempt the test once, but to skip
retrying the failures. This is a very useful thing to do when debugging new
scripts.

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

*   `build_and_test.txt`: The top-level log that holds the stdout/stderr for
    the Kokoro job. Near the bottom is a summary of which tests passed and
    which ones failed.
*   `sponge_log.xml`: Structured data about which tests
    passed/failed, but not very human readable.
*   `main_log.txt`: The main log for the particular test shard (e.g.
    `TestThirdPartyApps_ops-agent_debian-10_nginx`) that ran. This is the place
    to start if you are wondering what happened to a particular shard.

The rest of these files are only uploaded if the test fails.

*   `syslog.txt`: The system's `/var/log/{syslog,messages}`. Highly useful.
    OTel collector logs can be found here by searching for `otelopscol`.
*   `logging-module.log.txt`: The Fluent-Bit log file. Not useful right now.
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
    

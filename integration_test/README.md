# Integration Testing

Currently the only thing that is implemented is a test for third-party
application support. This test is currently implemented as a Kokoro build
that is run on all PRs. The test builds the Ops Agent on one distro (currently
Debian 10) and attempts to verify, for each application in
`supported_applications.txt`, that the application can be installed on a real GCE
VM and that a single representative metric is successfully uploaded to Google Cloud
Monitoring.

Currently the only supported app is called `noop` and all of its
install/configure scripts are empty.

The test is designed to be highly parameterizable. It reads various files from
`third_party_apps_data` and decides what to do based on their contents. First
it reads `agent/ops-agent/<platform>/supported_applications.txt` to determine
which applications to test. Each application is tested in parallel. For each,
the test will:

1.  Bring up a GCE VM
1.  Install the application on the VM by running
    `applications/<application>/<platform>/install` on the VM
1.  Install the Ops Agent (built from the contents of the PR) on the VM
1.  Configure the application to expose its metrics by running
    `applications/<application>/<platform>/post` on the VM
1.  Configure the the Ops Agent to look for the application's metrics by
    running
    `agent/ops-agent/<platform>/enable_<application>` on the VM
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
    configure the application to expose metrics somewhere.
1.  `agent/ops-agent/linux/enable_<application>` to configure the Ops Agent to
    read the application's metrics exposed in the previous step.

# Seeing Test Logs

Unfortunately, the test logs are only visible to Googlers at the moment. We are
working to fix that.

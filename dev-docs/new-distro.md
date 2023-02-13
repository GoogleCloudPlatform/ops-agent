# Adding Support for a New Distro

These instructions cover part of what is needed to add support for a new distro;
the rest of the steps are documented in a Google-internal markdown doc.

## Setup

The following steps refer to `$DISTRO_SHORT` and `$DISTRO_FAMILY`. You will
need to replace these with appropriate names for your new distro. Example
values for these variables are: `hirsute` for `$DISTRO_SHORT` and
`ubuntu-2104` for `$DISTRO_FAMILY`.

NOTE: Some Ubuntu releases are LTS and some are not. For LTS releases, make
sure the distro family ends in `-lts`. For example, `ubuntu-2004-lts`.

For a list of existing distro families, consult
[image_lists.gcl](https://github.com/GoogleCloudPlatform/ops-agent/blob/master/kokoro/config/test/image_lists.gcl)
or run `gcloud compute images list` and look at the `FAMILY` column.


## Adding Build Support

On your dev branch, add an entry to `dockerfileArguments` in
[compile.go](https://github.com/GoogleCloudPlatform/ops-agent/blob/master/dockerfiles/compile.go)
for the new distro. You can duplicate the most recent distro as a starting
point. Then iterate on your change to `compile.go` by running:

```shell
go run dockerfiles/compile.go && docker build --target $DISTRO_SHORT -t scratch .
```

See [Troubleshooting](#troubleshooting) if you have problems. Once that
succeeds and `make test` succeeds you can proceed to the next step. You may
need to come back to make further changes to `compile.go` in a later step,
when triggering Kokoro to run tests, since some Dockerfile errors can
manifest as runtime errors that won't show up until tests are run.

### Running `ops_agent_test` against the new distro

1.  Temporarily repurpose one of the existing Kokoro builds for testing
    your new distro. For example, in
    [build/presubmit/bullseye.gcl](https://github.com/GoogleCloudPlatform/ops-agent/blob/master/kokoro/config/build/presubmit/bullseye.gcl),
    replace `bullseye` with `$DISTRO_SHORT` and change `deb` to `rpm` if
    needed. Then in
    [test/ops_agent/bullseye.gcl](https://github.com/GoogleCloudPlatform/ops-agent/blob/master/kokoro/config/test/ops_agent/bullseye.gcl),
    replace `image_lists.debian.distros.bullseye.presubmit` with
    `$DISTRO_FAMILY`. This will cause the `bullseye` Kokoro build to build
    and test your distro instead.

1.  Make a PR at this point if you haven't already. GitHub will kick off
    unit tests and integration tests for your PR. **Ignore any
    `third_party_apps_test` failures for now, since we will not enable that
    test for your new distro until later.** (See
    [later section on third_party_apps_test](#running-third-party-apps-test-against-the-new-distro)
    for details.)

1.  Once builds and "Ops Agent integration test" (AKA `ops_agent_test`) are
    passing, Revert the temporary changes to the Kokoro configs (the two
    `bullseye.gcl` files in the earlier step). Get your PR reviewed and
    merge it to `master`.

### Running `third_party_apps_test` against the new distro

For new distros, we require `third_party_apps_test` to be run against the new
distro, and based on the results we may decide to either:

*   start running the test regularly (if the tests are passing)
*   proceed with the release despite some test failures (if the failures appear
    to be with the test infrastructure), or
*   block the release (if the failures appear to be with the Ops Agent itself).

Most of the time, failures with the test infrastructure manifest as failures to
install applications on the new distro. These are common and can be tricky to
fix. Furthermore, they can mask real issues. It is safer to ignore errors like
this if relatively few applications have install errors and the applications
with install errors have few users in production.

For other errors, file a blocking bug and discuss with the team.

The rest of these instructions are for how to try out `third_party_apps_test` on
your new distro and see what fails.

The instructions are very similar to the instructions for `ops_agent_test`.

1.  Temporarily repurpose one of the existing Kokoro builds for testing your
    new distro. Make a new git branch, and make the following changes
    ([Sample PR](https://github.com/GoogleCloudPlatform/ops-agent/pull/1044)):

    *   In
        [build/presubmit/bullseye.gcl](https://github.com/GoogleCloudPlatform/ops-agent/blob/master/kokoro/config/build/presubmit/bullseye.gcl),
        set `DISTRO` to `'$DISTRO_SHORT'` (instead of `'bullseye'`) and
        change `deb` to `rpm` if needed.
    *   In
        [test/third_party_apps/bullseye.gcl](https://github.com/GoogleCloudPlatform/ops-agent/blob/master/kokoro/config/test/third_party_apps/bullseye.gcl),
        set `platforms` to `['$DISTRO_FAMILY']` (instead of
        `['debian-11']`). You should also add this section right after the
        `platforms =` line:

        ```gcl
        environment {
            // DO NOT MERGE: Temporarily disable smart test skipping for
            // this distro so that all apps actually run.
            SHORT = 'false'
        }
        ```

    *   In
        [test/ops_agent/bullseye.gcl](https://github.com/GoogleCloudPlatform/ops-agent/blob/master/kokoro/config/test/ops_agent/bullseye.gcl),
        set `platforms` to `['$DISTRO_FAMILY']`. This step is optional but
        if you don't do it, `ops_agent_test` will later fail and you'll have
        to remember to ignore those failures (since it will still be trying
        to run `ops_agent_test` on bullseye).

1.  Make a PR at this point to make Kokro run builds and tests. Problems
    with the new distro will show up as failures in the `third party apps
    integration test (Bullseye)`, since you repurposed `bullseye.gcl` in
    the previous step. At this point you can look at the test failures and
    determine how much work is needed to get them to pass. You may need to
    add your new distro to a few places in
    [third_party_apps_data/test_config.yaml](https://github.com/GoogleCloudPlatform/ops-agent/blob/master/integration_test/third_party_apps_data/test_config.yaml)
    to disable your new platform for certain problematic apps.

1.  Once `third party apps integration test (Bullseye)` (AKA
    `third_party_apps_test`) is passing, Revert the temporary changes to the
    Kokoro configs (the 2-3 `bullseye.gcl` files in the earlier step).
    Get your PR reviewed and merge it to `master`.
    
## Troubleshooting

Error: *failed to solve with frontend dockerfile.v0: failed to create LLB definition: target stage $DISTRO_SHORT could not be found*

Reason: The build didn't pick up your Dockerfile changes or there was a name
mismatch. There could be a few reasons for this:

*   You didn't run `compile.go`, or you made a typo for the target name.
*   The value of `DISTRO` in `kokoro/config/build/presubmit/$DISTRO_SHORT.gcl`
    doesn't match the `target_name` in `dockerfiles/compile.go`.
*   You haven't committed *and pushed* the `Dockerfile` change yet.

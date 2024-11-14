# Transformation Tests

The transformation tests are tests that will run Fluent Bit and OpenTelemetry Operations Collector (otelopscol) directly in your environment and verify the results of various logging operations by reading their output from stdout and stderr. This is to check the correctness of log parsing expectations in a more lightweight way than full integration tests.

## Makefile Usage

NOTE: It is assumed that you have followed the [makefile setup instructions](./makefile.md#usage).

For your convenience, there is a Makefile target to run transformation tests:
```bash
make transformation_test
```
You can also update the transformation test goldens:
```bash
make transformation_test_update
```
These targets are part of the `precommit` and `precommit_update` targets respectively.

If you are on a new branch that updates Fluent Bit or otelopscol and are locally verifying the transformation tests, you can force the agents to rebuild by deleting the `dist` folder before running `make transfomration_test`; the target will rebuild the agents if they are not present.

Read on if you are interested in how the transformation tests actually work.

## Usage

The transformation tests take paths to Fluent Bit and otelopscol binaries to execute directly and compare expected output. To run the tests, first you will need to build both binaries. Use the build scripts to build each agent and install them into the `dist` folder:
```bash
sh ./builds/fluent_bit.sh $(pwd)/dist
SKIP_OTEL_JAVA=true sh ./builds/otel.sh $(pwd)/dist
```
After running each script, you will have a `dist` folder that contains both agents within the full install directory. (Note that these instructions will not build OTel Java; if that is needed, do not pass `SKIP_OTEL_JAVA`).

The paths to each can passed into the test using environment variables or flags.
```bash
go test ./transformation_test \
-flb=${PWD}/dist/opt/google-cloud-ops-agent/subagents/fluent-bit/bin/fluent-bit \
-otelopscol=${PWD}/dist/opt/google-cloud-ops-agent/subagents/opentelemetry-collector/otelopscol
```
or
```bash
FLB=${PWD}/dist/opt/google-cloud-ops-agent/subagents/fluent-bit/bin/fluent-bit \
OTELOPSCOL=${PWD}/dist/opt/google-cloud-ops-agent/subagents/opentelemetry-collector/otelopscol \
go test ./transformation_test
```

If you have updated the fluent-bit or otelopscol submodules and need to update the golden testdata, you can add the `-update` flag to the test.
```bash
FLB=${PWD}/dist/opt/google-cloud-ops-agent/subagents/fluent-bit/bin/fluent-bit \
OTELOPSCOL=${PWD}/dist/opt/google-cloud-ops-agent/subagents/opentelemetry-collector/otelopscol \
go test ./transformation_test -update
```
(Note: the `-update` flag belongs to the test, not to `go test`. The flag must go after the specified package, not before.)

# Transformation Tests

The transformation tests are tests that will run Fluent Bit and OpenTelemetry Operations Collector (otelopscol) directly in your environment and test the results of various logging operations to stdout. This is to check the correctness of log parsing expectations in a more lightweight way than full integration tests.

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

Read on if you are interested in how the transformation tests actually work.

## Usage

The transformation tests take paths to Fluent Bit and otelopscol binaries to execute directly and compare expected output. To run the tests, first you will need to build both binaries. There are scripts to locally build each agent:
```bash
sh ./builds/fluent_bit_local.sh
sh ./builds/otel_local.sh
```
After running each script, you will have a `dist` folder that contains each binary.

The paths to each can passed into the test using environment variables or flags.
```bash
go test ./transformation_test -flb=$(pwd)/dist/fluent_bit -otelopscol=$(pwd)/dist/otelopscol
```
or
```bash
FLB=$(pwd)/dist/fluent_bit OTELOPSCOL=$(pwd)/dist/otelopscol go test ./transformation_test
```

If you have updated the Fluent Bit or otelopscol submodules and need to update the golden testdata, you can add the `-update` flag to the test.
```bash
FLB=$(pwd)/dist/fluent_bit OTELOPSCOL=$(pwd)/dist/otelopscol go test ./transformation_test -update
```
(Note: this is a flag to the test, not to `go test`. The flag must go after the specified package, not before.)
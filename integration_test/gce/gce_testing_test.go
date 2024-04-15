// Copyright 2023 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build integration_test

/*
	Tests for gce_testing.go.

Not comprehensive, since most features in gce_testing.go are tested indirectly
by various other tests like ops_agent_test.go. Features that require detailed,
specific testing should have tests here.

This test uses gce_testing.go, so it needs all of its required environment
variables to be defined, specifically:

- PROJECT
- ZONES

See gce_testing.go for documentation on what these do.
*/
package gce_test

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/rand"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/ops-agent/integration_test/agents"
	"github.com/GoogleCloudPlatform/ops-agent/integration_test/gce"
	"github.com/GoogleCloudPlatform/ops-agent/integration_test/logging"
)

// SetupLoggerAndVM sets up a VM for testing. This is very similar to
// agents.CommonSetup with some slight differences, like skipping
// RunOpsAgentDiagnostics().
func SetupLoggerAndVM(t *testing.T, platform string) (context.Context, *logging.DirectoryLogger, *gce.VM) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), gce.SuggestedTimeout)
	t.Cleanup(cancel)

	logger := gce.SetupLogger(t)
	logger.ToMainLog().Println("Calling SetupVM(). For details, see VM_initialization.txt.")
	vm := gce.SetupVM(ctx, t, logger.ToFile("VM_initialization.txt"), gce.VMOptions{ImageSpec: platform, MachineType: agents.RecommendedMachineType(platform)})
	logger.ToMainLog().Printf("VM is ready: %#v", vm)
	return ctx, logger, vm
}

// randomBytes returns a byte slice of the given size with randomly-generated
// contents.
func randomBytes(t *testing.T, size int) []byte {
	result := make([]byte, size)
	if _, err := rand.Read(result); err != nil {
		t.Fatal(err)
	}
	return result
}

type testCase struct {
	command string
	stdin   io.Reader

	fail bool

	// Regular expressions for checking whether stdout/stderr contain the
	// expected text.
	// An empty string means "don't check stdout" and likewise for stderr.
	stdoutRegexp string
	stderrRegexp string

	// An escape hatch to skip certain tests when run with RunScriptRemotely.
	// This is for two reasons:
	// 1. Some tests pass in data through stdin, which is (currently) not
	//    supported by RunScriptRemotely.
	// 2. To work around a bug with Powershell -File, which ignores some kinds
	//    of errors that it really shouldn't:
	//    "With normal termination, the exit code is always 0."
	//    https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.core/about/about_powershell_exe?view=powershell-5.1#-file----filepath-args
	//    Ideally RunScriptRemotely would work around this Powershell bug
	//    (which exists in pwsh too) and then the behavior of RunRemotely would
	//    match RunScriptRemotely, but I've so far been unable to do that.
	skipRunScriptRemotely bool
}

var powershellTestCases = []testCase{
	{
		command: "'1234'",
		fail:    false,
		// Expect this command to output "1234".
		stdoutRegexp: "1234",
	},
	{
		command: "dir /",
		fail:    false,
	},
	{
		// Expect an error, because dir is successfully run but finishes with
		// exit code 1.  This is because "The process exit code is determined
		// by status of the last (executed) command within the script block.
		// The exit code is 0 when $? is $true or 1 when $? is $false."
		// https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.core/about/about_powershell_exe?view=powershell-5.1#-command
		command:               "dir /nonexistent",
		fail:                  true,
		skipRunScriptRemotely: true,
	},
	{
		// Because nothing called asdfqwerty is installed.
		command:               "asdfqwerty help",
		fail:                  true,
		skipRunScriptRemotely: true,
	},
	{
		command:               "Get-Content -NoSuchFlag /",
		fail:                  true,
		skipRunScriptRemotely: true,
	},
	{
		command: "'ParseErrorMissingQuote",
		fail:    true,
	},
	{
		command:      "throw 'error_msg'",
		fail:         true,
		stderrRegexp: "error_msg",
	},
	{
		command:      "$Input | Write-Output",
		stdin:        strings.NewReader("5555"),
		fail:         false,
		stdoutRegexp: "5555",
		// Skip RunScriptRemotely because it doesn't support stdin.
		skipRunScriptRemotely: true,
	},

	// ================= Tests for multi-line commands follow. ===================

	{
		// Supposed to print "hello" and then run cd, which will set $?,
		// resulting in an exit code of 1 because it's the last command.
		command: `
Write-Output 'hello'
cd /nonexistent`,
		fail:                  true,
		stdoutRegexp:          "hello",
		skipRunScriptRemotely: true,
	},

	{
		// Supposed to exit immediately with code 1, without
		// proceeding to print "hello".
		command: `
exit 1
Write-Output 'hello'`,
		fail:         true,
		stdoutRegexp: "^$",
	},

	{
		// Supposed to fail to cd to a nonexistent directory, but proceed anyway.
		command: `
cd /nonexistent
Write-Output 'hello'`,
		fail:         false,
		stdoutRegexp: "hello",
	},
	{
		// Same thing, but with a Cmdlet failure instead of an external program failure.
		command: `
Get-Content -Path /nonexistent
Write-Output 'hello'`,
		fail:         false,
		stdoutRegexp: "hello",
	},
	{
		// Same thing, but with $ErrorActionPreference set to 'Stop'.
		// Supposed to fail and not print "hello" this time.
		command: `
$ErrorActionPreference = 'Stop'
Get-Content -Path /nonexistent
Write-Output 'hello'`,
		fail:         true,
		stdoutRegexp: "^$",
	},
}

func bashTestCases(t *testing.T) []testCase {
	return []testCase{
		{
			command: "echo 1234 && echo 5678 1>&2",
			fail:    false,
			// Expect this command to output "1234" to stdout and "5678" to stderr.
			stdoutRegexp: "1234",
			stderrRegexp: "5678",
		},
		{
			command: "ls /",
			fail:    false,
		},
		{
			command: "ls /nonexistent",
			fail:    true,
		},
		{
			// Because nothing called asdfqwerty is installed.
			command: "asdfqwerty help",
			fail:    true,
		},
		{
			command: "'parse_error_missing_quote",
			fail:    true,
		},
		{
			command: "exit 1",
			fail:    true,
		},
		{
			command:      "cat /dev/stdin",
			stdin:        strings.NewReader("5555"),
			fail:         false,
			stdoutRegexp: "5555",
			// Skip RunScriptRemotely because it doesn't support stdin.
			skipRunScriptRemotely: true,
		},
		{
			// Test a large stdin as a regression test for a bug where runCommand()
			// would hang if its stdin was too long.
			command:      "cat /dev/stdin | wc --bytes",
			stdin:        bytes.NewReader(randomBytes(t, 100_000_000)),
			fail:         false,
			stdoutRegexp: "100000000",
			// Skip RunScriptRemotely because it doesn't support stdin.
			skipRunScriptRemotely: true,
		},

		// A pair of tests for a detached process.
		// These two tests must be run consecutively.
		{
			command: `nohup bash -c "sleep 3 && echo 'done sleeping' > out.txt" &`,
			fail:    false,
		},
		{
			command:      "sleep 5 && cat out.txt",
			fail:         false,
			stdoutRegexp: "done sleeping",
		},

		// ================= Tests for multi-line commands follow. ===================

		{
			// Supposed to print "hello" and then run cd, which will set $?,
			// resulting in an exit code of 1 because it's the last command.
			command: `
echo hello
cd /nonexistent`,
			fail:         true,
			stdoutRegexp: "hello",
		},

		{
			// Supposed to exit immediately with code 1, without
			// proceeding to print "hello".
			command: `
exit 1
echo hello`,
			fail:         true,
			stdoutRegexp: "^$",
		},

		{
			// Supposed to fail to cd to a nonexistent directory, but proceed anyway.
			command: `
cd /nonexistent
echo hello`,
			fail:         false,
			stdoutRegexp: "hello",
		},
		{
			// Same thing, but with "set -e".
			// Supposed to fail and not print "hello" this time.
			command: `
set -e
cd /nonexistent
echo hello`,
			fail:         true,
			stdoutRegexp: "^$",
		},
	}
}

// testRunRemotelyHelper runs all the given test cases on the given VM, checking
// that RunRemotelyStdin and RunScriptRemotely report all expected errors and that
// standard out/error are as expected.
func testRunRemotelyHelper(ctx context.Context, t *testing.T, logger *log.Logger, vm *gce.VM, testCases []testCase) {
	runners := []struct {
		name   string
		runner func(io.Reader, string) (gce.CommandOutput, error)
	}{
		{
			name: "RunRemotelyStdin",
			runner: func(stdin io.Reader, command string) (gce.CommandOutput, error) {
				return gce.RunRemotelyStdin(ctx, logger, vm, stdin, command)
			},
		},
		{
			name: "RunScriptRemotely",
			runner: func(stdin io.Reader, command string) (gce.CommandOutput, error) {
				if stdin != nil {
					msg := "RunScriptRemotely doesn't support nonempty values of stdin."
					logger.Println(msg)
					t.Error(msg)
				}
				return gce.RunScriptRemotely(ctx, logger, vm, command, nil, nil)
			},
		},
	}

	for _, runnerCase := range runners {
		t.Run(runnerCase.name, func(t *testing.T) {
			logger.Printf("Starting test for %v", runnerCase.name)

			for _, tc := range testCases {
				if tc.skipRunScriptRemotely && runnerCase.name == "RunScriptRemotely" {
					logger.Printf("Skipping test for command %q due to skipRunScriptRemotely", tc.command)
					continue
				}
				output, err := runnerCase.runner(tc.stdin, tc.command)
				if tc.fail {
					if err == nil {
						t.Errorf("%q unexpectedly finished with no error (exit code 0)", tc.command)
					}
				} else {
					if err != nil {
						t.Error(err)
					}
				}

				// Define a helper to check tc.stdoutRegexp and tc.stderrRegexp against
				// output.Stdout and output.Stderr.
				regexChecker := func(output string, regularExpression string) {
					if regularExpression == "" {
						return
					}
					matched, err := regexp.MatchString(regularExpression, output)
					if err != nil {
						t.Errorf("Regexp parse failure: %v", err)
					} else if !matched {
						t.Errorf("output %q did not match expected regexp %q", output, regularExpression)
					}
				}
				regexChecker(output.Stdout, tc.stdoutRegexp)
				regexChecker(output.Stderr, tc.stderrRegexp)
			}
		})
	}
}

func TestRunRemotely(t *testing.T) {
	t.Parallel()
	gce.RunForEachPlatform(t, func(t *testing.T, platform string) {
		t.Parallel()

		ctx, logger, vm := SetupLoggerAndVM(t, platform)

		var cases []testCase
		if gce.IsWindows(platform) {
			cases = powershellTestCases
		} else {
			cases = bashTestCases(t)
		}

		testRunRemotelyHelper(ctx, t, logger.ToMainLog(), vm, cases)
	})
}

// eachByte returns a byte slice with each byte represented once in it.
func eachByte() []byte {
	result := make([]byte, 128)
	for i := 0; i < 128; i++ {
		result[i] = byte(i)
	}
	return result
}

// calculateRemoteMD5 computes the MD5 of the given file in a
// platform-specific way and returns the result as a lowercase hex string.
func calculateRemoteMD5(ctx context.Context, logger *log.Logger, vm *gce.VM, path string) (string, error) {
	if gce.IsWindows(vm.Platform) {
		output, err := gce.RunRemotely(ctx, logger, vm, fmt.Sprintf("(Get-FileHash -Algorithm MD5 -Path '%s').Hash", path))
		if err != nil {
			return "", err
		}
		return strings.ToLower(strings.TrimSpace(output.Stdout)), nil
	}

	output, err := gce.RunRemotely(ctx, logger, vm, fmt.Sprintf("set -o pipefail; md5sum '%s' | cut --field 1 --delimiter ' '", path))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output.Stdout), nil
}

func TestUploadContent(t *testing.T) {
	t.Parallel()
	gce.RunForEachPlatform(t, func(t *testing.T, platform string) {
		t.Parallel()

		ctx, logger, vm := SetupLoggerAndVM(t, platform)

		cases := [][]byte{
			[]byte("hello"),
			[]byte("goodbye\n"),
			[]byte(""),
			eachByte(),
			randomBytes(t, 100_000_000),
		}
		// Chosen to be platform agnostic, and as a bonus, requires sudo on Linux.
		path := "/test_upload_content"

		for _, data := range cases {
			if err := gce.UploadContent(ctx, logger.ToMainLog(), vm, bytes.NewReader(data), path); err != nil {
				t.Fatalf("Uploading %v bytes failed: %v", len(data), err)
			}

			expectedMD5 := fmt.Sprintf("%x", md5.Sum(data))
			actualMD5, err := calculateRemoteMD5(ctx, logger.ToMainLog(), vm, path)
			if err != nil {
				t.Fatal(err)
			}

			if expectedMD5 != actualMD5 {
				t.Errorf("got MD5 %q for file %v (size %v), want %q", actualMD5, path, len(data), expectedMD5)
				if len(data) < 1000 {
					// Use pwsh instead of powershell to get access to "-AsByteStream".
					output, err := gce.RunRemotely(ctx, logger.ToMainLog(), vm, fmt.Sprintf(`pwsh -Command "Get-Content -AsByteStream '%s'"`, path))
					if err != nil {
						t.Fatal(err)
					}
					t.Errorf("size %v file contents (as bytes): %v", len(data), output.Stdout)
				}
			}
		}
	})
}

func TestMain(m *testing.M) {
	code := m.Run()
	gce.CleanupKeysOrDie()
	os.Exit(code)
}

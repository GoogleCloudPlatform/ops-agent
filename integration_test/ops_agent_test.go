//go:build integration_test

/*
Test for core functionality of the Ops Agent.
Can be run with Kokoro or "go test". For instructions, see the top of
gce_testing.go.

This test needs the following environment variables to be defined, in addition
to the ones mentioned at the top of gce_testing.go:

PLATFORMS: a comma-separated list of distros to test, e.g. "centos-7,centos-8".

The following variables are optional:

REPO_SUFFIX: If provided, what Rapture repo suffix to install the ops agent from.
AGENT_PACKAGES_IN_GCS: If provided, a URL for a directory in GCS containing
    .deb/.rpm/.goo files to install on the testing VMs. Each agent in
		AGENTS_TO_TEST must have its own subdirectory, so for example this would be
		a valid structure inside AGENT_PACKAGES_IN_GCS if
		AGENTS_TO_TEST=metrics,ops_agent:
	├── metrics
    │   ├── collectd-4.5.6.deb
    │   ├── collectd-4.5.6.rpm
    │   └── otel-collector-0.1.2.goo
    └── ops-agent
        ├── ops-agent-google-cloud-1.2.3.deb
        ├── ops-agent-google-cloud-1.2.3.rpm
        └── ops-agent-google-cloud-1.2.3.goo
*/
package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/ops-agent/integration_test/agents"
	"github.com/GoogleCloudPlatform/ops-agent/integration_test/gce"
	"github.com/GoogleCloudPlatform/ops-agent/integration_test/logging"

	"go.uber.org/multierr"
	"google.golang.org/protobuf/proto"
	structpb "google.golang.org/protobuf/types/known/structpb"
)

var (
	packagesInGCS = os.Getenv("AGENT_PACKAGES_IN_GCS")
)

func logPathForPlatform(platform string) string {
	if gce.IsWindows(platform) {
		return "C:/mylog"
	}
	return "/tmp/mylog"
}

func configPathForPlatform(platform string) string {
	if gce.IsWindows(platform) {
		return `C:\Program Files\Google\Cloud Operations\Ops Agent\config\config.yaml`
	}
	return "/etc/google-cloud-ops-agent/config.yaml"
}

func restartCommandForPlatform(platform string) string {
	if gce.IsWindows(platform) {
		return "Restart-Service google-cloud-ops-agent -Force"
	}
	// Return a command that works for both < 2.0.0 and >= 2.0.0 agents.
	return "sudo service google-cloud-ops-agent restart || sudo systemctl restart google-cloud-ops-agent.target"
}

func systemLogTagForPlatform(platform string) string {
	if gce.IsWindows(platform) {
		return "windows_event_log"
	}
	return "syslog"
}

func logMessageQueryForPlatform(platform, message string) string {
	if gce.IsWindows(platform) {
		// On Windows, we have to look in the capital-M Message field. On linux
		// it is lowercase-m message.
		// Also, Fluent-Bit adds an extra newline to our payload, so we need to
		// use ":" instead of "=" here.
		return "jsonPayload.Message:" + message
	}
	return "jsonPayload.message=" + message
}

func metricsAgentProcessNamesForPlatform(platform string) []string {
	if gce.IsWindows(platform) {
		return []string{"google-cloud-metrics-agent_windows_amd64"}
	}
	return []string{"otelopscol", "collectd"}
}

func writeToWindowsEventLog(ctx context.Context, logger *log.Logger, vm *gce.VM, logName, payload string) error {
	// If this is the first time we're trying to write to logName, we need to
	// register a fake log source with New-EventLog.
	// There's a problem:  there's no way (that I can find) to check whether a
	// particular log source is registered to write to logName: the closest I
	// can get is checking whether a log source is registered to write
	// *somewhere*. So the workaround is to make the log source's name unique
	// per logName.
	source := logName + "__ops_agent_test"
	if _, err := gce.RunRemotely(ctx, logger, vm, "", fmt.Sprintf("if(![System.Diagnostics.EventLog]::SourceExists('%s')) { New-EventLog –LogName '%s' –Source '%s' }", source, logName, source)); err != nil {
		return fmt.Errorf("writeToWindowsEventLog(logName=%q, payload=%q) failed to register new source %v: %v", logName, payload, source, err)
	}

	// Use a Powershell here-string to avoid most quoting issues.
	if _, err := gce.RunRemotely(ctx, logger, vm, "", fmt.Sprintf("Write-EventLog -LogName '%s' -EventId 1 -Source '%s' -Message @'\n%s\n'@", logName, source, payload)); err != nil {
		return fmt.Errorf("writeToWindowsEventLog(logName=%q, payload=%q) failed: %v", logName, payload, err)
	}
	return nil
}

// writeToSystemLog writes the given payload to the VM's normal log location.
// On Linux this is /var/log/syslog or /var/log/messages, depending on the
// distro.
// On Windows this is the "System" log within the event logging system.
func writeToSystemLog(ctx context.Context, logger *log.Logger, vm *gce.VM, payload string) error {
	if gce.IsWindows(vm.Platform) {
		return writeToWindowsEventLog(ctx, logger, vm, "System", payload)
	}
	line := payload + "\n"
	location := gce.SyslogLocation(vm.Platform)
	// Pass the content in on stdin and run "cat -" to tell cat to copy from stdin.
	// This is to avoid having to quote the content correctly for the shell.
	// "tee -a" will append to the file.
	if _, err := gce.RunRemotely(ctx, logger, vm, line, fmt.Sprintf("cat - | sudo tee -a '%s' > /dev/null", location)); err != nil {
		return fmt.Errorf("writeToSystemLog() failed to write %q to %s: %v", line, location, err)
	}
	return nil
}

func installOpsAgent(ctx context.Context, logger *logging.DirectoryLogger, vm *gce.VM) error {
	if packagesInGCS != "" {
		return agents.InstallPackageFromGCS(ctx, logger, vm, agents.OpsAgentType, packagesInGCS)
	}
	if gce.IsWindows(vm.Platform) {
		suffix := os.Getenv("REPO_SUFFIX")
		if suffix == "" {
			suffix = "all"
		}
		runGoogetInstall := func() error {
			_, err := gce.RunRemotely(ctx, logger.ToMainLog(), vm, "", fmt.Sprintf("googet -noconfirm install -sources https://packages.cloud.google.com/yuck/repos/google-cloud-ops-agent-windows-%s google-cloud-ops-agent", suffix))
			return err
		}
		if err := agents.RunInstallFuncWithRetry(ctx, logger.ToMainLog(), vm, runGoogetInstall); err != nil {
			return fmt.Errorf("installOpsAgent() failed to run googet: %v", err)
		}
		return nil
	}

	if _, err := gce.RunRemotely(ctx,
		logger.ToMainLog(), vm, "", "curl -sSO https://dl.google.com/cloudagents/add-google-cloud-ops-agent-repo.sh"); err != nil {
		return fmt.Errorf("installOpsAgent() failed to download repo script: %v", err)
	}

	runInstallScript := func() error {
		_, err := gce.RunRemotely(ctx, logger.ToMainLog(), vm, "", "sudo REPO_SUFFIX="+os.Getenv("REPO_SUFFIX")+" bash -x add-google-cloud-ops-agent-repo.sh --also-install")
		return err
	}
	if err := agents.RunInstallFuncWithRetry(ctx, logger.ToMainLog(), vm, runInstallScript); err != nil {
		return fmt.Errorf("installOpsAgent() error running repo script: %v", err)
	}
	return nil
}

func setupOpsAgent(ctx context.Context, logger *logging.DirectoryLogger, vm *gce.VM, config string) error {
	if err := installOpsAgent(ctx, logger, vm); err != nil {
		return err
	}
	startupDelay := 20 * time.Second
	if len(config) > 0 {
		if gce.IsWindows(vm.Platform) {
			// Sleep to avoid some flaky errors when restarting the agent because the
			// services have not fully started up yet.
			time.Sleep(startupDelay)
		}
		if err := gce.UploadContent(ctx, logger, vm, strings.NewReader(config), configPathForPlatform(vm.Platform)); err != nil {
			return fmt.Errorf("setupOpsAgent() failed to upload config file: %v", err)
		}
		if _, err := gce.RunRemotely(ctx, logger.ToMainLog(), vm, "", restartCommandForPlatform(vm.Platform)); err != nil {
			return fmt.Errorf("setupOpsAgent() failed to restart ops agent: %v", err)
		}
		// Give agents time to shut down. Fluent-Bit's default shutdown grace period
		// is 5 seconds, so we should probably give it at least that long.
		time.Sleep(10 * time.Second)
	}
	// Give agents time to start up.
	time.Sleep(startupDelay)
	return nil
}

func TestCustomLogFile(t *testing.T) {
	t.Parallel()
	gce.RunForEachPlatform(t, func(t *testing.T, platform string) {
		t.Parallel()
		ctx, logger, vm := agents.CommonSetup(t, platform)
		logPath := logPathForPlatform(vm.Platform)
		config := fmt.Sprintf(`logging:
  receivers:
    mylog_source:
      type: files
      include_paths:
      - %s
  exporters:
    google:
      type: google_cloud_logging
  processors:
    my_exclude:
      type: exclude_logs
      match_any:
      - jsonPayload.message =~ "test pattern"
  service:
    pipelines:
      my_pipeline:
        receivers: [mylog_source]
        processors: [my_exclude]
        exporters: [google]
`, logPath)

		if err := setupOpsAgent(ctx, logger, vm, config); err != nil {
			t.Fatal(err)
		}

		if err := gce.UploadContent(ctx, logger, vm, strings.NewReader("abc test pattern xyz\n7654321\n"), logPath); err != nil {
			t.Fatalf("error writing dummy log line: %v", err)
		}

		if err := gce.WaitForLog(ctx, logger.ToMainLog(), vm, "mylog_source", time.Hour, "jsonPayload.message=7654321"); err != nil {
			t.Error(err)
		}
		time.Sleep(60 * time.Second)
		_, err := gce.QueryLog(ctx, logger.ToMainLog(), vm, "mylog_source", time.Hour, `jsonPayload.message="abc test pattern xyz"`, 5)
		if err == nil {
			t.Error("expected log to be excluded but was included")
		} else if !strings.Contains(err.Error(), "not found, exhausted retries") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestCustomLogFormat(t *testing.T) {
	t.Parallel()
	gce.RunForEachPlatform(t, func(t *testing.T, platform string) {
		t.Parallel()
		ctx, logger, vm := agents.CommonSetup(t, platform)

		logPath := logPathForPlatform(vm.Platform)
		config := fmt.Sprintf(`logging:
  receivers:
    mylog_source:
      type: files
      include_paths:
      - %s
  exporters:
    google:
      type: google_cloud_logging
  processors:
    rfc5424:
      type: parse_regex
      regex: ^\<(?<pri>[0-9]{1,5})\>1 (?<time>[^ ]+) (?<host>[^ ]+) (?<ident>[^ ]+) (?<pid>[-0-9]+) (?<msgid>[^ ]+) (?<extradata>(\[(.*?)\]|-)) (?<message>.+)$
      time_key: time
      time_format: "%s"
  service:
    pipelines:
      my_pipeline:
        receivers: [mylog_source]
        processors: [rfc5424]
        exporters: [google]
`, logPath, "%Y-%m-%dT%H:%M:%S.%L%z")

		if err := setupOpsAgent(ctx, logger, vm, config); err != nil {
			t.Fatal(err)
		}

		// When not using UTC timestamps, the parsing with "%Y-%m-%dT%H:%M:%S.%L%z" doesn't work
		// correctly in windows (b/218888265).
		line := fmt.Sprintf("<13>1 %s %s my_app_id - - - qqqqrrrr\n", time.Now().UTC().Format(time.RFC3339Nano), vm.Name)
		if err := gce.UploadContent(ctx, logger, vm, strings.NewReader(line), logPath); err != nil {
			t.Fatalf("error writing dummy log line: %v", err)
		}

		if err := gce.WaitForLog(ctx, logger.ToMainLog(), vm, "mylog_source", time.Hour, "jsonPayload.message=qqqqrrrr AND jsonPayload.ident=my_app_id"); err != nil {
			t.Error(err)
		}
	})
}

func TestInvalidConfig(t *testing.T) {
	t.Parallel()
	gce.RunForEachPlatform(t, func(t *testing.T, platform string) {
		t.Parallel()
		ctx, logger, vm := agents.CommonSetup(t, platform)

		// Sample bad config sourced from:
		// https://github.com/GoogleCloudPlatform/ops-agent/blob/master/confgenerator/testdata/invalid/linux/logging-receiver_reserved_id_prefix/input.yaml
		config := `logging:
  receivers:
    lib:receiver_1:
      type: files
      include_paths:
      - /var/log/user-log
  service:
    pipelines:
      default_pipeline:
        receivers: [lib:receiver_1]
`

		// Run install with an invalid config. We expect to see an error.
		if err := setupOpsAgent(ctx, logger, vm, config); err == nil {
			t.Fatal("Expected agent to reject bad config.")
		}
	})
}

func TestProcessorOrder(t *testing.T) {
	// See b/194632049 and b/195105380.  In that bug, the generated Fluent Bit
	// config had mis-ordered filters: json2 came before json1 because "log"
	// sorts before "message".  The correct order is json1 then json2.
	//
	// Due to the bug, the log contents came through as a string, not as
	// parsed JSON.
	t.Parallel()
	gce.RunForEachPlatform(t, func(t *testing.T, platform string) {
		t.Parallel()
		ctx, logger, vm := agents.CommonSetup(t, platform)

		logPath := logPathForPlatform(vm.Platform)
		config := fmt.Sprintf(`logging:
  receivers:
    mylog_source:
      type: files
      include_paths:
      - %s
  exporters:
    google:
      type: google_cloud_logging
  processors:
    json1:
      type: parse_json
      field: message
      time_key: time
      time_format: "%s"
    json2:
      type: parse_json
      field: log
  service:
    pipelines:
      my_pipeline:
        receivers: [mylog_source]
        processors: [json1, json2]
        exporters: [google]
`, logPath, "%Y-%m-%dT%H:%M:%S.%L%z")

		if err := setupOpsAgent(ctx, logger, vm, config); err != nil {
			t.Fatal(err)
		}

		// When not using UTC timestamps, the parsing with "%Y-%m-%dT%H:%M:%S.%L%z" doesn't work
		// correctly in windows (b/218888265).
		line := fmt.Sprintf(`{"log":"{\"level\":\"info\",\"message\":\"start\"}\n","time":"%s"}`, time.Now().UTC().Format(time.RFC3339Nano)) + "\n"
		if err := gce.UploadContent(ctx, logger, vm, strings.NewReader(line), logPath); err != nil {
			t.Fatalf("error writing dummy log line: %v", err)
		}

		entry, err := gce.QueryLog(ctx, logger.ToMainLog(), vm, "mylog_source", time.Hour, "", gce.QueryMaxAttempts)
		if err != nil {
			t.Fatal(err)
		}

		want := &structpb.Struct{Fields: map[string]*structpb.Value{
			"level":   {Kind: &structpb.Value_StringValue{StringValue: "info"}},
			"message": {Kind: &structpb.Value_StringValue{StringValue: "start"}},
		}}

		got, ok := entry.Payload.(proto.Message)
		if !ok {
			t.Fatalf("got %+v of type %T, want type proto.Message", entry.Payload, entry.Payload)
		}
		if !proto.Equal(got, want) {
			t.Errorf("got %+v, want %+v", got, want)
		}
	})
}

func TestSyslogTCP(t *testing.T) {
	t.Parallel()
	gce.RunForEachPlatform(t, func(t *testing.T, platform string) {
		t.Parallel()
		if gce.IsWindows(platform) {
			t.SkipNow()
		}
		ctx, logger, vm := agents.CommonSetup(t, platform)

		config := `logging:
  receivers:
    mylog_source:
      type: syslog
      transport_protocol: tcp
      listen_host: 0.0.0.0
      listen_port: 5140
  processors:
    my_exclude:
      type: exclude_logs
      match_any:
      - jsonPayload.message =~ "test pattern"
  exporters:
    google:
      type: google_cloud_logging
  service:
    pipelines:
      my_pipeline:
        receivers: [mylog_source]
        processors: [my_exclude]
        exporters: [google]
`

		if err := setupOpsAgent(ctx, logger, vm, config); err != nil {
			t.Fatal(err)
		}

		// By writing the inclusion message last, we increase the likelihood that once the inclusion message is detected, that the
		// exclusion message would have already made it through the backend -- in other words, this increases the likelihood of
		// detecting a failure if the exclusion message were to actually be included.

		// Write test message for exclusion using the program called logger.
		if _, err := gce.RunRemotely(ctx, logger.ToMainLog(), vm, "", "logger -n 0.0.0.0 --tcp --port=5140 -- abc test pattern xyz"); err != nil {
			t.Fatalf("Error writing dummy log line: %v", err)
		}
		// Write test message for inclusion.
		if _, err := gce.RunRemotely(ctx, logger.ToMainLog(), vm, "", "logger -n 0.0.0.0 --tcp --port=5140 -- abcdefg"); err != nil {
			t.Fatalf("Error writing dummy log line: %v", err)
		}

		if err := gce.WaitForLog(ctx, logger.ToMainLog(), vm, "mylog_source", time.Hour, "jsonPayload.message:abcdefg"); err != nil {
			t.Error(err)
		}
		time.Sleep(60 * time.Second)
		_, err := gce.QueryLog(ctx, logger.ToMainLog(), vm, "mylog_source", time.Hour, `jsonPayload.message:"test pattern"`, 5)
		if err == nil {
			t.Error("expected log to be excluded but was included")
		} else if !strings.Contains(err.Error(), "not found, exhausted retries") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestSyslogUDP(t *testing.T) {
	t.Parallel()
	gce.RunForEachPlatform(t, func(t *testing.T, platform string) {
		t.Parallel()
		if gce.IsWindows(platform) {
			t.SkipNow()
		}
		ctx, logger, vm := agents.CommonSetup(t, platform)

		config := `logging:
  receivers:
    mylog_source:
      type: syslog
      transport_protocol: udp
      listen_host: 0.0.0.0
      listen_port: 5140
  exporters:
    google:
      type: google_cloud_logging
  service:
    pipelines:
      my_pipeline:
        receivers: [mylog_source]
        exporters: [google]
`

		if err := setupOpsAgent(ctx, logger, vm, config); err != nil {
			t.Fatal(err)
		}

		// Write "abcdefg" using the program called logger.
		if _, err := gce.RunRemotely(ctx, logger.ToMainLog(), vm, "", "logger -n 0.0.0.0 --udp --port=5140 -- abcdefg"); err != nil {
			t.Fatalf("Error writing dummy log line: %v", err)
		}

		if err := gce.WaitForLog(ctx, logger.ToMainLog(), vm, "mylog_source", time.Hour, "jsonPayload.message:abcdefg"); err != nil {
			t.Error(err)
		}
	})
}

func TestExcludeLogsParseJsonOrder(t *testing.T) {
	t.Parallel()
	gce.RunForEachPlatform(t, func(t *testing.T, platform string) {
		t.Parallel()
		ctx, logger, vm := agents.CommonSetup(t, platform)
		file1 := fmt.Sprintf("%s_1", logPathForPlatform(vm.Platform))
		file2 := fmt.Sprintf("%s_2", logPathForPlatform(vm.Platform))

		// This exclude_logs processor operates on a non-default field which is
		// present if and only if the log is structured accordingly.
		// The intended mechanism for inputting structured logs from a file is to
		// use a parse_json processor. Since processors operate in the order in
		// which they're written, the expectation is that if a parse_json processor
		// comes before the exclude_logs processor then the log is
		// excluded. (pipeline p1)
		// Conversely, if a parse_json processor comes after the exclude_logs
		// processor then the log is not excluded: the log inputted to exclude_logs
		// is unstructured, and unstructured logs do not contain non-default
		// fields, so it cannot be matched by the match_any expression
		// below. (pipeline p2)
		config := fmt.Sprintf(`logging:
  receivers:
    f1:
      type: files
      include_paths:
      - %s
    f2:
      type: files
      include_paths:
      - %s
  processors:
    exclude:
      type: exclude_logs
      match_any:
      - jsonPayload.field =~ "value"
    json:
      type: parse_json
  exporters:
    google:
      type: google_cloud_logging
  service:
    pipelines:
      p1:
        receivers: [f1]
        processors: [exclude, json]
        exporters: [google]
      p2:
        receivers: [f2]
        processors: [json, exclude]
        exporters: [google]
`, file1, file2)

		if err := setupOpsAgent(ctx, logger, vm, config); err != nil {
			t.Fatal(err)
		}

		line := `{"field":"value"}` + "\n"
		if err := gce.UploadContent(ctx, logger, vm, strings.NewReader(line), file2); err != nil {
			t.Fatalf("error uploading log: %v", err)
		}
		if err := gce.UploadContent(ctx, logger, vm, strings.NewReader(line), file1); err != nil {
			t.Fatalf("error uploading log: %v", err)
		}

		// Expect to see the log included in p1 but not p2.
		if err := gce.WaitForLog(ctx, logger.ToMainLog(), vm, "f1", time.Hour, `jsonPayload.field="value"`); err != nil {
			t.Error(err)
		}
		// Give the excluded log some time to show up.
		time.Sleep(60 * time.Second)
		_, err := gce.QueryLog(ctx, logger.ToMainLog(), vm, "f2", time.Hour, `jsonPayload.field="value"`, 5)
		if err == nil {
			t.Error("expected log to be excluded but was included")
		} else if !strings.Contains(err.Error(), "not found, exhausted retries") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestTCPLog(t *testing.T) {
	t.Parallel()
	gce.RunForEachPlatform(t, func(t *testing.T, platform string) {
		t.Parallel()
		if gce.IsWindows(platform) {
			t.SkipNow()
		}
		ctx, logger, vm := agents.CommonSetup(t, platform)

		config := `logging:
  receivers:
    tcp_logs:
      type: tcp
      format: json
      listen_host: 0.0.0.0
      listen_port: 5170
  service:
    pipelines:
      tcp_pipeline:
        receivers: [tcp_logs]
`

		if err := setupOpsAgent(ctx, logger, vm, config); err != nil {
			t.Fatal(err)
		}

		// Write JSON test log to TCP socket via bash redirect, to get around installing and using netcat.
		// https://www.gnu.org/savannah-checkouts/gnu/bash/manual/bash.html#Redirections
		if _, err := gce.RunRemotely(ctx, logger.ToMainLog(), vm, "", "echo '{\"msg\":\"test tcp log 1\"}{\"msg\":\"test tcp log 2\"}' > /dev/tcp/localhost/5170"); err != nil {
			t.Fatalf("Error writing dummy TCP log line: %v", err)
		}

		if err := gce.WaitForLog(ctx, logger.ToMainLog(), vm, "tcp_logs", time.Hour, "jsonPayload.msg:test tcp log 1"); err != nil {
			t.Error(err)
		}

		if err := gce.WaitForLog(ctx, logger.ToMainLog(), vm, "tcp_logs", time.Hour, "jsonPayload.msg:test tcp log 2"); err != nil {
			t.Error(err)
		}
	})
}

func TestFluentForwardLog(t *testing.T) {
	t.Parallel()
	gce.RunForEachPlatform(t, func(t *testing.T, platform string) {
		t.Parallel()
		if gce.IsWindows(platform) {
			t.SkipNow()
		}
		ctx, logger, vm := agents.CommonSetup(t, platform)

		config := `logging:
  receivers:
    fluent_logs:
      type: fluent_forward
      listen_host: 127.0.0.1
      listen_port: 24224
  service:
    pipelines:
      fluent_pipeline:
        receivers: [fluent_logs]
`
		if err := setupOpsAgent(ctx, logger, vm, config); err != nil {
			t.Fatal(err)
		}

		// Use another instance of Fluent Bit to read from stdin and  forward to the
		// Ops Agent.
		//
		// The forwarding Fluent Bit uses the tag "forwarder_tag" when sending the
		// log record. This will be preserved in the LogName.
		if _, err := gce.RunRemotely(ctx, logger.ToMainLog(), vm, "", "echo '{\"msg\":\"test fluent forward log\"}' | /opt/google-cloud-ops-agent/subagents/fluent-bit/bin/fluent-bit -i stdin -o forward://127.0.0.1:24224 -t forwarder_tag"); err != nil {
			t.Fatalf("Error writing dummy forward protocol log line: %v", err)
		}

		if err := gce.WaitForLog(ctx, logger.ToMainLog(), vm, "fluent_logs.forwarder_tag", time.Hour, "jsonPayload.msg:test fluent forward log"); err != nil {
			t.Error(err)
		}
	})
}

func TestWindowsEventLog(t *testing.T) {
	t.Parallel()
	gce.RunForEachPlatform(t, func(t *testing.T, platform string) {
		t.Parallel()
		if !gce.IsWindows(platform) {
			t.SkipNow()
		}
		ctx, logger, vm := agents.CommonSetup(t, platform)

		config := `logging:
  receivers:
    windows_event_log:
      type: windows_event_log
      channels: [Application,System]
  exporters:
    google:
      type: google_cloud_logging
  service:
    pipelines:
      default_pipeline:
        receivers: [windows_event_log]
        exporters: [google]
`
		if err := setupOpsAgent(ctx, logger, vm, config); err != nil {
			t.Fatal(err)
		}

		payloads := map[string]string{
			"Application": "application_msg",
			"System":      "system_msg",
		}
		for log, payload := range payloads {
			if err := writeToWindowsEventLog(ctx, logger.ToMainLog(), vm, log, payload); err != nil {
				t.Fatal(err)
			}
		}

		for _, payload := range payloads {
			if err := gce.WaitForLog(ctx, logger.ToMainLog(), vm, "windows_event_log", time.Hour, logMessageQueryForPlatform(vm.Platform, payload)); err != nil {
				t.Fatal(err)
			}
		}
	})
}

func TestSystemdLog(t *testing.T) {
	t.Parallel()
	gce.RunForEachPlatform(t, func(t *testing.T, platform string) {
		t.Parallel()
		if gce.IsWindows(platform) {
			t.SkipNow()
		}
		ctx, logger, vm := agents.CommonSetup(t, platform)

		config := `logging:
  receivers:
    systemd_logs:
      type: systemd_journald
  service:
    pipelines:
      systemd_pipeline:
        receivers: [systemd_logs]
`

		if err := setupOpsAgent(ctx, logger, vm, config); err != nil {
			t.Fatal(err)
		}

		if _, err := gce.RunRemotely(ctx, logger.ToMainLog(), vm, "", "echo 'my_systemd_log_message' | systemd-cat"); err != nil {
			t.Fatalf("Error writing dummy Systemd log line: %v", err)
		}

		if err := gce.WaitForLog(ctx, logger.ToMainLog(), vm, "systemd_logs", time.Hour, "my_systemd_log_message"); err != nil {
			t.Error(err)
		}
	})
}

func TestSystemLogByDefault(t *testing.T) {
	t.Parallel()
	gce.RunForEachPlatform(t, func(t *testing.T, platform string) {
		t.Parallel()
		ctx, logger, vm := agents.CommonSetup(t, platform)

		if err := setupOpsAgent(ctx, logger, vm, ""); err != nil {
			t.Fatal(err)
		}

		if err := writeToSystemLog(ctx, logger.ToMainLog(), vm, "123456789"); err != nil {
			t.Fatal(err)
		}

		tag := systemLogTagForPlatform(vm.Platform)
		if err := gce.WaitForLog(ctx, logger.ToMainLog(), vm, tag, time.Hour, logMessageQueryForPlatform(vm.Platform, "123456789")); err != nil {
			t.Error(err)
		}
	})
}

// agentVersionRegexesForPlatform returns a slice containing all agents that
// we expect to upload an uptime metric on the given platform. This function
// returns a list of regexes that we can match against the "version" field of the
// uptime metric.
func agentVersionRegexesForPlatform(platform string) []string {
	// TODO(b/191363559): Remove this whole `gce.IsWindows(platform)` section after we
	// release a stable Ops Agent version that changes user agent to
	// google-cloud-ops-agent-metrics/ for Windows as well.
	if gce.IsWindows(platform) {
		return []string{
			// TODO(b/176832677): The Logging agent does not currently upload an uptime metric.
			// TODO(b/191362867): Update the regex to be more strict after a version of
			// Ops Agent has been released with the expected user agent.
			// strings.Join([]string{
			// 	 "google-cloud-ops-agent-metrics/[0-9]+[.][0-9]+[.][0-9]+.*-.+",
			// 	 "Google Cloud Metrics Agent/.*",
			// }, "|"),
			strings.Join([]string{
				"google-cloud-ops-agent-metrics/[0-9]+[.][0-9]+[.][0-9].*",
				"Google Cloud Metrics Agent/.*",
			}, "|"),
		}
	}
	return []string{
		// TODO(jschulz): Enable this label once it exists.
		//"google-cloud-ops-agent-engine/",
		// TODO(b/170138116): Enable this label once it is being collected.
		//"google-cloud-ops-agent-logs/",
		// TODO(b/191362867): Update the regex to be more strict after a version of
		// Ops Agent has been released with the expected user agent.
		// "google-cloud-ops-agent-metrics/[0-9]+[.][0-9]+[.][0-9]+.*-.+",
		"google-cloud-ops-agent-metrics/[0-9]+[.][0-9]+[.][0-9]+.*",
	}
}

func metricsForPlatform(platform string) []string {
	commonMetrics := []string{
		"agent.googleapis.com/agent/api_request_count",
		"agent.googleapis.com/agent/memory_usage",
		"agent.googleapis.com/agent/monitoring/point_count",

		// TODO(b/170138116): Enable these metrics once they are being collected.
		//"agent.googleapis.com/agent/log_entry_count",
		//"agent.googleapis.com/agent/log_entry_retry_count",
		//"agent.googleapis.com/agent/request_count",

		"agent.googleapis.com/cpu/load_1m",
		"agent.googleapis.com/cpu/load_5m",
		"agent.googleapis.com/cpu/load_15m",
		"agent.googleapis.com/cpu/utilization",

		"agent.googleapis.com/disk/bytes_used",
		"agent.googleapis.com/disk/io_time",
		"agent.googleapis.com/disk/operation_count",
		"agent.googleapis.com/disk/operation_time",
		"agent.googleapis.com/disk/percent_used",
		"agent.googleapis.com/disk/read_bytes_count",
		"agent.googleapis.com/disk/write_bytes_count",

		"agent.googleapis.com/interface/errors",
		"agent.googleapis.com/interface/packets",
		"agent.googleapis.com/interface/traffic",

		"agent.googleapis.com/memory/bytes_used",
		"agent.googleapis.com/memory/percent_used",

		"agent.googleapis.com/network/tcp_connections",

		"agent.googleapis.com/processes/cpu_time",
		"agent.googleapis.com/processes/disk/read_bytes_count",
		"agent.googleapis.com/processes/disk/write_bytes_count",
		"agent.googleapis.com/processes/rss_usage",
		"agent.googleapis.com/processes/vm_usage",

		"agent.googleapis.com/swap/bytes_used",
		"agent.googleapis.com/swap/io",
	}
	if gce.IsWindows(platform) {
		windowsOnlyMetrics := []string{
			"agent.googleapis.com/pagefile/percent_used",
		}
		return append(commonMetrics, windowsOnlyMetrics...)
	}

	linuxOnlyMetrics := []string{
		"agent.googleapis.com/disk/merged_operations",
		"agent.googleapis.com/processes/count_by_state",
	}
	return append(commonMetrics, linuxOnlyMetrics...)
}

func testDefaultMetrics(ctx context.Context, t *testing.T, logger *logging.DirectoryLogger, vm *gce.VM, window time.Duration) {
	if !gce.IsWindows(vm.Platform) {
		// Enable swap file: https://linuxize.com/post/create-a-linux-swap-file/
		// We do this so that swap file metrics will show up.
		_, err := gce.RunRemotely(ctx, logger.ToMainLog(), vm, "", strings.Join([]string{
			"sudo dd if=/dev/zero of=/swapfile bs=1024 count=102400",
			"sudo chmod 600 /swapfile",
			"sudo mkswap /swapfile",
			"sudo swapon /swapfile",
		}, " && "))
		if err != nil {
			t.Fatalf("Failed to enable swap file: %v", err)
		}
	}

	// First make sure that the uptime metrics are being uploaded.
	var uptimeWaitGroup sync.WaitGroup
	regexes := agentVersionRegexesForPlatform(vm.Platform)
	for _, versionRegex := range regexes {
		versionRegex := versionRegex
		uptimeWaitGroup.Add(1)
		go func() {
			defer uptimeWaitGroup.Done()
			if err := gce.WaitForMetric(ctx, logger.ToMainLog(), vm, "agent.googleapis.com/agent/uptime", window,
				[]string{fmt.Sprintf("metric.labels.version = monitoring.regex.full_match(%q)", versionRegex)},
			); err != nil {
				t.Error(err)
			}
		}()
	}
	uptimeWaitGroup.Wait()

	if t.Failed() {
		// Return early instead of waiting up to 7 minutes for the second round
		// of querying for metrics.
		return
	}

	// Now that we've established that the preceding metrics are being uploaded
	// and have percolated through the monitoring backend, let's proceed to
	// query for the rest of the metrics. We used to query for all the metrics
	// at once, but due to the "no metrics yet" retries, this ran us out of
	// quota (b/185363780).
	var metricsWaitGroup sync.WaitGroup
	for _, metric := range metricsForPlatform(vm.Platform) {
		metric := metric
		metricsWaitGroup.Add(1)
		go func() {
			defer metricsWaitGroup.Done()
			if err := gce.WaitForMetric(ctx, logger.ToMainLog(), vm, metric, window, nil); err != nil {
				t.Error(err)
			}
		}()
	}
	metricsWaitGroup.Wait()
}

func TestDefaultMetricsNoProxy(t *testing.T) {
	t.Parallel()
	gce.RunForEachPlatform(t, func(t *testing.T, platform string) {
		t.Parallel()
		ctx, logger, vm := agents.CommonSetup(t, platform)
		if err := setupOpsAgent(ctx, logger, vm, ""); err != nil {
			t.Fatal(err)
		}

		testDefaultMetrics(ctx, t, logger, vm, time.Hour)
	})
}

// go/sdi-integ-test#proxy-testing
func TestDefaultMetricsWithProxy(t *testing.T) {
	t.Parallel()
	gce.RunForEachPlatform(t, func(t *testing.T, platform string) {
		t.Parallel()
		if !gce.IsWindows(platform) {
			t.Skip("Proxy test is currently only supported on windows.")
		}
		proxySettingsVal, proxySettingsPresent := os.LookupEnv("PROXY_SETTINGS")
		if !proxySettingsPresent {
			t.Skip("No proxy settings were set in the global PROXY_SETTINGS variable.")
		}
		settings := make(map[string]string)
		if err := json.Unmarshal([]byte(proxySettingsVal), &settings); err != nil {
			t.Fatal(err)
		}
		ctx, logger, vm := agents.CommonSetup(t, platform)
		if err := gce.SetEnvironmentVariables(ctx, logger.ToMainLog(), vm, settings); err != nil {
			t.Fatal(err)
		}
		if err := setupOpsAgent(ctx, logger, vm, ""); err != nil {
			t.Fatal(err)
		}

		if err := gce.RemoveExternalIP(ctx, logger.ToMainLog(), vm); err != nil {
			t.Fatal(err)
		}
		// Sleep for 3 minutes to make sure that if any metrics were sent between agent install and removal of the IP address, then they will fall out of the 2 minute window.
		time.Sleep(3 * time.Minute)
		testDefaultMetrics(ctx, t, logger, vm, 2*time.Minute)
	})
}

func TestExcludeMetrics(t *testing.T) {
	t.Parallel()
	gce.RunForEachPlatform(t, func(t *testing.T, platform string) {
		t.Parallel()

		ctx, logger, vm := agents.CommonSetup(t, platform)

		excludeConfig := `logging:
  receivers:
    syslog:
      type: files
      include_paths:
      - /var/log/messages
      - /var/log/syslog
  service:
    pipelines:
      default_pipeline:
        receivers: [syslog]
metrics:
  receivers:
    hostmetrics:
      type: hostmetrics
      collection_interval: 60s
  processors:
    metrics_filter:
      type: exclude_metrics
      metrics_pattern:
      - agent.googleapis.com/processes/*
  service:
    pipelines:
      default_pipeline:
        receivers: [hostmetrics]
        processors: [metrics_filter]
`

		if err := setupOpsAgent(ctx, logger, vm, excludeConfig); err != nil {
			t.Fatal(err)
		}

		// Wait long enough for the data to percolate through the backends
		// under normal circumstances. Based on some experiments, 2 minutes
		// is normal; wait a bit longer to be on the safe side.
		time.Sleep(3 * time.Minute)

		existingMetric := "agent.googleapis.com/cpu/load_1m"
		excludedMetric := "agent.googleapis.com/processes/cpu_time"

		window := time.Minute
		if err := gce.WaitForMetric(ctx, logger.ToMainLog(), vm, existingMetric, window, nil); err != nil {
			t.Error(err)
		}
		if err := gce.AssertMetricMissing(ctx, logger.ToMainLog(), vm, excludedMetric, window); err != nil {
			t.Error(err)
		}
	})
}

// fetchPID returns the process ID of the process with the given name on the given VM.
func fetchPID(ctx context.Context, logger *log.Logger, vm *gce.VM, processName string) (string, error) {
	var cmd string
	if gce.IsWindows(vm.Platform) {
		cmd = fmt.Sprintf("Get-Process -Name '%s' | Select-Object -Property Id | Format-Wide", processName)
	} else {
		cmd = "sudo pgrep " + processName
	}
	output, err := gce.RunRemotely(ctx, logger, vm, "", cmd)
	if err != nil {
		return "", fmt.Errorf("fetchPID(%q) failed: %v", processName, err)
	}
	return strings.TrimSpace(output.Stdout), nil
}

// fetchPIDAndProcessName returns the process ID and name of the first matching process from a given list of names on the given VM.
func fetchPIDAndProcessName(ctx context.Context, logger *log.Logger, vm *gce.VM, processNames []string) (string, string, error) {
	var errors error
	for _, pn := range processNames {
		output, err := fetchPID(ctx, logger, vm, pn)
		if err != nil {
			errors = multierr.Append(errors, err)
		} else {
			return output, pn, nil
		}
	}
	return "", "", errors
}

func terminateProcess(ctx context.Context, logger *log.Logger, vm *gce.VM, processName string) error {
	var cmd string
	if gce.IsWindows(vm.Platform) {
		cmd = fmt.Sprintf("Stop-Process -Name '%s' -PassThru -Force", processName)
	} else {
		cmd = "sudo pkill -SIGABRT " + processName
	}
	_, err := gce.RunRemotely(ctx, logger, vm, "", cmd)
	if err != nil {
		return fmt.Errorf("terminateProcess(%q) failed: %v", processName, err)
	}
	return nil
}

func testAgentCrashRestart(ctx context.Context, t *testing.T, logger *logging.DirectoryLogger, vm *gce.VM, processNames []string, livenessChecker func(context.Context, *log.Logger, *gce.VM) error) {
	if err := setupOpsAgent(ctx, logger, vm, ""); err != nil {
		t.Fatal(err)
	}

	pidOutputBefore, processName, err := fetchPIDAndProcessName(ctx, logger.ToMainLog(), vm, processNames)
	if err != nil {
		t.Fatal(err)
	}

	logger.ToMainLog().Printf("testAgentCrashRestart: Found %s", processName)

	// Simulate a crash.
	if err := terminateProcess(ctx, logger.ToMainLog(), vm, processName); err != nil {
		t.Fatal(err)
	}

	if err := livenessChecker(ctx, logger.ToMainLog(), vm); err != nil {
		t.Fatalf("Liveness checker reported error: %v", err)
	}

	// Consistency check: make sure that the agent's PID actually changed so that
	// we know we crashed it successfully.
	pidOutputAfter, err := fetchPID(ctx, logger.ToMainLog(), vm, processName)
	if err != nil {
		t.Fatal(err)
	}

	if pidOutputBefore == pidOutputAfter {
		t.Errorf("PID did not change; we failed to crash %v.", processName)
	}
}

func TestMetricsAgentCrashRestart(t *testing.T) {
	t.Parallel()
	gce.RunForEachPlatform(t, func(t *testing.T, platform string) {
		t.Parallel()
		ctx, logger, vm := agents.CommonSetup(t, platform)

		livenessChecker := func(ctx context.Context, logger *log.Logger, vm *gce.VM) error {
			time.Sleep(3 * time.Minute)

			if err := gce.WaitForMetric(ctx, logger, vm, "agent.googleapis.com/cpu/utilization", time.Minute, nil); err != nil {
				return err
			}
			return nil
		}
		testAgentCrashRestart(ctx, t, logger, vm, metricsAgentProcessNamesForPlatform(vm.Platform), livenessChecker)
	})
}

func TestLoggingAgentCrashRestart(t *testing.T) {
	t.Parallel()
	gce.RunForEachPlatform(t, func(t *testing.T, platform string) {
		t.Parallel()
		ctx, logger, vm := agents.CommonSetup(t, platform)

		livenessChecker := func(ctx context.Context, logger *log.Logger, vm *gce.VM) error {
			if err := writeToSystemLog(ctx, logger, vm, "33337777"); err != nil {
				t.Fatal(err)
			}

			tag := systemLogTagForPlatform(vm.Platform)
			if err := gce.WaitForLog(ctx, logger, vm, tag, time.Hour, logMessageQueryForPlatform(vm.Platform, "33337777")); err != nil {
				return err
			}
			return nil
		}
		testAgentCrashRestart(ctx, t, logger, vm, []string{"fluent-bit"}, livenessChecker)
	})
}

func testWindowsStandaloneAgentConflict(t *testing.T, installStandalone func(ctx context.Context, logger *log.Logger, vm *gce.VM) error, wantError string) {
	t.Parallel()
	gce.RunForEachPlatform(t, func(t *testing.T, platform string) {
		t.Parallel()
		if !gce.IsWindows(platform) {
			t.SkipNow()
		}
		ctx, logger, vm := agents.CommonSetup(t, platform)

		// 1. Install the standalone agent.
		if err := installStandalone(ctx, logger.ToMainLog(), vm); err != nil {
			t.Fatal(err)
		}

		// 2. Install the Ops Agent.  Installation will succeed but log an error.
		if err := installOpsAgent(ctx, logger, vm); err != nil {
			t.Fatal(err)
		}

		// 3. Check the error log for a message about Ops Agent conflicting with standalone agent.
		getEvents := `Get-WinEvent -FilterHashtable @{
		  LogName = 'Application'
			ProviderName = 'google-cloud-ops-agent'
		} | Select-Object -ExpandProperty Message`
		out, err := gce.RunRemotely(ctx, logger.ToMainLog(), vm, "", getEvents)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.Stdout, wantError) {
			t.Fatalf("got error log = %q, want substring %q", out.Stdout, wantError)
		}
	})
}

func TestWindowsLoggingAgentConflict(t *testing.T) {
	wantError := "We detected an existing Windows service for the StackdriverLogging agent"
	testWindowsStandaloneAgentConflict(t, agents.InstallStandaloneWindowsLoggingAgent, wantError)
}

func TestWindowsMonitoringAgentConflict(t *testing.T) {
	wantError := "We detected an existing Windows service for the StackdriverMonitoring agent"
	testWindowsStandaloneAgentConflict(t, agents.InstallStandaloneWindowsMonitoringAgent, wantError)
}

func TestMain(m *testing.M) {
	code := m.Run()
	gce.CleanupKeysOrDie()
	os.Exit(code)
}

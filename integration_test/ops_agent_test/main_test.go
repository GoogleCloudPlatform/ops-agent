// Copyright 2022 Google LLC
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
Test for core functionality of the Ops Agent.
Can be run with Kokoro or "go test". For instructions, see the top of
gce_testing.go.

This test needs the following environment variables to be defined, in addition
to the ones mentioned at the top of gce_testing.go:

IAMGE_SPECS: a comma-separated list of images to test, e.g. "centos-cloud:centos-7,centos-cloud:centos-8".

The following variables are optional:

REPO_SUFFIX: If provided, what package repo suffix to install the ops agent from.
ARTIFACT_REGISTRY_REGION: If provided, signals to the install scripts that the

	above REPO_SUFFIX is an artifact registry repo and specifies what region it
	is in. If not provided, that means that the packages are accessed through
	packages.cloud.google.com instead, which may point to Cloud Rapture or
	Artifact Registry under the hood.

AGENT_PACKAGES_IN_GCS: If provided, a URL for a directory in GCS containing

	.deb/.rpm/.goo files to install on the testing VMs. Takes precedence over
	REPO_SUFFIX.
*/
package ops_agent_test

import (
	"context"
	"embed"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	cloudlogging "cloud.google.com/go/logging"
	"cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/resourcedetector"
	"github.com/GoogleCloudPlatform/ops-agent/integration_test/agents"
	feature_tracking_metadata "github.com/GoogleCloudPlatform/ops-agent/integration_test/feature_tracking"
	"github.com/GoogleCloudPlatform/ops-agent/integration_test/gce"
	"github.com/GoogleCloudPlatform/ops-agent/integration_test/metadata"
	"github.com/GoogleCloudPlatform/ops-agent/integration_test/util"
	"github.com/google/uuid"
	"go.uber.org/multierr"
	"golang.org/x/exp/slices"
	"google.golang.org/genproto/googleapis/api/distribution"
	"google.golang.org/genproto/googleapis/api/metric"
	"google.golang.org/protobuf/proto"
	structpb "google.golang.org/protobuf/types/known/structpb"
	"gopkg.in/yaml.v2"
)

//go:embed testdata
var testdataDir embed.FS

func logPathForImage(imageSpec string) string {
	if gce.IsWindows(imageSpec) {
		return `C:\mylog`
	}
	return "/tmp/mylog"
}

func workDirForImage(imageSpec string) string {
	if gce.IsWindows(imageSpec) {
		return `C:\work`
	}
	return "/tmp/work"
}

func startCommandForImage(imageSpec string) string {
	if gce.IsWindows(imageSpec) {
		return "Start-Service google-cloud-ops-agent"
	}
	// Return a command that works for both < 2.0.0 and >= 2.0.0 agents.
	return "sudo service google-cloud-ops-agent start || sudo systemctl start google-cloud-ops-agent"
}

func stopCommandForImage(imageSpec string) string {
	if gce.IsWindows(imageSpec) {
		return "Stop-Service google-cloud-ops-agent -Force"
	}
	// Return a command that works for both < 2.0.0 and >= 2.0.0 agents.
	return "sudo service google-cloud-ops-agent stop || sudo systemctl stop google-cloud-ops-agent"
}

func systemLogTagForImage(imageSpec string) string {
	if gce.IsWindows(imageSpec) {
		return "windows_event_log"
	}
	return "syslog"
}

func logMessageQueryForImage(imageSpec, message string) string {
	if gce.IsWindows(imageSpec) {
		// On Windows, we have to look in the capital-M Message field. On linux
		// it is lowercase-m message.
		// Also, Fluent-Bit adds an extra newline to our payload, so we need to
		// use ":" instead of "=" here.
		return "jsonPayload.Message:" + message
	}
	return "jsonPayload.message=" + message
}

func metricsAgentProcessNamesForImage(imageSpec string) []string {
	if gce.IsWindows(imageSpec) {
		return []string{"google-cloud-metrics-agent_windows_amd64"}
	}
	return []string{"otelopscol", "collectd"}
}

func diagnosticsProcessNamesForImage(imageSpec string) []string {
	if gce.IsWindows(imageSpec) {
		return []string{"google-cloud-ops-agent-diagnostics"}
	}
	return []string{"google_cloud_ops_agent_diagnostics"}
}

func makeDirectory(ctx context.Context, logger *log.Logger, vm *gce.VM, directory string) error {
	var createFolderCmd string
	if gce.IsWindows(vm.ImageSpec) {
		createFolderCmd = fmt.Sprintf("New-Item -ItemType Directory -Path %s", directory)
	} else {
		createFolderCmd = fmt.Sprintf("mkdir -p %s", directory)
	}
	_, err := gce.RunRemotely(ctx, logger, vm, createFolderCmd)
	return err
}

// writeToWindowsEventLog writes an event payload to the given channel (logName).
func writeToWindowsEventLog(ctx context.Context, logger *log.Logger, vm *gce.VM, logName, payload string) error {
	return writeToWindowsEventLogWithSeverity(ctx, logger, vm, logName, payload, "Information")
}

// writeToWindowsEventLogWithSeverity writes an event payload to the given channel (logName)
// using the given severity (Error, Information, FailureAudit, SuccessAudit, Warning).
func writeToWindowsEventLogWithSeverity(ctx context.Context, logger *log.Logger, vm *gce.VM, logName, payload string, severity string) error {
	// If this is the first time we're trying to write to logName, we need to
	// register a fake log source with New-EventLog.
	// There's a problem:  there's no way (that I can find) to check whether a
	// particular log source is registered to write to logName: the closest I
	// can get is checking whether a log source is registered to write
	// *somewhere*. So the workaround is to make the log source's name unique
	// per logName.
	source := logName + "__ops_agent_test"
	if _, err := gce.RunRemotely(ctx, logger, vm, fmt.Sprintf("if(![System.Diagnostics.EventLog]::SourceExists('%s')) { New-EventLog -LogName '%s' -Source '%s' }", source, logName, source)); err != nil {
		return fmt.Errorf("writeToWindowsEventLogWithSeverity(logName=%q, payload=%q, severity=%q) failed to register new source %v: %v", logName, payload, severity, source, err)
	}

	// Use a Powershell here-string to avoid most quoting issues.
	if _, err := gce.RunRemotely(ctx, logger, vm, fmt.Sprintf("Write-EventLog -LogName '%s' -EventId 1 -Source '%s' -EntryType '%s' -Message @'\n%s\n'@", logName, source, severity, payload)); err != nil {
		return fmt.Errorf("writeToWindowsEventLogWithSeverity(logName=%q, payload=%q, severity=%q) failed: %v", logName, payload, severity, err)
	}
	return nil
}

// setupMainLogAndVM sets up a VM for testing and returns it, along with a logger
// that writes to a file called main_log.txt.
// This function is just a wrapper for agents.CommonSetup that returns a "plain"
// log.Logger instead so that the callsite doesn't need to write
// logger.ToMainLog() throughout.
// If you need to write to something besides the main log, just call
// agents.CommonSetup instead.
func setupMainLogAndVM(t *testing.T, imageSpec string) (context.Context, *log.Logger, *gce.VM) {
	ctx, dirLog, vm := agents.CommonSetup(t, imageSpec)
	return ctx, dirLog.ToMainLog(), vm
}

// writeToSystemLog writes the given payload to the VM's normal log location.
// On Linux this is /var/log/syslog or /var/log/messages, depending on the
// distro.
// On Windows this is the "System" log within the event logging system.
func writeToSystemLog(ctx context.Context, logger *log.Logger, vm *gce.VM, payload string) error {
	if gce.IsWindows(vm.ImageSpec) {
		return writeToWindowsEventLog(ctx, logger, vm, "System", payload)
	}
	line := payload + "\n"
	location := gce.SyslogLocation(vm.ImageSpec)
	// Pass the content in on stdin and run "cat -" to tell cat to copy from stdin.
	// This is to avoid having to quote the content correctly for the shell.
	// "tee -a" will append to the file.
	if _, err := gce.RunRemotelyStdin(ctx, logger, vm, strings.NewReader(line), fmt.Sprintf("cat - | sudo tee -a '%s' > /dev/null", location)); err != nil {
		return fmt.Errorf("writeToSystemLog() failed to write %q to %s: %v", line, location, err)
	}
	return nil
}

// retrieveOtelConfig retrieves the file content of the generated Otel config
// file from the remote VM
func retrieveOtelConfig(ctx context.Context, logger *log.Logger, vm *gce.VM) (content string, err error) {
	return gce.RetrieveContent(ctx, logger, vm, util.GetOtelConfigPath(vm.ImageSpec))
}

func TestParseMultilineFileJava(t *testing.T) {
	t.Parallel()
	gce.RunForEachImage(t, func(t *testing.T, imageSpec string) {
		t.Parallel()
		if gce.IsWindows(imageSpec) {
			t.SkipNow()
		}
		ctx, logger, vm := setupMainLogAndVM(t, imageSpec)
		logPath := logPathForImage(vm.ImageSpec)
		config := fmt.Sprintf(`logging:
  receivers:
    files_1:
      type: files
      include_paths: [%s]
      record_log_file_path: true
      wildcard_refresh_interval: 30s
  processors:
    multiline_parser_1:
      type: parse_multiline
      match_any:
      - type: language_exceptions
        language: java
  service:
    pipelines:
      p1:
        receivers: [files_1]
        processors: [multiline_parser_1]`, logPath)

		//Below lines comes from 3 java exception stacktraces, thus expect 3 logEntries.
		if err := gce.UploadContent(ctx, logger, vm, strings.NewReader(`Jul 09, 2015 3:23:29 PM com.google.devtools.search.cloud.feeder.MakeLog: RuntimeException: Run from this message!
  at com.my.app.Object.do$a1(MakeLog.java:50)
  at java.lang.Thing.call(Thing.java:10)
javax.servlet.ServletException: Something bad happened
    at com.example.myproject.OpenSessionInViewFilter.doFilter(OpenSessionInViewFilter.java:60)
    at org.mortbay.jetty.servlet.ServletHandler$CachedChain.doFilter(ServletHandler.java:1157)
    at com.example.myproject.ExceptionHandlerFilter.doFilter(ExceptionHandlerFilter.java:28)
    at org.mortbay.jetty.servlet.ServletHandler$CachedChain.doFilter(ServletHandler.java:1157)
    at com.example.myproject.OutputBufferFilter.doFilter(OutputBufferFilter.java:33)
    at org.mortbay.jetty.servlet.ServletHandler$CachedChain.doFilter(ServletHandler.java:1157)
    at org.mortbay.jetty.servlet.ServletHandler.handle(ServletHandler.java:388)
    at org.mortbay.jetty.security.SecurityHandler.handle(SecurityHandler.java:216)
    at org.mortbay.jetty.servlet.SessionHandler.handle(SessionHandler.java:182)
    at org.mortbay.jetty.handler.ContextHandler.handle(ContextHandler.java:765)
    at org.mortbay.jetty.webapp.WebAppContext.handle(WebAppContext.java:418)
    at org.mortbay.jetty.handler.HandlerWrapper.handle(HandlerWrapper.java:152)
    at org.mortbay.jetty.Server.handle(Server.java:326)
    at org.mortbay.jetty.HttpConnection.handleRequest(HttpConnection.java:542)
    at org.mortbay.jetty.HttpConnection$RequestHandler.content(HttpConnection.java:943)
    at org.mortbay.jetty.HttpParser.parseNext(HttpParser.java:756)
    at org.mortbay.jetty.HttpParser.parseAvailable(HttpParser.java:218)
    at org.mortbay.jetty.HttpConnection.handle(HttpConnection.java:404)
    at org.mortbay.jetty.bio.SocketConnector$Connection.run(SocketConnector.java:228)
    at org.mortbay.thread.QueuedThreadPool$PoolThread.run(QueuedThreadPool.java:582)
Caused by: com.example.myproject.MyProjectServletException
    at com.example.myproject.MyServlet.doPost(MyServlet.java:169)
    at javax.servlet.http.HttpServlet.service(HttpServlet.java:727)
    at javax.servlet.http.HttpServlet.service(HttpServlet.java:820)
    at org.mortbay.jetty.servlet.ServletHolder.handle(ServletHolder.java:511)
    at org.mortbay.jetty.servlet.ServletHandler$CachedChain.doFilter(ServletHandler.java:1166)
    at com.example.myproject.OpenSessionInViewFilter.doFilter(OpenSessionInViewFilter.java:30)
    ... 27 common frames omitted
java.lang.RuntimeException: javax.mail.SendFailedException: Invalid Addresses;
  nested exception is:
com.sun.mail.smtp.SMTPAddressFailedException: 550 5.7.1 <[REDACTED_EMAIL_ADDRESS]>... Relaying denied
	at com.nethunt.crm.api.server.adminsync.AutomaticEmailFacade.sendWithSmtp(AutomaticEmailFacade.java:236)
	at com.nethunt.crm.api.server.adminsync.AutomaticEmailFacade.sendSingleEmail(AutomaticEmailFacade.java:285)
	at com.nethunt.crm.api.server.adminsync.AutomaticEmailFacade.lambda$sendSingleEmail$3(AutomaticEmailFacade.java:254)
	at java.util.Optional.ifPresent(Optional.java:159)
	at com.nethunt.crm.api.server.adminsync.AutomaticEmailFacade.sendSingleEmail(AutomaticEmailFacade.java:253)
	at com.nethunt.crm.api.server.adminsync.AutomaticEmailFacade.sendSingleEmail(AutomaticEmailFacade.java:249)
	at com.nethunt.crm.api.email.EmailSender.lambda$notifyPerson$0(EmailSender.java:80)
	at com.nethunt.crm.api.util.ManagedExecutor.lambda$execute$0(ManagedExecutor.java:36)
	at com.nethunt.crm.api.util.RequestContextActivator.lambda$withRequestContext$0(RequestContextActivator.java:36)
	at java.base/java.util.concurrent.ThreadPoolExecutor.runWorker(ThreadPoolExecutor.java:1149)
	at java.base/java.util.concurrent.ThreadPoolExecutor$Worker.run(ThreadPoolExecutor.java:624)
	at java.base/java.lang.Thread.run(Thread.java:748)
Caused by: javax.mail.SendFailedException: Invalid Addresses;
  nested exception is:
com.sun.mail.smtp.SMTPAddressFailedException: 550 5.7.1 <[REDACTED_EMAIL_ADDRESS]>... Relaying denied
	at com.sun.mail.smtp.SMTPTransport.rcptTo(SMTPTransport.java:2064)
	at com.sun.mail.smtp.SMTPTransport.sendMessage(SMTPTransport.java:1286)
	at com.nethunt.crm.api.server.adminsync.AutomaticEmailFacade.sendWithSmtp(AutomaticEmailFacade.java:229)
	... 12 more
Caused by: com.sun.mail.smtp.SMTPAddressFailedException: 550 5.7.1 <[REDACTED_EMAIL_ADDRESS]>... Relaying denied
`), logPath); err != nil {
			t.Fatalf("error writing dummy log lines for Java: %v", err)
		}

		if err := agents.SetupOpsAgent(ctx, logger, vm, config); err != nil {
			t.Fatal(err)
		}

		if err := gce.WaitForLog(ctx, logger, vm, "files_1", time.Hour, `jsonPayload.message="Jul 09, 2015 3:23:29 PM com.google.devtools.search.cloud.feeder.MakeLog: RuntimeException: Run from this message!\n  at com.my.app.Object.do$a1(MakeLog.java:50)\n  at java.lang.Thing.call(Thing.java:10)\n"`); err != nil {
			t.Error(err)
		}
		if err := gce.WaitForLog(ctx, logger, vm, "files_1", time.Hour, `jsonPayload.message="javax.servlet.ServletException: Something bad happened\n    at com.example.myproject.OpenSessionInViewFilter.doFilter(OpenSessionInViewFilter.java:60)\n    at org.mortbay.jetty.servlet.ServletHandler$CachedChain.doFilter(ServletHandler.java:1157)\n    at com.example.myproject.ExceptionHandlerFilter.doFilter(ExceptionHandlerFilter.java:28)\n    at org.mortbay.jetty.servlet.ServletHandler$CachedChain.doFilter(ServletHandler.java:1157)\n    at com.example.myproject.OutputBufferFilter.doFilter(OutputBufferFilter.java:33)\n    at org.mortbay.jetty.servlet.ServletHandler$CachedChain.doFilter(ServletHandler.java:1157)\n    at org.mortbay.jetty.servlet.ServletHandler.handle(ServletHandler.java:388)\n    at org.mortbay.jetty.security.SecurityHandler.handle(SecurityHandler.java:216)\n    at org.mortbay.jetty.servlet.SessionHandler.handle(SessionHandler.java:182)\n    at org.mortbay.jetty.handler.ContextHandler.handle(ContextHandler.java:765)\n    at org.mortbay.jetty.webapp.WebAppContext.handle(WebAppContext.java:418)\n    at org.mortbay.jetty.handler.HandlerWrapper.handle(HandlerWrapper.java:152)\n    at org.mortbay.jetty.Server.handle(Server.java:326)\n    at org.mortbay.jetty.HttpConnection.handleRequest(HttpConnection.java:542)\n    at org.mortbay.jetty.HttpConnection$RequestHandler.content(HttpConnection.java:943)\n    at org.mortbay.jetty.HttpParser.parseNext(HttpParser.java:756)\n    at org.mortbay.jetty.HttpParser.parseAvailable(HttpParser.java:218)\n    at org.mortbay.jetty.HttpConnection.handle(HttpConnection.java:404)\n    at org.mortbay.jetty.bio.SocketConnector$Connection.run(SocketConnector.java:228)\n    at org.mortbay.thread.QueuedThreadPool$PoolThread.run(QueuedThreadPool.java:582)\nCaused by: com.example.myproject.MyProjectServletException\n    at com.example.myproject.MyServlet.doPost(MyServlet.java:169)\n    at javax.servlet.http.HttpServlet.service(HttpServlet.java:727)\n    at javax.servlet.http.HttpServlet.service(HttpServlet.java:820)\n    at org.mortbay.jetty.servlet.ServletHolder.handle(ServletHolder.java:511)\n    at org.mortbay.jetty.servlet.ServletHandler$CachedChain.doFilter(ServletHandler.java:1166)\n    at com.example.myproject.OpenSessionInViewFilter.doFilter(OpenSessionInViewFilter.java:30)\n    ... 27 common frames omitted\n"`); err != nil {
			t.Error(err)
		}
	})
}

func TestParseMultilineFileJavaPython(t *testing.T) {
	t.Parallel()
	gce.RunForEachImage(t, func(t *testing.T, imageSpec string) {
		t.Parallel()
		if gce.IsWindows(imageSpec) {
			t.SkipNow()
		}
		ctx, logger, vm := setupMainLogAndVM(t, imageSpec)
		logPath := logPathForImage(vm.ImageSpec)
		config := fmt.Sprintf(`logging:
  receivers:
    files_1:
      type: files
      include_paths: [%s]
      wildcard_refresh_interval: 30s
  processors:
    multiline_parser_1:
      type: parse_multiline
      match_any:
      - type: language_exceptions
        language: java
      - type: language_exceptions
        language: python
  service:
    pipelines:
      p1:
        receivers: [files_1]
        processors: [multiline_parser_1]`, logPath)

		//Below lines comes from 3 java and 3 python exception stacktraces, thus expect 6 logEntries.
		if err := gce.UploadContent(ctx, logger, vm, strings.NewReader(`Jul 09, 2015 3:23:29 PM com.google.devtools.search.cloud.feeder.MakeLog: RuntimeException: Run from this message!
  at com.my.app.Object.do$a1(MakeLog.java:50)
  at java.lang.Thing.call(Thing.java:10)
Traceback (most recent call last):
  File "/base/data/home/runtimes/python27/python27_lib/versions/third_party/webapp2-2.5.2/webapp2.py", line 1535, in __call__
    rv = self.handle_exception(request, response, e)
  File "/base/data/home/apps/s~nearfieldspy/1.378705245900539993/nearfieldspy.py", line 17, in start
    return get()
  File "/base/data/home/apps/s~nearfieldspy/1.378705245900539993/nearfieldspy.py", line 5, in get
    raise Exception('spam', 'eggs')
Exception: ('spam', 'eggs')
javax.servlet.ServletException: Something bad happened
    at com.example.myproject.OpenSessionInViewFilter.doFilter(OpenSessionInViewFilter.java:60)
    at org.mortbay.jetty.servlet.ServletHandler$CachedChain.doFilter(ServletHandler.java:1157)
    at com.example.myproject.ExceptionHandlerFilter.doFilter(ExceptionHandlerFilter.java:28)
    at org.mortbay.jetty.servlet.ServletHandler$CachedChain.doFilter(ServletHandler.java:1157)
    at com.example.myproject.OutputBufferFilter.doFilter(OutputBufferFilter.java:33)
    at org.mortbay.jetty.servlet.ServletHandler$CachedChain.doFilter(ServletHandler.java:1157)
    at org.mortbay.jetty.servlet.ServletHandler.handle(ServletHandler.java:388)
    at org.mortbay.jetty.security.SecurityHandler.handle(SecurityHandler.java:216)
    at org.mortbay.jetty.servlet.SessionHandler.handle(SessionHandler.java:182)
    at org.mortbay.jetty.handler.ContextHandler.handle(ContextHandler.java:765)
    at org.mortbay.jetty.webapp.WebAppContext.handle(WebAppContext.java:418)
    at org.mortbay.jetty.handler.HandlerWrapper.handle(HandlerWrapper.java:152)
    at org.mortbay.jetty.Server.handle(Server.java:326)
    at org.mortbay.jetty.HttpConnection.handleRequest(HttpConnection.java:542)
    at org.mortbay.jetty.HttpConnection$RequestHandler.content(HttpConnection.java:943)
    at org.mortbay.jetty.HttpParser.parseNext(HttpParser.java:756)
    at org.mortbay.jetty.HttpParser.parseAvailable(HttpParser.java:218)
    at org.mortbay.jetty.HttpConnection.handle(HttpConnection.java:404)
    at org.mortbay.jetty.bio.SocketConnector$Connection.run(SocketConnector.java:228)
    at org.mortbay.thread.QueuedThreadPool$PoolThread.run(QueuedThreadPool.java:582)
Caused by: com.example.myproject.MyProjectServletException
    at com.example.myproject.MyServlet.doPost(MyServlet.java:169)
    at javax.servlet.http.HttpServlet.service(HttpServlet.java:727)
    at javax.servlet.http.HttpServlet.service(HttpServlet.java:820)
    at org.mortbay.jetty.servlet.ServletHolder.handle(ServletHolder.java:511)
    at org.mortbay.jetty.servlet.ServletHandler$CachedChain.doFilter(ServletHandler.java:1166)
    at com.example.myproject.OpenSessionInViewFilter.doFilter(OpenSessionInViewFilter.java:30)
    ... 27 common frames omitted
java.lang.RuntimeException: javax.mail.SendFailedException: Invalid Addresses;
  nested exception is:
com.sun.mail.smtp.SMTPAddressFailedException: 550 5.7.1 <[REDACTED_EMAIL_ADDRESS]>... Relaying denied
	at com.nethunt.crm.api.server.adminsync.AutomaticEmailFacade.sendWithSmtp(AutomaticEmailFacade.java:236)
	at com.nethunt.crm.api.server.adminsync.AutomaticEmailFacade.sendSingleEmail(AutomaticEmailFacade.java:285)
	at com.nethunt.crm.api.server.adminsync.AutomaticEmailFacade.lambda$sendSingleEmail$3(AutomaticEmailFacade.java:254)
	at java.util.Optional.ifPresent(Optional.java:159)
	at com.nethunt.crm.api.server.adminsync.AutomaticEmailFacade.sendSingleEmail(AutomaticEmailFacade.java:253)
	at com.nethunt.crm.api.server.adminsync.AutomaticEmailFacade.sendSingleEmail(AutomaticEmailFacade.java:249)
	at com.nethunt.crm.api.email.EmailSender.lambda$notifyPerson$0(EmailSender.java:80)
	at com.nethunt.crm.api.util.ManagedExecutor.lambda$execute$0(ManagedExecutor.java:36)
	at com.nethunt.crm.api.util.RequestContextActivator.lambda$withRequestContext$0(RequestContextActivator.java:36)
	at java.base/java.util.concurrent.ThreadPoolExecutor.runWorker(ThreadPoolExecutor.java:1149)
	at java.base/java.util.concurrent.ThreadPoolExecutor$Worker.run(ThreadPoolExecutor.java:624)
	at java.base/java.lang.Thread.run(Thread.java:748)
Caused by: javax.mail.SendFailedException: Invalid Addresses;
  nested exception is:
com.sun.mail.smtp.SMTPAddressFailedException: 550 5.7.1 <[REDACTED_EMAIL_ADDRESS]>... Relaying denied
	at com.sun.mail.smtp.SMTPTransport.rcptTo(SMTPTransport.java:2064)
	at com.sun.mail.smtp.SMTPTransport.sendMessage(SMTPTransport.java:1286)
	at com.nethunt.crm.api.server.adminsync.AutomaticEmailFacade.sendWithSmtp(AutomaticEmailFacade.java:229)
	... 12 more
Caused by: com.sun.mail.smtp.SMTPAddressFailedException: 550 5.7.1 <[REDACTED_EMAIL_ADDRESS]>... Relaying denied
Traceback (most recent call last):
  File "/test/exception.py", line 21, in <module>
    conn.request("GET", "/")
  File "/usr/lib/python3.10/http/client.py", line 1282, in request
    self._send_request(method, url, body, headers, encode_chunked)
  File "/usr/lib/python3.10/http/client.py", line 1328, in _send_request
    self.endheaders(body, encode_chunked=encode_chunked)
  File "/usr/lib/python3.10/http/client.py", line 1277, in endheaders
    self._send_output(message_body, encode_chunked=encode_chunked)
  File "/usr/lib/python3.10/http/client.py", line 1037, in _send_output
    self.send(msg)
  File "/usr/lib/python3.10/http/client.py", line 975, in send
    self.connect()
  File "/usr/lib/python3.10/http/client.py", line 941, in connect
    self.sock = self._create_connection(
  File "/usr/lib/python3.10/socket.py", line 824, in create_connection
    for res in getaddrinfo(host, port, 0, SOCK_STREAM):
  File "/usr/lib/python3.10/socket.py", line 955, in getaddrinfo
    for res in _socket.getaddrinfo(host, port, family, type, proto, flags):
socket.gaierror: [Errno -2] Name or service not known
Traceback (most recent call last):
  File "/usr/local/google/home/lujieduan/source/test/exception.py", line 11, in <module>
    '2' + 2
TypeError: can only concatenate str (not "int") to str
`), logPath); err != nil {
			t.Fatalf("error writing dummy log lines for Java + Python: %v", err)
		}

		if err := agents.SetupOpsAgent(ctx, logger, vm, config); err != nil {
			t.Fatal(err)
		}

		// 1st one is Java
		if err := gce.WaitForLog(ctx, logger, vm, "files_1", time.Hour, `jsonPayload.message="Jul 09, 2015 3:23:29 PM com.google.devtools.search.cloud.feeder.MakeLog: RuntimeException: Run from this message!\n  at com.my.app.Object.do$a1(MakeLog.java:50)\n  at java.lang.Thing.call(Thing.java:10)\n"`); err != nil {
			t.Error(err)
		}

		// 2nd Python
		if err := gce.WaitForLog(ctx, logger, vm, "files_1", time.Hour, `jsonPayload.message="Traceback (most recent call last):\n  File \"/base/data/home/runtimes/python27/python27_lib/versions/third_party/webapp2-2.5.2/webapp2.py\", line 1535, in __call__\n    rv = self.handle_exception(request, response, e)\n  File \"/base/data/home/apps/s~nearfieldspy/1.378705245900539993/nearfieldspy.py\", line 17, in start\n    return get()\n  File \"/base/data/home/apps/s~nearfieldspy/1.378705245900539993/nearfieldspy.py\", line 5, in get\n    raise Exception('spam', 'eggs')\nException: ('spam', 'eggs')\n"`); err != nil {
			t.Error(err)
		}

		// 3rd Java
		if err := gce.WaitForLog(ctx, logger, vm, "files_1", time.Hour, `jsonPayload.message="javax.servlet.ServletException: Something bad happened\n    at com.example.myproject.OpenSessionInViewFilter.doFilter(OpenSessionInViewFilter.java:60)\n    at org.mortbay.jetty.servlet.ServletHandler$CachedChain.doFilter(ServletHandler.java:1157)\n    at com.example.myproject.ExceptionHandlerFilter.doFilter(ExceptionHandlerFilter.java:28)\n    at org.mortbay.jetty.servlet.ServletHandler$CachedChain.doFilter(ServletHandler.java:1157)\n    at com.example.myproject.OutputBufferFilter.doFilter(OutputBufferFilter.java:33)\n    at org.mortbay.jetty.servlet.ServletHandler$CachedChain.doFilter(ServletHandler.java:1157)\n    at org.mortbay.jetty.servlet.ServletHandler.handle(ServletHandler.java:388)\n    at org.mortbay.jetty.security.SecurityHandler.handle(SecurityHandler.java:216)\n    at org.mortbay.jetty.servlet.SessionHandler.handle(SessionHandler.java:182)\n    at org.mortbay.jetty.handler.ContextHandler.handle(ContextHandler.java:765)\n    at org.mortbay.jetty.webapp.WebAppContext.handle(WebAppContext.java:418)\n    at org.mortbay.jetty.handler.HandlerWrapper.handle(HandlerWrapper.java:152)\n    at org.mortbay.jetty.Server.handle(Server.java:326)\n    at org.mortbay.jetty.HttpConnection.handleRequest(HttpConnection.java:542)\n    at org.mortbay.jetty.HttpConnection$RequestHandler.content(HttpConnection.java:943)\n    at org.mortbay.jetty.HttpParser.parseNext(HttpParser.java:756)\n    at org.mortbay.jetty.HttpParser.parseAvailable(HttpParser.java:218)\n    at org.mortbay.jetty.HttpConnection.handle(HttpConnection.java:404)\n    at org.mortbay.jetty.bio.SocketConnector$Connection.run(SocketConnector.java:228)\n    at org.mortbay.thread.QueuedThreadPool$PoolThread.run(QueuedThreadPool.java:582)\nCaused by: com.example.myproject.MyProjectServletException\n    at com.example.myproject.MyServlet.doPost(MyServlet.java:169)\n    at javax.servlet.http.HttpServlet.service(HttpServlet.java:727)\n    at javax.servlet.http.HttpServlet.service(HttpServlet.java:820)\n    at org.mortbay.jetty.servlet.ServletHolder.handle(ServletHolder.java:511)\n    at org.mortbay.jetty.servlet.ServletHandler$CachedChain.doFilter(ServletHandler.java:1166)\n    at com.example.myproject.OpenSessionInViewFilter.doFilter(OpenSessionInViewFilter.java:30)\n    ... 27 common frames omitted\n"`); err != nil {
			t.Error(err)
		}

		// 4th Java
		if err := gce.WaitForLog(ctx, logger, vm, "files_1", time.Hour, `jsonPayload.message="java.lang.RuntimeException: javax.mail.SendFailedException: Invalid Addresses;\n  nested exception is:\ncom.sun.mail.smtp.SMTPAddressFailedException: 550 5.7.1 <[REDACTED_EMAIL_ADDRESS]>... Relaying denied\n	at com.nethunt.crm.api.server.adminsync.AutomaticEmailFacade.sendWithSmtp(AutomaticEmailFacade.java:236)\n	at com.nethunt.crm.api.server.adminsync.AutomaticEmailFacade.sendSingleEmail(AutomaticEmailFacade.java:285)\n	at com.nethunt.crm.api.server.adminsync.AutomaticEmailFacade.lambda$sendSingleEmail$3(AutomaticEmailFacade.java:254)\n	at java.util.Optional.ifPresent(Optional.java:159)\n	at com.nethunt.crm.api.server.adminsync.AutomaticEmailFacade.sendSingleEmail(AutomaticEmailFacade.java:253)\n	at com.nethunt.crm.api.server.adminsync.AutomaticEmailFacade.sendSingleEmail(AutomaticEmailFacade.java:249)\n	at com.nethunt.crm.api.email.EmailSender.lambda$notifyPerson$0(EmailSender.java:80)\n	at com.nethunt.crm.api.util.ManagedExecutor.lambda$execute$0(ManagedExecutor.java:36)\n	at com.nethunt.crm.api.util.RequestContextActivator.lambda$withRequestContext$0(RequestContextActivator.java:36)\n	at java.base/java.util.concurrent.ThreadPoolExecutor.runWorker(ThreadPoolExecutor.java:1149)\n	at java.base/java.util.concurrent.ThreadPoolExecutor$Worker.run(ThreadPoolExecutor.java:624)\n	at java.base/java.lang.Thread.run(Thread.java:748)\nCaused by: javax.mail.SendFailedException: Invalid Addresses;\n  nested exception is:\ncom.sun.mail.smtp.SMTPAddressFailedException: 550 5.7.1 <[REDACTED_EMAIL_ADDRESS]>... Relaying denied\n	at com.sun.mail.smtp.SMTPTransport.rcptTo(SMTPTransport.java:2064)\n	at com.sun.mail.smtp.SMTPTransport.sendMessage(SMTPTransport.java:1286)\n	at com.nethunt.crm.api.server.adminsync.AutomaticEmailFacade.sendWithSmtp(AutomaticEmailFacade.java:229)\n	... 12 more\nCaused by: com.sun.mail.smtp.SMTPAddressFailedException: 550 5.7.1 <[REDACTED_EMAIL_ADDRESS]>... Relaying denied\n"`); err != nil {
			t.Error(err)
		}

		// 5th Python
		if err := gce.WaitForLog(ctx, logger, vm, "files_1", time.Hour, `jsonPayload.message="Traceback (most recent call last):\n  File \"/test/exception.py\", line 21, in <module>\n    conn.request(\"GET\", \"/\")\n  File \"/usr/lib/python3.10/http/client.py\", line 1282, in request\n    self._send_request(method, url, body, headers, encode_chunked)\n  File \"/usr/lib/python3.10/http/client.py\", line 1328, in _send_request\n    self.endheaders(body, encode_chunked=encode_chunked)\n  File \"/usr/lib/python3.10/http/client.py\", line 1277, in endheaders\n    self._send_output(message_body, encode_chunked=encode_chunked)\n  File \"/usr/lib/python3.10/http/client.py\", line 1037, in _send_output\n    self.send(msg)\n  File \"/usr/lib/python3.10/http/client.py\", line 975, in send\n    self.connect()\n  File \"/usr/lib/python3.10/http/client.py\", line 941, in connect\n    self.sock = self._create_connection(\n  File \"/usr/lib/python3.10/socket.py\", line 824, in create_connection\n    for res in getaddrinfo(host, port, 0, SOCK_STREAM):\n  File \"/usr/lib/python3.10/socket.py\", line 955, in getaddrinfo\n    for res in _socket.getaddrinfo(host, port, family, type, proto, flags):\nsocket.gaierror: [Errno -2] Name or service not known\n"`); err != nil {
			t.Error(err)
		}

		// 6th Python
		if err := gce.WaitForLog(ctx, logger, vm, "files_1", time.Hour, `jsonPayload.message="Traceback (most recent call last):\n  File \"/usr/local/google/home/lujieduan/source/test/exception.py\", line 11, in <module>\n    '2' + 2\nTypeError: can only concatenate str (not \"int\") to str\n"`); err != nil {
			t.Error(err)
		}
	})
}

func TestParseMultilineFileGolangJavaPython(t *testing.T) {
	t.Parallel()
	gce.RunForEachImage(t, func(t *testing.T, imageSpec string) {
		t.Parallel()
		if gce.IsWindows(imageSpec) {
			t.SkipNow()
		}
		ctx, logger, vm := setupMainLogAndVM(t, imageSpec)
		logPath := logPathForImage(vm.ImageSpec)
		config := fmt.Sprintf(`logging:
  receivers:
    files_1:
      type: files
      include_paths: [%s]
      wildcard_refresh_interval: 30s
  processors:
    multiline_parser_1:
      type: parse_multiline
      match_any:
      - type: language_exceptions
        language: go
      - type: language_exceptions
        language: java
      - type: language_exceptions
        language: python
  service:
    pipelines:
      p1:
        receivers: [files_1]
        processors: [multiline_parser_1]`, logPath)

		//Below lines comes from Go, Python and Java exception stacktraces.
		if err := gce.UploadContent(ctx, logger, vm, strings.NewReader(`2019/01/15 07:48:05 http: panic serving [::1]:54143: test panic
goroutine 24 [running]:
net/http.(*conn).serve.func1(0xc00007eaa0)
	/usr/local/go/src/net/http/server.go:1746 +0xd0
panic(0x12472a0, 0x12ece10)
	/usr/local/go/src/runtime/panic.go:513 +0x1b9
main.doPanic(0x12f0ea0, 0xc00010e1c0, 0xc000104400)
	/Users/ingvar/src/go/src/httppanic.go:8 +0x39
net/http.HandlerFunc.ServeHTTP(0x12be2e8, 0x12f0ea0, 0xc00010e1c0, 0xc000104400)
	/usr/local/go/src/net/http/server.go:1964 +0x44
net/http.(*ServeMux).ServeHTTP(0x14a17a0, 0x12f0ea0, 0xc00010e1c0, 0xc000104400)
	/usr/local/go/src/net/http/server.go:2361 +0x127
net/http.serverHandler.ServeHTTP(0xc000085040, 0x12f0ea0, 0xc00010e1c0, 0xc000104400)
	/usr/local/go/src/net/http/server.go:2741 +0xab
net/http.(*conn).serve(0xc00007eaa0, 0x12f10a0, 0xc00008a780)
	/usr/local/go/src/net/http/server.go:1847 +0x646
created by net/http.(*Server).Serve
	/usr/local/go/src/net/http/server.go:2851 +0x2f5
Traceback (most recent call last):
  File "/base/data/home/runtimes/python27/python27_lib/versions/third_party/webapp2-2.5.2/webapp2.py", line 1535, in __call__
    rv = self.handle_exception(request, response, e)
  File "/base/data/home/apps/s~nearfieldspy/1.378705245900539993/nearfieldspy.py", line 17, in start
    return get()
  File "/base/data/home/apps/s~nearfieldspy/1.378705245900539993/nearfieldspy.py", line 5, in get
    raise Exception('spam', 'eggs')
Exception: ('spam', 'eggs')
2023-07-09 00:00:00,000 ERROR    some_app custom string prefix to the exception: Traceback (most recent call last):
  File "/base/data/home/runtimes/python27/python27_lib/versions/third_party/webapp2-2.5.2/webapp2.py", line 1535, in __call__
    rv = self.handle_exception(request, response, e)
  File "/base/data/home/apps/s~nearfieldspy/1.378705245900539993/nearfieldspy.py", line 17, in start
    return get()
  File "/base/data/home/apps/s~nearfieldspy/1.378705245900539993/nearfieldspy.py", line 5, in get
    raise Exception('spam', 'eggs')
Exception: ('spam', 'eggs')
java.lang.RuntimeException: javax.mail.SendFailedException: Invalid Addresses;
  nested exception is:
com.sun.mail.smtp.SMTPAddressFailedException: 550 5.7.1 <[REDACTED_EMAIL_ADDRESS]>... Relaying denied
	at com.nethunt.crm.api.server.adminsync.AutomaticEmailFacade.sendWithSmtp(AutomaticEmailFacade.java:236)
	at com.nethunt.crm.api.server.adminsync.AutomaticEmailFacade.sendSingleEmail(AutomaticEmailFacade.java:285)
	at com.nethunt.crm.api.server.adminsync.AutomaticEmailFacade.lambda$sendSingleEmail$3(AutomaticEmailFacade.java:254)
	at java.util.Optional.ifPresent(Optional.java:159)
	at com.nethunt.crm.api.server.adminsync.AutomaticEmailFacade.sendSingleEmail(AutomaticEmailFacade.java:253)
	at com.nethunt.crm.api.server.adminsync.AutomaticEmailFacade.sendSingleEmail(AutomaticEmailFacade.java:249)
	at com.nethunt.crm.api.email.EmailSender.lambda$notifyPerson$0(EmailSender.java:80)
	at com.nethunt.crm.api.util.ManagedExecutor.lambda$execute$0(ManagedExecutor.java:36)
	at com.nethunt.crm.api.util.RequestContextActivator.lambda$withRequestContext$0(RequestContextActivator.java:36)
	at java.base/java.util.concurrent.ThreadPoolExecutor.runWorker(ThreadPoolExecutor.java:1149)
	at java.base/java.util.concurrent.ThreadPoolExecutor$Worker.run(ThreadPoolExecutor.java:624)
	at java.base/java.lang.Thread.run(Thread.java:748)
Caused by: javax.mail.SendFailedException: Invalid Addresses;
  nested exception is:
com.sun.mail.smtp.SMTPAddressFailedException: 550 5.7.1 <[REDACTED_EMAIL_ADDRESS]>... Relaying denied
	at com.sun.mail.smtp.SMTPTransport.rcptTo(SMTPTransport.java:2064)
	at com.sun.mail.smtp.SMTPTransport.sendMessage(SMTPTransport.java:1286)
	at com.nethunt.crm.api.server.adminsync.AutomaticEmailFacade.sendWithSmtp(AutomaticEmailFacade.java:229)
	... 12 more
Caused by: com.sun.mail.smtp.SMTPAddressFailedException: 550 5.7.1 <[REDACTED_EMAIL_ADDRESS]>... Relaying denied
`), logPath); err != nil {
			t.Fatalf("error writing dummy log lines for Go + Java + Python: %v", err)
		}

		if err := agents.SetupOpsAgent(ctx, logger, vm, config); err != nil {
			t.Fatal(err)
		}

		// 1st one is Golang
		if err := gce.WaitForLog(ctx, logger, vm, "files_1", time.Hour, `jsonPayload.message="2019/01/15 07:48:05 http: panic serving [::1]:54143: test panic\ngoroutine 24 [running]:\nnet/http.(*conn).serve.func1(0xc00007eaa0)\n	/usr/local/go/src/net/http/server.go:1746 +0xd0\npanic(0x12472a0, 0x12ece10)\n	/usr/local/go/src/runtime/panic.go:513 +0x1b9\nmain.doPanic(0x12f0ea0, 0xc00010e1c0, 0xc000104400)\n	/Users/ingvar/src/go/src/httppanic.go:8 +0x39\nnet/http.HandlerFunc.ServeHTTP(0x12be2e8, 0x12f0ea0, 0xc00010e1c0, 0xc000104400)\n	/usr/local/go/src/net/http/server.go:1964 +0x44\nnet/http.(*ServeMux).ServeHTTP(0x14a17a0, 0x12f0ea0, 0xc00010e1c0, 0xc000104400)\n	/usr/local/go/src/net/http/server.go:2361 +0x127\nnet/http.serverHandler.ServeHTTP(0xc000085040, 0x12f0ea0, 0xc00010e1c0, 0xc000104400)\n	/usr/local/go/src/net/http/server.go:2741 +0xab\nnet/http.(*conn).serve(0xc00007eaa0, 0x12f10a0, 0xc00008a780)\n	/usr/local/go/src/net/http/server.go:1847 +0x646\ncreated by net/http.(*Server).Serve\n	/usr/local/go/src/net/http/server.go:2851 +0x2f5\n"`); err != nil {
			t.Error(err)
		}

		// 2nd Python
		if err := gce.WaitForLog(ctx, logger, vm, "files_1", time.Hour, `jsonPayload.message="Traceback (most recent call last):\n  File \"/base/data/home/runtimes/python27/python27_lib/versions/third_party/webapp2-2.5.2/webapp2.py\", line 1535, in __call__\n    rv = self.handle_exception(request, response, e)\n  File \"/base/data/home/apps/s~nearfieldspy/1.378705245900539993/nearfieldspy.py\", line 17, in start\n    return get()\n  File \"/base/data/home/apps/s~nearfieldspy/1.378705245900539993/nearfieldspy.py\", line 5, in get\n    raise Exception('spam', 'eggs')\nException: ('spam', 'eggs')\n"`); err != nil {
			t.Error(err)
		}

		// 3nd Python - With custom string prefix, common when using https://docs.python.org/3/library/logging.html#logging.Logger.exception
		if err := gce.WaitForLog(ctx, logger, vm, "files_1", time.Hour, `jsonPayload.message="2023-07-09 00:00:00,000 ERROR    some_app custom string prefix to the exception: Traceback (most recent call last):\n  File \"/base/data/home/runtimes/python27/python27_lib/versions/third_party/webapp2-2.5.2/webapp2.py\", line 1535, in __call__\n    rv = self.handle_exception(request, response, e)\n  File \"/base/data/home/apps/s~nearfieldspy/1.378705245900539993/nearfieldspy.py\", line 17, in start\n    return get()\n  File \"/base/data/home/apps/s~nearfieldspy/1.378705245900539993/nearfieldspy.py\", line 5, in get\n    raise Exception('spam', 'eggs')\nException: ('spam', 'eggs')\n"`); err != nil {
			t.Error(err)
		}

		// 3rd Java
		if err := gce.WaitForLog(ctx, logger, vm, "files_1", time.Hour, `jsonPayload.message="java.lang.RuntimeException: javax.mail.SendFailedException: Invalid Addresses;\n  nested exception is:\ncom.sun.mail.smtp.SMTPAddressFailedException: 550 5.7.1 <[REDACTED_EMAIL_ADDRESS]>... Relaying denied\n	at com.nethunt.crm.api.server.adminsync.AutomaticEmailFacade.sendWithSmtp(AutomaticEmailFacade.java:236)\n	at com.nethunt.crm.api.server.adminsync.AutomaticEmailFacade.sendSingleEmail(AutomaticEmailFacade.java:285)\n	at com.nethunt.crm.api.server.adminsync.AutomaticEmailFacade.lambda$sendSingleEmail$3(AutomaticEmailFacade.java:254)\n	at java.util.Optional.ifPresent(Optional.java:159)\n	at com.nethunt.crm.api.server.adminsync.AutomaticEmailFacade.sendSingleEmail(AutomaticEmailFacade.java:253)\n	at com.nethunt.crm.api.server.adminsync.AutomaticEmailFacade.sendSingleEmail(AutomaticEmailFacade.java:249)\n	at com.nethunt.crm.api.email.EmailSender.lambda$notifyPerson$0(EmailSender.java:80)\n	at com.nethunt.crm.api.util.ManagedExecutor.lambda$execute$0(ManagedExecutor.java:36)\n	at com.nethunt.crm.api.util.RequestContextActivator.lambda$withRequestContext$0(RequestContextActivator.java:36)\n	at java.base/java.util.concurrent.ThreadPoolExecutor.runWorker(ThreadPoolExecutor.java:1149)\n	at java.base/java.util.concurrent.ThreadPoolExecutor$Worker.run(ThreadPoolExecutor.java:624)\n	at java.base/java.lang.Thread.run(Thread.java:748)\nCaused by: javax.mail.SendFailedException: Invalid Addresses;\n  nested exception is:\ncom.sun.mail.smtp.SMTPAddressFailedException: 550 5.7.1 <[REDACTED_EMAIL_ADDRESS]>... Relaying denied\n	at com.sun.mail.smtp.SMTPTransport.rcptTo(SMTPTransport.java:2064)\n	at com.sun.mail.smtp.SMTPTransport.sendMessage(SMTPTransport.java:1286)\n	at com.nethunt.crm.api.server.adminsync.AutomaticEmailFacade.sendWithSmtp(AutomaticEmailFacade.java:229)\n	... 12 more\nCaused by: com.sun.mail.smtp.SMTPAddressFailedException: 550 5.7.1 <[REDACTED_EMAIL_ADDRESS]>... Relaying denied\n"`); err != nil {
			t.Error(err)
		}
	})
}

func TestParseMultilineFileMissingParser(t *testing.T) {
	t.Parallel()
	gce.RunForEachImage(t, func(t *testing.T, imageSpec string) {
		t.Parallel()
		if gce.IsWindows(imageSpec) {
			t.SkipNow()
		}
		ctx, logger, vm := setupMainLogAndVM(t, imageSpec)
		logPath := logPathForImage(vm.ImageSpec)
		// In the config file, only match for Golang exceptions
		config := fmt.Sprintf(`logging:
  receivers:
    files_1:
      type: files
      include_paths: [%s]
      wildcard_refresh_interval: 30s
  processors:
    multiline_parser_1:
      type: parse_multiline
      match_any:
      - type: language_exceptions
        language: go
  service:
    pipelines:
      p1:
        receivers: [files_1]
        processors: [multiline_parser_1]`, logPath)

		//Below lines comes from Go, Python and Java exception stacktraces.
		if err := gce.UploadContent(ctx, logger, vm, strings.NewReader(`2019/01/15 07:48:05 http: panic serving [::1]:54143: test panic
goroutine 24 [running]:
net/http.(*conn).serve.func1(0xc00007eaa0)
	/usr/local/go/src/net/http/server.go:1746 +0xd0
panic(0x12472a0, 0x12ece10)
	/usr/local/go/src/runtime/panic.go:513 +0x1b9
main.doPanic(0x12f0ea0, 0xc00010e1c0, 0xc000104400)
	/Users/ingvar/src/go/src/httppanic.go:8 +0x39
net/http.HandlerFunc.ServeHTTP(0x12be2e8, 0x12f0ea0, 0xc00010e1c0, 0xc000104400)
	/usr/local/go/src/net/http/server.go:1964 +0x44
net/http.(*ServeMux).ServeHTTP(0x14a17a0, 0x12f0ea0, 0xc00010e1c0, 0xc000104400)
	/usr/local/go/src/net/http/server.go:2361 +0x127
net/http.serverHandler.ServeHTTP(0xc000085040, 0x12f0ea0, 0xc00010e1c0, 0xc000104400)
	/usr/local/go/src/net/http/server.go:2741 +0xab
net/http.(*conn).serve(0xc00007eaa0, 0x12f10a0, 0xc00008a780)
	/usr/local/go/src/net/http/server.go:1847 +0x646
created by net/http.(*Server).Serve
	/usr/local/go/src/net/http/server.go:2851 +0x2f5
Traceback (most recent call last):
  File "/base/data/home/runtimes/python27/python27_lib/versions/third_party/webapp2-2.5.2/webapp2.py", line 1535, in __call__
    rv = self.handle_exception(request, response, e)
  File "/base/data/home/apps/s~nearfieldspy/1.378705245900539993/nearfieldspy.py", line 17, in start
    return get()
  File "/base/data/home/apps/s~nearfieldspy/1.378705245900539993/nearfieldspy.py", line 5, in get
    raise Exception('spam', 'eggs')
Exception: ('spam', 'eggs')
java.lang.RuntimeException: javax.mail.SendFailedException: Invalid Addresses;
  nested exception is:
com.sun.mail.smtp.SMTPAddressFailedException: 550 5.7.1 <[REDACTED_EMAIL_ADDRESS]>... Relaying denied
	at com.nethunt.crm.api.server.adminsync.AutomaticEmailFacade.sendWithSmtp(AutomaticEmailFacade.java:236)
	at com.nethunt.crm.api.server.adminsync.AutomaticEmailFacade.sendSingleEmail(AutomaticEmailFacade.java:285)
	at com.nethunt.crm.api.server.adminsync.AutomaticEmailFacade.lambda$sendSingleEmail$3(AutomaticEmailFacade.java:254)
	at java.util.Optional.ifPresent(Optional.java:159)
	at com.nethunt.crm.api.server.adminsync.AutomaticEmailFacade.sendSingleEmail(AutomaticEmailFacade.java:253)
	at com.nethunt.crm.api.server.adminsync.AutomaticEmailFacade.sendSingleEmail(AutomaticEmailFacade.java:249)
	at com.nethunt.crm.api.email.EmailSender.lambda$notifyPerson$0(EmailSender.java:80)
	at com.nethunt.crm.api.util.ManagedExecutor.lambda$execute$0(ManagedExecutor.java:36)
	at com.nethunt.crm.api.util.RequestContextActivator.lambda$withRequestContext$0(RequestContextActivator.java:36)
	at java.base/java.util.concurrent.ThreadPoolExecutor.runWorker(ThreadPoolExecutor.java:1149)
	at java.base/java.util.concurrent.ThreadPoolExecutor$Worker.run(ThreadPoolExecutor.java:624)
	at java.base/java.lang.Thread.run(Thread.java:748)
Caused by: javax.mail.SendFailedException: Invalid Addresses;
  nested exception is:
com.sun.mail.smtp.SMTPAddressFailedException: 550 5.7.1 <[REDACTED_EMAIL_ADDRESS]>... Relaying denied
	at com.sun.mail.smtp.SMTPTransport.rcptTo(SMTPTransport.java:2064)
	at com.sun.mail.smtp.SMTPTransport.sendMessage(SMTPTransport.java:1286)
	at com.nethunt.crm.api.server.adminsync.AutomaticEmailFacade.sendWithSmtp(AutomaticEmailFacade.java:229)
	... 12 more
Caused by: com.sun.mail.smtp.SMTPAddressFailedException: 550 5.7.1 <[REDACTED_EMAIL_ADDRESS]>... Relaying denied
`), logPath); err != nil {
			t.Fatalf("error writing dummy log lines for Go + Java + Python: %v", err)
		}

		if err := agents.SetupOpsAgent(ctx, logger, vm, config); err != nil {
			t.Fatal(err)
		}

		// 1st one is Golang
		if err := gce.WaitForLog(ctx, logger, vm, "files_1", time.Hour, `jsonPayload.message="2019/01/15 07:48:05 http: panic serving [::1]:54143: test panic\ngoroutine 24 [running]:\nnet/http.(*conn).serve.func1(0xc00007eaa0)\n	/usr/local/go/src/net/http/server.go:1746 +0xd0\npanic(0x12472a0, 0x12ece10)\n	/usr/local/go/src/runtime/panic.go:513 +0x1b9\nmain.doPanic(0x12f0ea0, 0xc00010e1c0, 0xc000104400)\n	/Users/ingvar/src/go/src/httppanic.go:8 +0x39\nnet/http.HandlerFunc.ServeHTTP(0x12be2e8, 0x12f0ea0, 0xc00010e1c0, 0xc000104400)\n	/usr/local/go/src/net/http/server.go:1964 +0x44\nnet/http.(*ServeMux).ServeHTTP(0x14a17a0, 0x12f0ea0, 0xc00010e1c0, 0xc000104400)\n	/usr/local/go/src/net/http/server.go:2361 +0x127\nnet/http.serverHandler.ServeHTTP(0xc000085040, 0x12f0ea0, 0xc00010e1c0, 0xc000104400)\n	/usr/local/go/src/net/http/server.go:2741 +0xab\nnet/http.(*conn).serve(0xc00007eaa0, 0x12f10a0, 0xc00008a780)\n	/usr/local/go/src/net/http/server.go:1847 +0x646\ncreated by net/http.(*Server).Serve\n	/usr/local/go/src/net/http/server.go:2851 +0x2f5\n"`); err != nil {
			t.Error(err)
		}

		// 2nd one is Python - the golang parser will send those lines as single-line logs
		if err := gce.WaitForLog(ctx, logger, vm, "files_1", time.Hour, `jsonPayload.message="Traceback (most recent call last):\n"`); err != nil {
			t.Error(err)
		}

		if err := gce.WaitForLog(ctx, logger, vm, "files_1", time.Hour, `jsonPayload.message="    raise Exception('spam', 'eggs')\n"`); err != nil {
			t.Error(err)
		}

		// 3rd one is Java - the golang parser will send those lines as single-line logs
		if err := gce.WaitForLog(ctx, logger, vm, "files_1", time.Hour, `jsonPayload.message="java.lang.RuntimeException: javax.mail.SendFailedException: Invalid Addresses;\n"`); err != nil {
			t.Error(err)
		}

		if err := gce.WaitForLog(ctx, logger, vm, "files_1", time.Hour, `jsonPayload.message="Caused by: com.sun.mail.smtp.SMTPAddressFailedException: 550 5.7.1 <[REDACTED_EMAIL_ADDRESS]>... Relaying denied\n"`); err != nil {
			t.Error(err)
		}
	})
}

func TestCustomLogFile(t *testing.T) {
	t.Parallel()
	gce.RunForEachImage(t, func(t *testing.T, imageSpec string) {
		t.Parallel()
		ctx, logger, vm := setupMainLogAndVM(t, imageSpec)
		logPath := logPathForImage(vm.ImageSpec)
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
      - jsonPayload.missing_field = "value"
      - jsonPayload.message =~ "test pattern"
  service:
    pipelines:
      my_pipeline:
        receivers: [mylog_source]
        processors: [my_exclude]
        exporters: [google]
`, logPath)

		if err := agents.SetupOpsAgent(ctx, logger, vm, config); err != nil {
			t.Fatal(err)
		}

		if err := gce.UploadContent(ctx, logger, vm, strings.NewReader("abc test pattern xyz\n7654321\n"), logPath); err != nil {
			t.Fatalf("error writing dummy log line: %v", err)
		}

		if err := gce.WaitForLog(ctx, logger, vm, "mylog_source", time.Hour, "jsonPayload.message=7654321"); err != nil {
			t.Error(err)
		}
		time.Sleep(60 * time.Second)
		_, err := gce.QueryLog(ctx, logger, vm, "mylog_source", time.Hour, `jsonPayload.message="abc test pattern xyz"`, 5)
		if err == nil {
			t.Error("expected log to be excluded but was included")
		} else if !strings.Contains(err.Error(), "not found, exhausted retries") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestCustomLogFormat(t *testing.T) {
	t.Parallel()
	gce.RunForEachImage(t, func(t *testing.T, imageSpec string) {
		t.Parallel()
		ctx, logger, vm := setupMainLogAndVM(t, imageSpec)

		logPath := logPathForImage(vm.ImageSpec)
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

		if err := agents.SetupOpsAgent(ctx, logger, vm, config); err != nil {
			t.Fatal(err)
		}

		zone := time.FixedZone("UTC-8", int((-8 * time.Hour).Seconds()))
		line := fmt.Sprintf("<13>1 %s %s my_app_id - - - qqqqrrrr\n", time.Now().In(zone).Format(time.RFC3339Nano), vm.Name)
		if err := gce.UploadContent(ctx, logger, vm, strings.NewReader(line), logPath); err != nil {
			t.Fatalf("error writing dummy log line: %v", err)
		}

		// window (1 hour) is *less than* the time zone UTC offset (8 hours) to catch time zone parse failures
		if err := gce.WaitForLog(ctx, logger, vm, "mylog_source", time.Hour, "jsonPayload.message=qqqqrrrr AND jsonPayload.ident=my_app_id"); err != nil {
			t.Error(err)
		}
	})
}

func TestHTTPRequestLog(t *testing.T) {
	t.Parallel()

	gce.RunForEachImage(t, func(t *testing.T, imageSpec string) {
		t.Parallel()
		ctx, logger, vm := setupMainLogAndVM(t, imageSpec)
		logPath := logPathForImage(vm.ImageSpec)
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
  service:
    pipelines:
      my_pipeline:
        receivers: [mylog_source]
        processors: [json1]
        exporters: [google]`, logPath)

		if err := agents.SetupOpsAgent(ctx, logger, vm, config); err != nil {
			t.Fatal(err)
		}

		// The HTTP request data that will be used in each log
		httpRequestBody := map[string]interface{}{
			"requestMethod": "GET",
			"requestUrl":    "https://cool.site.net",
			"status":        200,
		}

		// Log with HTTP request data nested under "logging.googleapis.com/httpRequest".
		const newHTTPRequestKey = "logging.googleapis.com/httpRequest"
		const newHTTPRequestLogId = "new_request_log"
		newLogBody := map[string]interface{}{
			"logId":           newHTTPRequestLogId,
			newHTTPRequestKey: httpRequestBody,
		}
		newLogBytes, err := json.Marshal(newLogBody)
		if err != nil {
			t.Fatalf("could not marshal new test log: %v", err)
		}

		// Log with HTTP request data nested under "logging.googleapis.com/http_request".
		const oldHTTPRequestKey = "logging.googleapis.com/http_request"
		const oldHTTPRequestLogId = "old_request_log"
		oldLogBody := map[string]interface{}{
			"logId":           oldHTTPRequestLogId,
			oldHTTPRequestKey: httpRequestBody,
		}
		oldLogBytes, err := json.Marshal(oldLogBody)
		if err != nil {
			t.Fatalf("could not marshal old test log: %v", err)
		}

		// Write both logs to log source file at the same time.
		err = gce.UploadContent(
			ctx,
			logger,
			vm,
			strings.NewReader(fmt.Sprintf("%s\n%s\n", string(newLogBytes), string(oldLogBytes))),
			logPath)
		if err != nil {
			t.Fatalf("error writing log line: %v", err)
		}

		queryLogById := func(logId string) (*cloudlogging.Entry, error) {
			return gce.QueryLog(
				ctx,
				logger,
				vm,
				"mylog_source",
				time.Hour,
				fmt.Sprintf("jsonPayload.logId=%q", logId),
				gce.QueryMaxAttempts)
		}

		isKeyInPayload := func(httpRequestKey string, entry *cloudlogging.Entry) bool {
			payload := entry.Payload.(*structpb.Struct)
			for k := range payload.GetFields() {
				if k == httpRequestKey {
					return true
				}
			}
			return false
		}

		// Test that the new documented field, "logging.googleapis.com/httpRequest", will be
		// parsed as expected by Fluent Bit.
		t.Run("parse new HTTPRequest key", func(t *testing.T) {
			t.Parallel()
			entry, err := queryLogById(newHTTPRequestLogId)
			if err != nil {
				t.Fatalf("could not find written log with id %s: %v", newHTTPRequestLogId, err)
			}
			if entry.HTTPRequest == nil {
				t.Fatal("expected log entry to have HTTPRequest field")
			}
			if isKeyInPayload(newHTTPRequestKey, entry) {
				t.Fatalf("expected %s key to be stripped out of the payload", newHTTPRequestKey)
			}
		})

		// Test that the old field, "logging.googleapis.com/http_request", is no longer
		// parsed by Fluent Bit.
		t.Run("don't parse old HTTPRequest key", func(t *testing.T) {
			t.Parallel()
			entry, err := queryLogById(oldHTTPRequestLogId)
			if err != nil {
				t.Fatalf("could not find written log with id %s: %v", oldHTTPRequestLogId, err)
			}
			if entry.HTTPRequest != nil {
				t.Fatal("expected log entry not to have HTTPRequest field")
			}
			if !isKeyInPayload(oldHTTPRequestKey, entry) {
				t.Fatalf("expected %s key to be present in payload", oldHTTPRequestKey)
			}
		})
	})
}

func TestLogEntrySpecialFields(t *testing.T) {
	t.Parallel()
	gce.RunForEachImage(t, func(t *testing.T, imageSpec string) {
		t.Parallel()
		ctx, logger, vm := agents.CommonSetup(t, imageSpec)
		file1 := fmt.Sprintf("%s_1", logPathForImage(vm.ImageSpec))
		configStr := `
logging:
  receivers:
    f1:
      type: files
      include_paths:
        - %s
  processors:
    json:
      type: parse_json
  service:
    pipelines:
      p1:
        receivers:
          - f1
        processors:
          - json
`
		config := fmt.Sprintf(configStr, file1)
		if err := agents.SetupOpsAgent(ctx, logger.ToMainLog(), vm, config); err != nil {
			t.Fatal(err)
		}

		// httpRequest field is tested by TestHTTPRequestLog(); covers the rest
		// of the specfial fields here
		line := `{"logging.googleapis.com/severity": "WARNING", ` +
			`"logging.googleapis.com/labels": {"label1":"value1", "label2":"value2"}, ` +
			`"logging.googleapis.com/operation": {"id": "id", "producer": "producer", "first": true, "last": true}, ` +
			`"logging.googleapis.com/sourceLocation": {"file": "file", "line": "1", "function": "function"}, ` +
			`"logging.googleapis.com/trace":"trace", ` +
			`"logging.googleapis.com/spanId":"spanId", ` +
			`"normal_field": "value"}` + "\n"
		if err := gce.UploadContent(ctx, logger.ToMainLog(), vm, strings.NewReader(line), file1); err != nil {
			t.Fatalf("error uploading log: %v", err)
		}

		// Expect to see the log with the special fields to be placed to the top
		// level of the LogEntry and the rest to jsonPayload
		if err := gce.WaitForLog(ctx, logger.ToMainLog(), vm, "f1", time.Hour,
			`severity="WARNING" AND `+
				`labels.label1="value1" AND labels.label2="value2" AND `+
				`operation.id="id" AND operation.producer="producer" AND operation.first=true AND operation.last=true AND `+
				`sourceLocation.file="file" AND sourceLocation.line="1" AND sourceLocation.function="function" AND `+
				`trace="trace" AND `+
				`spanId="spanId" AND `+
				`jsonPayload.normal_field="value"`); err != nil {
			t.Error(err)
		}
	})
}

func TestInvalidConfig(t *testing.T) {
	t.Parallel()
	gce.RunForEachImage(t, func(t *testing.T, imageSpec string) {
		t.Parallel()
		ctx, logger, vm := setupMainLogAndVM(t, imageSpec)

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
		if err := agents.SetupOpsAgent(ctx, logger, vm, config); err == nil {
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
	gce.RunForEachImage(t, func(t *testing.T, imageSpec string) {
		t.Parallel()
		ctx, logger, vm := setupMainLogAndVM(t, imageSpec)

		logPath := logPathForImage(vm.ImageSpec)
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

		if err := agents.SetupOpsAgent(ctx, logger, vm, config); err != nil {
			t.Fatal(err)
		}

		// When not using UTC timestamps, the parsing with "%Y-%m-%dT%H:%M:%S.%L%z" doesn't work
		// correctly in windows (b/218888265).
		line := fmt.Sprintf(`{"log":"{\"level\":\"info\",\"message\":\"start\",\"overwritten\":\"yes\"}\n","time":"%s","preserved":"yes","overwritten":"no"}`, time.Now().UTC().Format(time.RFC3339Nano)) + "\n"
		if err := gce.UploadContent(ctx, logger, vm, strings.NewReader(line), logPath); err != nil {
			t.Fatalf("error writing dummy log line: %v", err)
		}

		entry, err := gce.QueryLog(ctx, logger, vm, "mylog_source", time.Hour, "", gce.QueryMaxAttempts)
		if err != nil {
			t.Fatal(err)
		}

		want := &structpb.Struct{Fields: map[string]*structpb.Value{
			"level":       {Kind: &structpb.Value_StringValue{StringValue: "info"}},
			"message":     {Kind: &structpb.Value_StringValue{StringValue: "start"}},
			"preserved":   {Kind: &structpb.Value_StringValue{StringValue: "yes"}},
			"overwritten": {Kind: &structpb.Value_StringValue{StringValue: "yes"}},
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
	gce.RunForEachImage(t, func(t *testing.T, imageSpec string) {
		t.Parallel()
		if gce.IsWindows(imageSpec) {
			t.SkipNow()
		}
		ctx, logger, vm := setupMainLogAndVM(t, imageSpec)

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

		if err := agents.SetupOpsAgent(ctx, logger, vm, config); err != nil {
			t.Fatal(err)
		}

		// By writing the inclusion message last, we increase the likelihood that once the inclusion message is detected, that the
		// exclusion message would have already made it through the backend -- in other words, this increases the likelihood of
		// detecting a failure if the exclusion message were to actually be included.

		// Write test message for exclusion using the program called logger.
		if _, err := gce.RunRemotely(ctx, logger, vm, "logger -n 0.0.0.0 --tcp --port=5140 -- abc test pattern xyz"); err != nil {
			t.Fatalf("Error writing dummy log line: %v", err)
		}
		// Write test message for inclusion.
		if _, err := gce.RunRemotely(ctx, logger, vm, "logger -n 0.0.0.0 --tcp --port=5140 -- abcdefg"); err != nil {
			t.Fatalf("Error writing dummy log line: %v", err)
		}

		if err := gce.WaitForLog(ctx, logger, vm, "mylog_source", time.Hour, "jsonPayload.message:abcdefg"); err != nil {
			t.Error(err)
		}
		time.Sleep(60 * time.Second)
		_, err := gce.QueryLog(ctx, logger, vm, "mylog_source", time.Hour, `jsonPayload.message:"test pattern"`, 5)
		if err == nil {
			t.Error("expected log to be excluded but was included")
		} else if !strings.Contains(err.Error(), "not found, exhausted retries") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestSyslogUDP(t *testing.T) {
	t.Parallel()
	gce.RunForEachImage(t, func(t *testing.T, imageSpec string) {
		t.Parallel()
		if gce.IsWindows(imageSpec) {
			t.SkipNow()
		}
		ctx, logger, vm := setupMainLogAndVM(t, imageSpec)

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

		if err := agents.SetupOpsAgent(ctx, logger, vm, config); err != nil {
			t.Fatal(err)
		}

		// Write "abcdefg" using the program called logger.
		if _, err := gce.RunRemotely(ctx, logger, vm, "logger -n 0.0.0.0 --udp --port=5140 -- abcdefg"); err != nil {
			t.Fatalf("Error writing dummy log line: %v", err)
		}

		if err := gce.WaitForLog(ctx, logger, vm, "mylog_source", time.Hour, "jsonPayload.message:abcdefg"); err != nil {
			t.Error(err)
		}
	})
}

func TestExcludeLogs(t *testing.T) {
	t.Parallel()
	gce.RunForEachImage(t, func(t *testing.T, imageSpec string) {
		t.Parallel()
		ctx, logger, vm := setupMainLogAndVM(t, imageSpec)
		file1 := fmt.Sprintf("%s_1", logPathForImage(vm.ImageSpec))
		file2 := fmt.Sprintf("%s_2", logPathForImage(vm.ImageSpec))

		// p1: validate the contains operator
		// p2: validate that a rule which doesn't exclude a log but still matches
		//     an individual regex pattern cleans up the temporary __match fields
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
    exclude1:
      type: exclude_logs
      match_any:
      - 'jsonPayload.field: "pattern"'
    exclude2:
      type: exclude_logs
      match_any:
      - 'jsonPayload.field1 = "first" AND jsonPayload.field2 =~ "second"'
    json:
      type: parse_json
  service:
    pipelines:
      p1:
        receivers: [f1]
        processors: [json, exclude1]
      p2:
        receivers: [f2]
        processors: [json, exclude2]
`, file1, file2)

		if err := agents.SetupOpsAgent(ctx, logger, vm, config); err != nil {
			t.Fatal(err)
		}

		logContents1 := `{"field":"string containing pattern"}` + "\n"
		logContents1 += `{"field":"other"}` + "\n"
		if err := gce.UploadContent(ctx, logger, vm, strings.NewReader(logContents1), file1); err != nil {
			t.Fatalf("error uploading log: %v", err)
		}

		logContents2 := `{"field1":"nope, include me!", "field2":"second"}` + "\n"
		if err := gce.UploadContent(ctx, logger, vm, strings.NewReader(logContents2), file2); err != nil {
			t.Fatalf("error uploading log: %v", err)
		}

		// p1: Expect to see the log that doesn't have pattern in it.
		if err := gce.WaitForLog(ctx, logger, vm, "f1", time.Hour, `jsonPayload.field:"other"`); err != nil {
			t.Error(err)
		}
		// p1: Give the excluded log some time to show up.
		time.Sleep(60 * time.Second)
		_, err := gce.QueryLog(ctx, logger, vm, "f1", time.Hour, `jsonPayload.field:"pattern"`, 5)
		if err == nil {
			t.Error("expected log to be excluded but was included")
		} else if !strings.Contains(err.Error(), "not found, exhausted retries") {
			t.Fatalf("unexpected error: %v", err)
		}

		// p2: Expect to see the log.
		resultingLog2, err := gce.QueryLog(ctx, logger, vm, "f2", time.Hour, `jsonPayload.field1:*`, gce.QueryMaxAttempts)
		if err != nil {
			t.Error(err)
		}
		// p2: Verify that there are no vestigial __match_ fields.
		payload := resultingLog2.Payload.(*structpb.Struct)
		for k := range payload.GetFields() {
			if strings.HasPrefix(k, "__match_") {
				t.Errorf("unexpected vestigial field: %s", k)
			}
		}
	})
}

func TestExcludeLogsParseJsonOrder(t *testing.T) {
	t.Parallel()
	gce.RunForEachImage(t, func(t *testing.T, imageSpec string) {
		t.Parallel()
		ctx, logger, vm := setupMainLogAndVM(t, imageSpec)
		file1 := fmt.Sprintf("%s_1", logPathForImage(vm.ImageSpec))
		file2 := fmt.Sprintf("%s_2", logPathForImage(vm.ImageSpec))

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

		if err := agents.SetupOpsAgent(ctx, logger, vm, config); err != nil {
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
		if err := gce.WaitForLog(ctx, logger, vm, "f1", time.Hour, `jsonPayload.field="value"`); err != nil {
			t.Error(err)
		}
		// Give the excluded log some time to show up.
		time.Sleep(60 * time.Second)
		_, err := gce.QueryLog(ctx, logger, vm, "f2", time.Hour, `jsonPayload.field="value"`, 5)
		if err == nil {
			t.Error("expected log to be excluded but was included")
		} else if !strings.Contains(err.Error(), "not found, exhausted retries") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestExcludeLogsModifyFieldsOrder(t *testing.T) {
	t.Parallel()
	gce.RunForEachImage(t, func(t *testing.T, imageSpec string) {
		t.Parallel()
		ctx, logger, vm := agents.CommonSetup(t, imageSpec)
		file1 := fmt.Sprintf("%s_1", logPathForImage(vm.ImageSpec))
		file2 := fmt.Sprintf("%s_2", logPathForImage(vm.ImageSpec))
		file3 := fmt.Sprintf("%s_3", logPathForImage(vm.ImageSpec))

		// This exclude_logs processor operates on a non-default field which is
		// present if and only if the log is structured accordingly.
		// The intended mechanism for inputting structured logs from a file is
		// to use a parse_json processor, and the processor will automatically
		// recongnize special fields for the LogEntry and place them at the top
		// level and can be used by the exclude_logs processor to filter logs.
		// (pipeline p1)
		// For top level fields that set by modify_fields, since processors
		// operate in the order in which they're written, the expectation is
		// that if a modify_fields processor comes before the exclude_logs
		// processor then the log is excluded. (pipeline p2)
		// Conversely, if a modify_fields processor comes after the exclude_logs
		// processor then the log is not excluded. (pipeline p3)
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
    f3:
      type: files
      include_paths:
      - %s
  processors:
    exclude_trace:
      type: exclude_logs
      match_any:
      - trace =~ "value1"
    exclude_span_id:
      type: exclude_logs
      match_any:
      - spanId =~ "value2"
    modify:
      type: modify_fields
      fields:
        trace:
          move_from: jsonPayload.none
          default_value: value1
    json:
      type: parse_json
  service:
    pipelines:
      p1:
        receivers: [f1]
        processors: [json, exclude_span_id]
      p2:
        receivers: [f2]
        processors: [json, modify, exclude_trace]
      p3:
        receivers: [f3]
        processors: [json, exclude_trace, modify]
`, file1, file2, file3)

		if err := agents.SetupOpsAgent(ctx, logger.ToMainLog(), vm, config); err != nil {
			t.Fatal(err)
		}

		line := `{"logging.googleapis.com/spanId":"value2", "query_field": "value"}` + "\n"
		for _, file := range []string{file1, file2, file3} {
			if err := gce.UploadContent(ctx, logger.ToMainLog(), vm, strings.NewReader(line), file); err != nil {
				t.Fatalf("error uploading log: %v", err)
			}
		}

		// Expect to see the log included in p3 but not p1 and p2.
		if err := gce.WaitForLog(ctx, logger.ToMainLog(), vm, "f3", time.Hour, `jsonPayload.query_field="value"`); err != nil {
			t.Error(err)
		}
		// Give the excluded log some time to show up.
		time.Sleep(60 * time.Second)
		for _, name := range []string{"f1", "f2"} {
			_, err := gce.QueryLog(ctx, logger.ToMainLog(), vm, name, time.Hour, `jsonPayload.query_field="value"`, 5)
			if err == nil {
				t.Error("expected log to be excluded but was included")
			} else if !strings.Contains(err.Error(), "not found, exhausted retries") {
				t.Fatalf("unexpected error: %v", err)
			}
		}
	})
}

func TestModifyFields(t *testing.T) {
	t.Parallel()
	gce.RunForEachImage(t, func(t *testing.T, imageSpec string) {
		t.Parallel()
		ctx, logger, vm := setupMainLogAndVM(t, imageSpec)
		file1 := fmt.Sprintf("%s_1", logPathForImage(vm.ImageSpec))

		config := fmt.Sprintf(`logging:
  receivers:
    f1:
      type: files
      include_paths:
      - %s
  processors:
    modify:
      type: modify_fields
      fields:
        labels."my.cool.service/foo":
          copy_from: jsonPayload.field
        labels."static":
          static_value: hello world
        labels."label2":
          move_from: labels."label1"
        severity:
          static_value: WARNING
        jsonPayload.field2:
          move_from: jsonPayload.field
          omit_if: jsonPayload.missing_field = "present"
        jsonPayload.default_present:
          default_value: default
        jsonPayload.default_absent:
          default_value: default
        jsonPayload.integer:
          static_value: 15
          type: integer
        jsonPayload.float:
          static_value: 10.5
          type: float
        jsonPayload.mapped_field:
          copy_from: jsonPayload.field
          map_values:
            value: new_value
            value2: wrong_value
        jsonPayload.omitted:
          static_value: broken
          omit_if: jsonPayload.field = "value"
        trace:
          move_from: jsonPayload.trace
        spanId:
          copy_from: jsonPayload.spanId
    json:
      type: parse_json
  exporters:
    google:
      type: google_cloud_logging
  service:
    pipelines:
      p1:
        receivers: [f1]
        processors: [json, modify]
        exporters: [google]
`, file1)

		if err := agents.SetupOpsAgent(ctx, logger, vm, config); err != nil {
			t.Fatal(err)
		}

		line := `{"field":"value", "default_present":"original", "logging.googleapis.com/labels": {"label1":"value"}, "trace":"trace_value", "spanId": "span_id_value"}` + "\n"
		if err := gce.UploadContent(ctx, logger, vm, strings.NewReader(line), file1); err != nil {
			t.Fatalf("error uploading log: %v", err)
		}

		// Expect to see the log with the modifications applied
		if err := gce.WaitForLog(ctx, logger, vm, "f1", time.Hour, `jsonPayload.field2="value" AND labels.static="hello world" AND labels.label2="value" AND NOT labels.label1:* AND labels."my.cool.service/foo"="value" AND severity="WARNING" AND NOT jsonPayload.field:* AND jsonPayload.default_present="original" AND jsonPayload.default_absent="default" AND jsonPayload.integer > 5 AND jsonPayload.float > 5 AND jsonPayload.mapped_field="new_value" AND (NOT jsonPayload.omitted = "broken") AND trace = "trace_value" AND NOT jsonPayload.trace:* AND spanId = "span_id_value" AND jsonPayload.spanId = "span_id_value"`); err != nil {
			t.Error(err)
		}
	})
}

func TestParseWithConflictsWithRecord(t *testing.T) {
	t.Parallel()
	gce.RunForEachImage(t, func(t *testing.T, imageSpec string) {
		t.Parallel()
		ctx, logger, vm := setupMainLogAndVM(t, imageSpec)
		file1 := fmt.Sprintf("%s_1", logPathForImage(vm.ImageSpec))
		configStr := `
logging:
  receivers:
    f1:
      type: files
      include_paths:
        - %s
  processors:
    modify:
      type: modify_fields
      fields:
        labels."non-overwritten-label":
          static_value: non-overwritten
        labels."overwritten-label":
          static_value: non-overwritten
        labels."original-label":
          static_value: original-label
        severity:
          static_value: WARNING
        sourceLocation.file:
          static_value: non-overwritten-file-path
        jsonPayload."non-overwritten-field":
          static_value: non-overwritten
        jsonPayload."overwritten-field":
          static_value: non-overwritten
        jsonPayload."original-field":
          static_value: original-value
    json:
      type: parse_json
  exporters:
    google:
      type: google_cloud_logging
  service:
    pipelines:
      p1:
        receivers:
          - f1
        processors:
          - modify
          - json
        exporters:
          - google
`
		config := fmt.Sprintf(configStr, file1)
		if err := agents.SetupOpsAgent(ctx, logger, vm, config); err != nil {
			t.Fatal(err)
		}

		line := `{"parsed-field":"parsed-value", "overwritten-field":"overwritten", "logging.googleapis.com/labels": {"parsed-label":"parsed-label", "overwritten-label":"overwritten"}, "logging.googleapis.com/sourceLocation": {"file": "overwritten-file-path"}}` + "\n"
		if err := gce.UploadContent(ctx, logger, vm, strings.NewReader(line), file1); err != nil {
			t.Fatalf("error uploading log: %v", err)
		}

		// Expect to see the log with the modifications applied
		if err := gce.WaitForLog(ctx, logger, vm, "f1", time.Hour,
			`jsonPayload.original-field="original-value" AND jsonPayload.parsed-field="parsed-value" AND jsonPayload.non-overwritten-field="non-overwritten" AND jsonPayload.overwritten-field="overwritten" AND labels.original-label="original-label" AND labels.parsed-label="parsed-label" AND labels.non-overwritten-label="non-overwritten" AND labels.overwritten-label="overwritten" AND severity="WARNING" AND sourceLocation.file="overwritten-file-path"`); err != nil {
			t.Error(err)
		}
	})
}

func TestResourceNameLabel(t *testing.T) {
	t.Parallel()
	gce.RunForEachImage(t, func(t *testing.T, imageSpec string) {
		t.Parallel()
		ctx, logger, vm := setupMainLogAndVM(t, imageSpec)
		file1 := fmt.Sprintf("%s_1", logPathForImage(vm.ImageSpec))

		config := fmt.Sprintf(`logging:
  receivers:
    f1:
      type: files
      include_paths:
      - %s
  processors:
    json:
      type: parse_json
  service:
    pipelines:
      p1:
        receivers: [f1]
        processors: [json]
`, file1)

		if err := agents.SetupOpsAgent(ctx, logger, vm, config); err != nil {
			t.Fatal(err)
		}

		line := `{"default_present":"original"}` + "\n"
		if err := gce.UploadContent(ctx, logger, vm, strings.NewReader(line), file1); err != nil {
			t.Fatalf("error uploading log: %v", err)
		}

		// Expect to see the log with the modifications applied
		check := fmt.Sprintf(`labels."compute.googleapis.com/resource_name"="%s" AND jsonPayload.default_present="original"`, vm.Name)
		if err := gce.WaitForLog(ctx, logger, vm, "f1", time.Hour, check); err != nil {
			t.Error(err)
		}
	})
}

func TestLogFilePathLabel(t *testing.T) {
	t.Parallel()
	t.Run("fluent-bit", func(t *testing.T) {
		testLogFilePathLabel(t, false)
	})
	t.Run("otel", func(t *testing.T) {
		testLogFilePathLabel(t, true)
	})
}

func testLogFilePathLabel(t *testing.T, otel bool) {
	t.Parallel()
	gce.RunForEachImage(t, func(t *testing.T, imageSpec string) {
		t.Parallel()
		ctx, logger, vm := setupMainLogAndVM(t, imageSpec)
		file1 := fmt.Sprintf("%s_1", logPathForImage(vm.ImageSpec))

		config := fmt.Sprintf(`logging:
  receivers:
    f1:
      type: files
      record_log_file_path: true
      include_paths:
      - %s
  processors:
    json:
      type: parse_json
  service:
    experimental_otel_logging: %v
    pipelines:
      p1:
        receivers: [f1]
        processors: [json]
`, file1, otel)

		if otel {
			// Turn on the otel feature gate.
			if err := gce.SetEnvironmentVariables(ctx, logger, vm, map[string]string{"EXPERIMENTAL_FEATURES": "otel_logging"}); err != nil {
				t.Fatal(err)
			}
		}

		if err := agents.SetupOpsAgent(ctx, logger, vm, config); err != nil {
			t.Fatal(err)
		}

		line := `{"default_present":"original"}` + "\n"
		if err := gce.UploadContent(ctx, logger, vm, strings.NewReader(line), file1); err != nil {
			t.Fatalf("error uploading log: %v", err)
		}

		// In Windows the generated log_file_path "C:\mylog_1" uses a backslash.
		// When constructing the query in WaithForLog the backslashes are escaped so
		// replacing with two backslahes correctly queries for "C:\mylog_1" label.
		if gce.IsWindows(imageSpec) {
			file1 = strings.Replace(file1, `\`, `\\`, 1)
		}

		// Expect to see log with label added.
		check := fmt.Sprintf(`labels."agent.googleapis.com/log_file_path"="%s" AND jsonPayload.default_present="original"`, file1)
		if err := gce.WaitForLog(ctx, logger, vm, "f1", time.Hour, check); err != nil {
			t.Error(err)
		}
	})
}

// startFluentBitBackgroundPipe starts a background fluent-bit process that tails a file (whose
// path is returned by this function) and pipes its contents to an output configured via outputConfig.
// An example outputConfig would be "-o tcp://127.0.0.1:5170".
// Use parseInputAsJSON for structured input.
func startFluentBitBackgroundPipe(ctx context.Context, logger *log.Logger, vm *gce.VM, imageSpec string, parseInputAsJSON bool, outputConfig string) (string, error) {
	dir := workDirForImage(imageSpec)
	if err := makeDirectory(ctx, logger, vm, dir); err != nil {
		return "", err
	}
	remoteFile := filepath.Join(dir, "pipe.log")
	parserFile := filepath.Join(dir, "parser.conf")
	parserConfig := `[PARSER]
	Name json
	Format json`

	fluentBitArgs := "-i tail" +
		" -p buffer_chunk_size=512k" +
		" -p buffer_max_size=512k" +
		" -p path=" + remoteFile
	if parseInputAsJSON {
		fluentBitArgs += " -p parser=json" +
			" -R " + parserFile
	}
	fluentBitArgs += " " + outputConfig

	// Create the pipe file, create the parser file, and start fluent-bit in the background.
	// The parser needs to be stored to a file because fluent-bit doesn't support configuring
	// parsers on the command line.
	command := fmt.Sprintf(`
		sudo touch %s
		sudo tee %s > /dev/null <<EOF
%s
EOF
		sudo nohup /opt/google-cloud-ops-agent/subagents/fluent-bit/bin/fluent-bit %s 1>/dev/null 2>/dev/null &`,
		remoteFile,
		parserFile,
		parserConfig,
		// Escape record accessor dollar-signs
		strings.ReplaceAll(fluentBitArgs, "$", `\$`))
	if gce.IsWindows(imageSpec) {
		command = fmt.Sprintf(`
			New-Item %s
			"%s" | Out-File -Encoding Ascii %s
			Invoke-WmiMethod -Class Win32_Process -Name Create -ArgumentList 'C:\Program Files\Google\Cloud Operations\Ops Agent\bin\fluent-bit.exe %s'`,
			remoteFile,
			parserConfig,
			parserFile,
			fluentBitArgs)
	}

	if _, err := gce.RunRemotely(ctx, logger, vm, command); err != nil {
		return "", err
	}
	return remoteFile, nil
}

// writeLinesToRemoteFile writes lines of text to a remote file.
// Lines each have an implicit terminating newline character.
// Lines are allowed to be huge.
func writeLinesToRemoteFile(ctx context.Context, logger *log.Logger, vm *gce.VM, imageSpec string, remoteFile string, lines ...string) error {
	for _, line := range lines {
		line += "\n"

		// Use a temp-file as a buffer to allow for long lines.
		tempPath := filepath.Join(workDirForImage(imageSpec), "pipe_temp.log")
		if err := gce.UploadContent(ctx, logger, vm, strings.NewReader(line), tempPath); err != nil {
			return err
		}

		// Append the temp-file to the remote file.
		appendCommand := fmt.Sprintf(`sudo cat %s | sudo tee -a %s > /dev/null`, tempPath, remoteFile)
		if gce.IsWindows(imageSpec) {
			appendCommand = fmt.Sprintf(`Get-Content %s | Out-File -Encoding Ascii -Append %s`, tempPath, remoteFile)
		}

		if _, err := gce.RunRemotely(ctx, logger, vm, appendCommand); err != nil {
			return err
		}
	}
	return nil
}

func TestTCPLog(t *testing.T) {
	t.Parallel()
	gce.RunForEachImage(t, func(t *testing.T, imageSpec string) {
		t.Parallel()

		ctx, logger, vm := setupMainLogAndVM(t, imageSpec)

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

		if err := agents.SetupOpsAgent(ctx, logger, vm, config); err != nil {
			t.Fatal(err)
		}

		// Start a background fluent-bit that outputs to TCP.
		// The TCP receiver in the Ops Agent already parses to JSON,
		// so don't double-parse it in the background fluent-bit.
		pipePath, err := startFluentBitBackgroundPipe(ctx, logger, vm, imageSpec, false, "-o tcp://127.0.0.1:5170 -p raw_message_key=$log")
		if err != nil {
			t.Fatalf("Error starting fluent-bit background pipe: %v", err)
		}

		linesToWrite := []string{
			// Verify that separate JSON messages are partitioned appropriately,
			// regardless of where the newlines appear between messages.
			`{"msg":"test tcp log 1"}{"msg":"test tcp log 2"}`,
			`{"msg":"test tcp log 3"}{"msg":"test tcp log 4"}`,

			// Verify a large log that's reasonably close to the limit of 256 KB.
			// Use "start" and "end" for querying later because the max query size
			// is only 20 KB.
			fmt.Sprintf(`{"large":"start%send"}`, strings.Repeat("a", 250_000)),
		}
		if err = writeLinesToRemoteFile(ctx, logger, vm, imageSpec, pipePath, linesToWrite...); err != nil {
			t.Fatalf("Error writing dummy TCP log lines: %v", err)
		}

		var waitGroup sync.WaitGroup
		addQueryToWaitGroup := func(query string) {
			waitGroup.Add(1)
			go func() {
				defer waitGroup.Done()
				if err := gce.WaitForLog(ctx, logger, vm, "tcp_logs", time.Hour, query); err != nil {
					t.Error(err)
				}
			}()
		}
		addQueryToWaitGroup(`jsonPayload.msg="test tcp log 1"`)
		addQueryToWaitGroup(`jsonPayload.msg="test tcp log 2"`)
		addQueryToWaitGroup(`jsonPayload.msg="test tcp log 3"`)
		addQueryToWaitGroup(`jsonPayload.msg="test tcp log 4"`)
		addQueryToWaitGroup(`jsonPayload.large:"start" AND jsonPayload.large:"end"`)
		waitGroup.Wait()
	})
}

func TestFluentForwardLog(t *testing.T) {
	t.Parallel()
	gce.RunForEachImage(t, func(t *testing.T, imageSpec string) {
		t.Parallel()

		ctx, logger, vm := setupMainLogAndVM(t, imageSpec)

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
		if err := agents.SetupOpsAgent(ctx, logger, vm, config); err != nil {
			t.Fatal(err)
		}

		// Start a background fluent-forward pipe. We want to verify that
		// the log structure is preserved, so enable JSON parsing.
		var pipePath string
		var err error
		if pipePath, err = startFluentBitBackgroundPipe(ctx, logger, vm, imageSpec, true, "-o forward://127.0.0.1:24224 -t forwarder_tag"); err != nil {
			t.Fatalf("Error starting fluent-bit background pipe: %v", err)
		}

		// Verify a large structured log that's reasonably close to the limit of 256 KB.
		largeLog := fmt.Sprintf(`{"large":"start%send"}`, strings.Repeat("a", 250_000))
		if err = writeLinesToRemoteFile(ctx, logger, vm, imageSpec, pipePath, largeLog); err != nil {
			t.Fatalf("Error writing dummy TCP log line: %v", err)
		}

		if err = gce.WaitForLog(ctx, logger, vm, "fluent_logs.forwarder_tag", time.Hour, `jsonPayload.large:"start" AND jsonPayload.large:"end"`); err != nil {
			t.Error(err)
		}
	})
}

func TestWindowsEventLog(t *testing.T) {
	t.Parallel()
	gce.RunForEachImage(t, func(t *testing.T, imageSpec string) {
		t.Parallel()
		if !gce.IsWindows(imageSpec) {
			t.SkipNow()
		}
		ctx, logger, vm := setupMainLogAndVM(t, imageSpec)

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
		if err := agents.SetupOpsAgent(ctx, logger, vm, config); err != nil {
			t.Fatal(err)
		}

		payloads := map[string]string{
			"Application": "application_msg",
			"System":      "system_msg",
		}
		for log, payload := range payloads {
			if err := writeToWindowsEventLog(ctx, logger, vm, log, payload); err != nil {
				t.Fatal(err)
			}
		}

		for _, payload := range payloads {
			if err := gce.WaitForLog(ctx, logger, vm, "windows_event_log", time.Hour, logMessageQueryForImage(vm.ImageSpec, payload)); err != nil {
				t.Fatal(err)
			}
		}
	})
}

func TestWindowsEventLogV1UnsupportedChannel(t *testing.T) {
	t.Parallel()
	gce.RunForEachImage(t, func(t *testing.T, imageSpec string) {
		t.Parallel()
		if !gce.IsWindows(imageSpec) {
			t.SkipNow()
		}
		ctx, logger, vm := setupMainLogAndVM(t, imageSpec)

		log := "windows_event_log"
		channel := "Microsoft-Windows-User Control Panel/Operational"
		config := fmt.Sprintf(`logging:
  receivers:
    %s:
      type: windows_event_log
      channels:
      - Application
      - %s
  service:
    pipelines:
      default_pipeline:
        receivers: [%s]
`, log, channel, log)
		if err := agents.SetupOpsAgent(ctx, logger, vm, config); err != nil {
			t.Fatal(err)
		}

		// Quote-and-escape the query string so that Cloud Logging accepts it
		expectedWarning := fmt.Sprintf(`"\"channels[1]\" contains a channel, \"%s\", which may not work properly on version 1 of windows_event_log"`, channel)
		if err := gce.WaitForLog(ctx, logger, vm, log, time.Hour, logMessageQueryForImage(vm.ImageSpec, expectedWarning)); err != nil {
			t.Fatal(err)
		}
	})
}

func TestWindowsEventLogV2(t *testing.T) {
	t.Parallel()
	gce.RunForEachImage(t, func(t *testing.T, imageSpec string) {
		t.Parallel()
		if !gce.IsWindows(imageSpec) {
			t.SkipNow()
		}
		ctx, logger, vm := setupMainLogAndVM(t, imageSpec)

		// Have to wait for startup feature tracking metrics to be sent
		// before we tear down the service.
		time.Sleep(20 * time.Second)

		// There is a limitation on custom event log sources that requires their associated
		// log names to have a unique eight-character prefix, so unfortunately we can only test
		// at most one "Microsoft-*" log.
		config := `logging:
  receivers:
    winlog2_space:
      type: windows_event_log
      receiver_version: 2
      channels:
      - Microsoft-Windows-User Profile Service/Operational
    winlog2_default:
      type: windows_event_log
      receiver_version: 2
      channels:
      - Application
      - System
    winlog2_xml:
      type: windows_event_log
      receiver_version: 2
      channels:
      - Application
      - System
      render_as_xml: true
  service:
    pipelines:
      default_pipeline:
        receivers: []
      pipeline_space:
        receivers: [winlog2_space]
      pipeline_v1_v2_simultaneously:
        receivers: [windows_event_log, winlog2_default]
      pipeline_xml:
        receivers: [winlog2_xml]
`
		if err := agents.SetupOpsAgent(ctx, logger, vm, config); err != nil {
			t.Fatal(err)
		}

		// Have to wait for startup feature tracking metrics to be sent
		// before we tear down the service.
		time.Sleep(20 * time.Second)

		payloads := map[string]map[string]string{
			"winlog2_space": {
				"Microsoft-Windows-User Profile Service/Operational": "control_panel_msg",
			},
			"winlog2_default": {
				"Application": "application_msg",
				"System":      "system_msg",
			},
			"winlog2_xml": {
				"Application": "application_xml_msg",
				"System":      "system_xml_msg",
			},
		}
		for r := range payloads {
			for log, payload := range payloads[r] {
				if err := writeToWindowsEventLog(ctx, logger, vm, log, payload); err != nil {
					t.Fatal(err)
				}
			}
		}
		// Manually re-send a log as a Warning to test severity parsing.
		if err := writeToWindowsEventLogWithSeverity(ctx, logger, vm, "Application", "warning_msg", "Warning"); err != nil {
			t.Fatal(err)
		}

		// For winlog2_space, we simply check that the logs were ingested.
		for _, payload := range payloads["winlog2_space"] {
			if err := gce.WaitForLog(ctx, logger, vm, "winlog2_space", time.Hour, logMessageQueryForImage(vm.ImageSpec, payload)); err != nil {
				t.Fatal(err)
			}
		}

		// For pipeline_v1_v2_simultaneously, we expect logs to show up *twice*: once for v1 and once for v2.
		// They can be distinguished by the presence of v1-only and v2-only fields
		// in jsonPayload, e.g. TimeGenerated and TimeCreated respectively.
		for _, payload := range payloads["winlog2_default"] {
			queryV1 := logMessageQueryForImage(vm.ImageSpec, payload) + " AND jsonPayload.TimeGenerated:*"
			queryV2 := logMessageQueryForImage(vm.ImageSpec, payload) + " AND jsonPayload.TimeCreated:*"
			if err := gce.WaitForLog(ctx, logger, vm, "windows_event_log", time.Hour, queryV1); err != nil {
				t.Fatalf("expected v1 log for %s but it wasn't found: err=%v", payload, err)
			}
			if err := gce.WaitForLog(ctx, logger, vm, "winlog2_default", time.Hour, queryV2); err != nil {
				t.Fatalf("expected v2 log for %s but it wasn't found: err=%v", payload, err)
			}
		}

		// Verify that the warning message has the correct severity.
		if err := gce.WaitForLog(ctx, logger, vm, "winlog2_default", time.Hour, logMessageQueryForImage(vm.ImageSpec, "warning_msg")+` AND severity="WARNING"`); err != nil {
			t.Fatal(err)
		}

		// For winlog2_xml, verify the following:
		// - that jsonPayload only has the fields we expect (Message, StringInserts, raw_xml).
		// - that jsonPayload.raw_xml contains a valid XML document.
		// - that a few sample fields are present in that XML document.
		for _, payload := range payloads["winlog2_xml"] {
			log, err := gce.QueryLog(ctx, logger, vm, "winlog2_xml", time.Hour, logMessageQueryForImage(vm.ImageSpec, payload), gce.QueryMaxAttempts)
			if err != nil {
				t.Fatal(err)
			}
			// We don't know (and don't care about) the runtime type graph of log.Payload, so normalize it into a simple map.
			rawJson, err := json.Marshal(log.Payload)
			if err != nil {
				t.Fatal(err)
			}
			jsonMap := map[string]any{}
			err = json.Unmarshal(rawJson, &jsonMap)
			if err != nil {
				t.Fatal(err)
			}
			if len(jsonMap) != 3 ||
				!hasKeyWithValueType[string](jsonMap, "Message") ||
				!hasKeyWithValueType[[]any](jsonMap, "StringInserts") ||
				!hasKeyWithValueType[string](jsonMap, "raw_xml") {
				t.Fatalf("expected exactly 3 fields in jsonPayload (Message, StringInserts, raw_xml): jsonPayload=%+v", jsonMap)
			}
			rawXml := jsonMap["raw_xml"].(string)
			xmlStruct := struct {
				System struct {
					TimeCreated struct {
						SystemTime string `xml:",attr"`
					}
				}
				EventData struct {
					Data string
				}
			}{}
			err = xml.Unmarshal([]byte(rawXml), &xmlStruct)
			if err != nil {
				t.Fatalf("expected raw_xml to contain a valid XML document: raw_xml=%s, err=%v", rawXml, err)
			}
			if xmlStruct.EventData.Data == "" || xmlStruct.System.TimeCreated.SystemTime == "" {
				t.Fatalf("expected raw_xml to contain a few sample fields, but it didn't: raw_xml=%s", rawXml)
			}
		}

		expectedFeatures := []*feature_tracking_metadata.FeatureTracking{
			{
				Module:  "logging",
				Feature: "service:pipelines",
				Key:     "default_pipeline_overridden",
				Value:   "false",
			},
			{
				Module:  "metrics",
				Feature: "service:pipelines",
				Key:     "default_pipeline_overridden",
				Value:   "false",
			},
			{
				Module:  "logging",
				Feature: "service:pipelines",
				Key:     "default_pipeline_overridden",
				Value:   "true",
			},
			{
				Module:  "logging",
				Feature: "receivers:windows_event_log",
				Key:     "[0].enabled",
				Value:   "true",
			},
			{
				Module:  "logging",
				Feature: "receivers:windows_event_log",
				Key:     "[0].receiver_version",
				Value:   "2",
			},
			{
				Module:  "logging",
				Feature: "receivers:windows_event_log",
				Key:     "[0].channels.__length",
				Value:   "2",
			},
			{
				Module:  "logging",
				Feature: "receivers:windows_event_log",
				Key:     "[1].enabled",
				Value:   "true",
			},
			{
				Module:  "logging",
				Feature: "receivers:windows_event_log",
				Key:     "[1].receiver_version",
				Value:   "2",
			},
			{
				Module:  "logging",
				Feature: "receivers:windows_event_log",
				Key:     "[1].channels.__length",
				Value:   "1",
			},
			{
				Module:  "logging",
				Feature: "receivers:windows_event_log",
				Key:     "[2].enabled",
				Value:   "true",
			},
			{
				Module:  "logging",
				Feature: "receivers:windows_event_log",
				Key:     "[2].receiver_version",
				Value:   "2",
			},
			{
				Module:  "logging",
				Feature: "receivers:windows_event_log",
				Key:     "[2].render_as_xml",
				Value:   "true",
			},
			{
				Module:  "logging",
				Feature: "receivers:windows_event_log",
				Key:     "[2].channels.__length",
				Value:   "2",
			},
		}

		series, err := gce.WaitForMetricSeries(ctx, logger, vm, "agent.googleapis.com/agent/internal/ops/feature_tracking", 2*time.Hour, nil, false, len(expectedFeatures))
		if err != nil {
			t.Error(err)
			return
		}

		err = feature_tracking_metadata.AssertFeatureTrackingMetrics(series, expectedFeatures)
		if err != nil {
			t.Error(err)
			return
		}
	})
}

func hasKeyWithValueType[V any](m map[string]any, k string) bool {
	v, okKey := m[k]
	if !okKey {
		return false
	}
	_, okValue := v.(V)
	return okValue
}

func TestWindowsEventLogWithNonDefaultTimeZone(t *testing.T) {
	t.Parallel()
	gce.RunForEachImage(t, func(t *testing.T, imageSpec string) {
		t.Parallel()
		if !gce.IsWindows(imageSpec) {
			t.SkipNow()
		}
		ctx, logger, vm := setupMainLogAndVM(t, imageSpec)
		if _, err := gce.RunRemotely(ctx, logger, vm, `Set-TimeZone -Id "Eastern Standard Time"`); err != nil {
			t.Fatal(err)
		}
		if err := agents.SetupOpsAgent(ctx, logger, vm, ""); err != nil {
			t.Fatal(err)
		}

		// Write an event and record its approximate time
		testMessage := "TestWindowsEventLogWithNonDefaultTimeZone"
		if err := writeToSystemLog(ctx, logger, vm, testMessage); err != nil {
			t.Fatal(err)
		}
		eventTime := time.Now()

		// Validate that the log written to Cloud Logging has a timestamp that's
		// close to eventTime. Use 24*time.Hour to cover all possible time zones.
		logEntry, err := gce.QueryLog(ctx, logger, vm, "windows_event_log", 24*time.Hour, logMessageQueryForImage(imageSpec, testMessage), gce.QueryMaxAttempts)
		if err != nil {
			t.Fatal(err)
		}
		timeDiff := eventTime.Sub(logEntry.Timestamp).Abs()
		if timeDiff.Minutes() > 5 {
			t.Fatalf("timestamp differs by %v minutes", timeDiff.Minutes())
		}
	})
}

func TestSystemdLog(t *testing.T) {
	t.Parallel()
	gce.RunForEachImage(t, func(t *testing.T, imageSpec string) {
		t.Parallel()
		if gce.IsWindows(imageSpec) {
			t.SkipNow()
		}
		ctx, logger, vm := setupMainLogAndVM(t, imageSpec)

		config := `logging:
  receivers:
    systemd_logs:
      type: systemd_journald
  service:
    pipelines:
      systemd_pipeline:
        receivers: [systemd_logs]
`

		if err := agents.SetupOpsAgent(ctx, logger, vm, config); err != nil {
			t.Fatal(err)
		}

		if _, err := gce.RunRemotely(ctx, logger, vm, "echo 'my_systemd_log_message' | systemd-cat"); err != nil {
			t.Fatalf("Error writing dummy Systemd log line: %v", err)
		}

		if err := gce.WaitForLog(ctx, logger, vm, "systemd_logs", time.Hour, "my_systemd_log_message"); err != nil {
			t.Error(err)
		}
	})
}

func TestSystemLogByDefault(t *testing.T) {
	t.Parallel()
	gce.RunForEachImage(t, func(t *testing.T, imageSpec string) {
		t.Parallel()
		ctx, logger, vm := setupMainLogAndVM(t, imageSpec)

		if err := agents.SetupOpsAgent(ctx, logger, vm, ""); err != nil {
			t.Fatal(err)
		}

		if err := writeToSystemLog(ctx, logger, vm, "123456789"); err != nil {
			t.Fatal(err)
		}

		tag := systemLogTagForImage(vm.ImageSpec)
		if err := gce.WaitForLog(ctx, logger, vm, tag, time.Hour, logMessageQueryForImage(vm.ImageSpec, "123456789")); err != nil {
			t.Error(err)
		}
	})
}

func testDefaultMetrics(ctx context.Context, t *testing.T, logger *log.Logger, vm *gce.VM, window time.Duration) {
	if !gce.IsWindows(vm.ImageSpec) {
		// Enable swap file: https://linuxize.com/post/create-a-linux-swap-file/
		// We do this so that swap file metrics will show up.
		_, err := gce.RunRemotely(ctx, logger, vm, strings.Join([]string{
			"sudo dd if=/dev/zero of=/swapfile bs=1024 count=102400",
			"sudo chmod 600 /swapfile",
			"(sudo mkswap /swapfile || sudo /usr/sbin/mkswap /swapfile)",
			"(sudo swapon /swapfile || sudo /usr/sbin/swapon /swapfile)",
		}, " && "))
		if err != nil {
			t.Fatalf("Failed to enable swap file: %v", err)
		}
	}

	agentMetricsMetadata := path.Join("agent_metrics", "metadata.yaml")
	bytes, err := os.ReadFile(agentMetricsMetadata)
	if err != nil {
		t.Fatal(err)
	}

	var agentMetrics metadata.ExpectedMetricsContainer
	err = metadata.UnmarshalAndValidate(agentMetricsMetadata, bytes, &agentMetrics)
	if err != nil {
		t.Fatal(err)
	}

	expectedMetrics := agentMetrics.ExpectedMetrics

	// First make sure that the representative metric is being uploaded.
	for _, metric := range expectedMetrics {
		if !metric.Representative {
			continue
		}

		var series *monitoringpb.TimeSeries
		series, err = gce.WaitForMetric(ctx, logger, vm, metric.Type, window, nil, false)
		if err != nil {
			t.Fatal(err)
		}

		err = metadata.AssertMetric(metric, series)
		if err != nil {
			t.Fatal(err)
		}

	}

	// Now that we've established that the preceding metrics are being uploaded
	// and have percolated through the monitoring backend, let's proceed to
	// query for the rest of the metrics. We used to query for all the metrics
	// at once, but due to the "no metrics yet" retries, this ran us out of
	// quota (b/185363780).
	osKind := gce.OSKind(vm.ImageSpec)
	var metricsWaitGroup sync.WaitGroup
	for _, metric := range expectedMetrics {
		metric := metric

		// Already validated the representative metric
		if metric.Representative {
			continue
		}

		// Don't validate optional metrics
		if metric.Optional {
			continue
		}

		if metric.Platform != "" && metric.Platform != osKind {
			continue
		}

		metricsWaitGroup.Add(1)
		go func() {
			defer metricsWaitGroup.Done()
			series, err := gce.WaitForMetric(ctx, logger, vm, metric.Type, window, nil, false)
			if err != nil {
				t.Error(err)
				return
			}

			err = metadata.AssertMetric(metric, series)
			if err != nil {
				t.Error(err)
			}
		}()
	}
	metricsWaitGroup.Wait()

	featureBytes, err := os.ReadFile(path.Join("agent_metrics", "features.yaml"))
	if err != nil {
		t.Fatal("Could not find features.yaml")
		return
	}

	var fc feature_tracking_metadata.FeatureTrackingContainer

	err = yaml.UnmarshalStrict(featureBytes, &fc)
	if err != nil {
		t.Fatal(err)
	}

	series, err := gce.WaitForMetricSeries(ctx, logger, vm, "agent.googleapis.com/agent/internal/ops/feature_tracking", window, nil, false, len(fc.Features))
	if err != nil {
		t.Error(err)
		return
	}

	err = feature_tracking_metadata.AssertFeatureTrackingMetrics(series, fc.Features)
	if err != nil {
		t.Error(err)
		return
	}
}

func TestDefaultMetricsNoProxy(t *testing.T) {
	t.Parallel()
	gce.RunForEachImage(t, func(t *testing.T, imageSpec string) {
		t.Parallel()

		ctx, logger, vm := setupMainLogAndVM(t, imageSpec)
		if err := agents.SetupOpsAgent(ctx, logger, vm, ""); err != nil {
			t.Fatal(err)
		}

		testDefaultMetrics(ctx, t, logger, vm, time.Hour)
	})
}

// go/sdi-integ-test#proxy-testing
func TestDefaultMetricsWithProxy(t *testing.T) {
	t.Parallel()
	gce.RunForEachImage(t, func(t *testing.T, imageSpec string) {
		t.Parallel()
		if !gce.IsWindows(imageSpec) {
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
		ctx, logger, vm := setupMainLogAndVM(t, imageSpec)
		if err := gce.SetEnvironmentVariables(ctx, logger, vm, settings); err != nil {
			t.Fatal(err)
		}
		if err := agents.SetupOpsAgent(ctx, logger, vm, ""); err != nil {
			t.Fatal(err)
		}

		if err := gce.RemoveExternalIP(ctx, logger, vm); err != nil {
			t.Fatal(err)
		}
		// Sleep for 3 minutes to make sure that if any metrics were sent between agent install and removal of the IP address, then they will fall out of the 2 minute window.
		time.Sleep(3 * time.Minute)
		testDefaultMetrics(ctx, t, logger, vm, 2*time.Minute)
	})
}

func TestPrometheusMetrics(t *testing.T) {
	t.Parallel()
	gce.RunForEachImage(t, func(t *testing.T, imageSpec string) {
		t.Parallel()
		ctx, logger, vm := setupMainLogAndVM(t, imageSpec)

		promConfig := `metrics:
  receivers:
    prometheus:
      type: prometheus
      config:
        scrape_configs:
          - job_name: 'prometheus'
            scrape_interval: 10s
            static_configs:
              - targets: ['localhost:20202']
            relabel_configs:
              - source_labels: [__meta_gce_instance_id]
                regex: '(.+)'
                replacement: '${1}'
                target_label: instance_id
              - source_labels: [__meta_gce_project]
                regex: '(.+)'
                replacement: '${1}'
                target_label: instance_project_id
              - source_labels: [__meta_gce_zone]
                regex: '(.+)'
                replacement: '${1}'
                target_label: zone
              - source_labels: [__meta_gce_tags]
                regex: '(.+)'
                replacement: '${1}'
                target_label: tags
              - source_labels: [__meta_gce_network]
                regex: '(.+)'
                replacement: '${1}'
                target_label: network
              - source_labels: [__meta_gce_subnetwork]
                regex: '(.+)'
                replacement: '${1}'
                target_label: subnetwork
              - source_labels: [__meta_gce_public_ip]
                regex: '(.+)'
                replacement: '${1}'
                target_label: public_ip
              - source_labels: [__meta_gce_private_ip]
                regex: '(.+)'
                replacement: '${1}'
                target_label: private_ip
  service:
    pipelines:
      prometheus_pipeline:
        receivers:
          - prometheus
`

		if err := agents.SetupOpsAgent(ctx, logger, vm, promConfig); err != nil {
			t.Fatal(err)
		}

		// Wait long enough for the data to percolate through the backends
		// under normal circumstances. Based on some experiments, 2 minutes
		// is normal; wait a bit longer to be on the safe side.
		time.Sleep(3 * time.Minute)

		existingMetric := "prometheus.googleapis.com/fluentbit_uptime/counter"
		window := time.Minute
		metric, err := gce.WaitForMetric(ctx, logger, vm, existingMetric, window, nil, true)
		if err != nil {
			t.Fatal(fmt.Errorf("failed to find metric %q in VM %q: %w", existingMetric, vm.Name, err))
		}

		var multiErr error
		metricValueType := metric.ValueType.String()
		metricKind := metric.MetricKind.String()
		metricResource := metric.Resource.Type
		metricLabels := metric.Metric.Labels

		if metricValueType != "DOUBLE" {
			multiErr = multierr.Append(multiErr, fmt.Errorf("metric %q has unexpected value type %q", existingMetric, metricValueType))
		}
		if metricKind != "CUMULATIVE" {
			multiErr = multierr.Append(multiErr, fmt.Errorf("metric %q has unexpected kind %q", existingMetric, metricKind))
		}
		if metricResource != "prometheus_target" {
			multiErr = multierr.Append(multiErr, fmt.Errorf("metric %q has unexpected resource type %q", existingMetric, metricResource))
		}
		if metricLabels["instance_name"] != vm.Name {
			multiErr = multierr.Append(multiErr, fmt.Errorf("metric %q has unexpected instance_name label %q. But expected %q", existingMetric, metricLabels["instance_name"], vm.Name))
		}
		if metricLabels["instance_id"] != fmt.Sprintf("%d", vm.ID) {
			multiErr = multierr.Append(multiErr, fmt.Errorf("metric %q has unexpected instance_id label %q. But expected %q", existingMetric, metricLabels["instance_id"], fmt.Sprintf("%d", vm.ID)))
		}
		expectedMachineType := regexp.MustCompile(fmt.Sprintf("^projects/[0-9]+/machineTypes/%s$", vm.MachineType))
		if !expectedMachineType.MatchString(metricLabels["machine_type"]) {
			multiErr = multierr.Append(multiErr, fmt.Errorf("metric %q has unexpected machine_type label %q. But expected %q", existingMetric, metricLabels["machine_type"], vm.MachineType))
		}
		if metricLabels["instance_project_id"] != vm.Project {
			multiErr = multierr.Append(multiErr, fmt.Errorf("metric %q has unexpected instance_project_id label %q. But expected %q", existingMetric, metricLabels["instance_project_id"], vm.Project))
		}
		if metricLabels["zone"] != vm.Zone {
			multiErr = multierr.Append(multiErr, fmt.Errorf("metric %q has unexpected zone label %q. But expected %q", existingMetric, metricLabels["zone"], vm.Zone))
		}
		expectedNetworkURL := regexp.MustCompile(fmt.Sprintf("^projects/[0-9]+/networks/%s$", vm.Network))
		if !expectedNetworkURL.MatchString(metricLabels["network"]) {
			multiErr = multierr.Append(multiErr, fmt.Errorf("metric %q has unexpected network label %q. But expected %q", existingMetric, metricLabels["network"], vm.Network))
		}
		if metricLabels["public_ip"] != vm.IPAddress && metricLabels["private_ip"] != vm.IPAddress {
			multiErr = multierr.Append(multiErr, fmt.Errorf("metric %q doesn't hace VM IP %q. Public IP %q Private IP %q", existingMetric, vm.IPAddress, metricLabels["public_ip"], metricLabels["private_ip"]))
		}
		if multiErr != nil {
			t.Error(multiErr)
		}

		expectedFeatures := []*feature_tracking_metadata.FeatureTracking{
			{
				Module:  "logging",
				Feature: "service:pipelines",
				Key:     "default_pipeline_overridden",
				Value:   "false",
			},
			{
				Module:  "metrics",
				Feature: "service:pipelines",
				Key:     "default_pipeline_overridden",
				Value:   "false",
			},
			{
				Module:  "metrics",
				Feature: "receivers:prometheus",
				Key:     "[0].enabled",
				Value:   "true",
			},
			{
				Module:  "metrics",
				Feature: "receivers:prometheus",
				Key:     "[0].config.[0].scrape_configs.relabel_configs",
				Value:   "8",
			},
			{
				Module:  "metrics",
				Feature: "receivers:prometheus",
				Key:     "[0].config.[0].scrape_configs.honor_timestamps",
				Value:   "true",
			},
			{
				Module:  "metrics",
				Feature: "receivers:prometheus",
				Key:     "[0].config.[0].scrape_configs.scheme",
				Value:   "http",
			},
			{
				Module:  "metrics",
				Feature: "receivers:prometheus",
				Key:     "[0].config.[0].scrape_configs.scrape_interval",
				Value:   "10s",
			},
			{
				Module:  "metrics",
				Feature: "receivers:prometheus",
				Key:     "[0].config.[0].scrape_configs.scrape_interval",
				Value:   "10s",
			},
			{
				Module:  "metrics",
				Feature: "receivers:prometheus",
				Key:     "[0].config.[0].scrape_configs.static_config_target_groups",
				Value:   "1",
			},
		}

		series, err := gce.WaitForMetricSeries(ctx, logger, vm, "agent.googleapis.com/agent/internal/ops/feature_tracking", time.Hour, nil, false, len(expectedFeatures))
		if err != nil {
			t.Error(err)
			return
		}

		err = feature_tracking_metadata.AssertFeatureTrackingMetrics(series, expectedFeatures)
		if err != nil {
			t.Error(err)
			return
		}
	})
}

func TestPrometheusMetricsWithMetadata(t *testing.T) {
	t.Parallel()
	gce.RunForEachImage(t, func(t *testing.T, imageSpec string) {
		t.Parallel()
		metadataKey, metadataValue := "test", "${test:value}"
		escapedMetadataValue := "_{test:value}"
		ctx, logger, vm := agents.CommonSetupWithExtraCreateArgumentsAndMetadata(t, imageSpec, nil, map[string]string{
			metadataKey: metadataValue,
		})

		promConfig := fmt.Sprintf(`metrics:
  receivers:
    prometheus:
      type: prometheus
      config:
        scrape_configs:
          - job_name: 'prometheus'
            scrape_interval: 10s
            static_configs:
              - targets: ['localhost:20202']
            relabel_configs:
              - source_labels: [__meta_gce_metadata_%s]
                regex: '(.+)'
                replacement: '${1}'
                target_label: %s
  service:
    pipelines:
      prometheus_pipeline:
        receivers:
          - prometheus
`, metadataKey, metadataKey)

		if err := agents.SetupOpsAgent(ctx, logger.ToMainLog(), vm, promConfig); err != nil {
			t.Fatal(err)
		}

		// Wait long enough for the data to percolate through the backends
		// under normal circumstances. Based on some experiments, 2 minutes
		// is normal; wait a bit longer to be on the safe side.
		time.Sleep(3 * time.Minute)

		existingMetric := "prometheus.googleapis.com/fluentbit_uptime/counter"
		window := time.Minute
		metric, err := gce.WaitForMetric(ctx, logger.ToMainLog(), vm, existingMetric, window, nil, true)
		if err != nil {
			t.Fatal(fmt.Errorf("failed to find metric %q in VM %q: %w", existingMetric, vm.Name, err))
		}

		metricLabels := metric.Metric.Labels
		if metricLabels[metadataKey] != escapedMetadataValue {
			t.Errorf("metric %q has VM metadata %q set to %q instead of %q", existingMetric, metadataKey, metricLabels[metadataKey], escapedMetadataValue)
		}
	})
}

// Test the Counter and Gauge metric types using a JSON Prometheus exporter
// The JSON exporter will connect to a http server that serve static JSON files
func TestPrometheusMetricsWithJSONExporter(t *testing.T) {
	t.Parallel()
	gce.RunForEachImage(t, func(t *testing.T, imageSpec string) {
		t.Parallel()
		// TODO: Set up JSON exporter stuff on Windows
		if gce.IsWindows(imageSpec) {
			t.SkipNow()
		}
		ctx, logger, vm := setupMainLogAndVM(t, imageSpec)
		prometheusTestdata := path.Join("testdata", "prometheus")
		filesToUpload := []fileToUpload{
			{
				local:  path.Join(prometheusTestdata, "http_server.go"),
				remote: path.Join("/opt", "go-http-server", "http_server.go"),
			},
			{
				local:  path.Join(prometheusTestdata, "data.json"),
				remote: path.Join("/opt", "go-http-server", "data.json"),
			},
			{
				local:  path.Join(prometheusTestdata, "json_exporter_config.yaml"),
				remote: path.Join("/opt", "json_exporter", "json_exporter_config.yaml"),
			},
			{
				local:  path.Join(prometheusTestdata, "http-server-for-prometheus-test.service"),
				remote: path.Join("/etc", "systemd", "system", "http-server-for-prometheus-test.service"),
			},
			{
				local:  path.Join(prometheusTestdata, "json-exporter-for-prometheus-test.service"),
				remote: path.Join("/etc", "systemd", "system", "json-exporter-for-prometheus-test.service"),
			},
		}

		err := uploadFiles(ctx, logger, vm, testdataDir, filesToUpload)
		if err != nil {
			t.Fatal(err)
		}

		if err := installGolang(ctx, logger, vm); err != nil {
			t.Fatal(err)
		}

		if err := buildGoBinary(ctx, logger, vm, filesToUpload[0].remote, "/opt/go-http-server/"); err != nil {
			t.Fatal(err)
		}

		// Run the setup script to run the http server and the JSON exporter
		setupScript, err := testdataDir.ReadFile(path.Join(prometheusTestdata, "setup_json_exporter.sh"))
		if err != nil {
			t.Fatalf("failed to open setup script: %s", err)
		}
		setupOut, err := gce.RunRemotely(ctx, logger, vm, string(setupScript))
		if err != nil {
			t.Fatalf("failed to run json exporter in VM with err: %v, stderr: %s", err, setupOut.Stderr)
		}
		// Wait until both are ready
		time.Sleep(30 * time.Second)
		liveCheckOut, liveCheckErr := gce.RunRemotely(ctx, logger, vm, `curl "http://localhost:7979/probe?module=default&target=http://localhost:8000/data.json"`)
		// We will abort when:
		// 1. JSON exporter is not started: in this case the stderr will have:
		// "curl: (7) Failed to connect to localhost port 7979 after 1 ms: Connection refused"
		// 2. The HTTP server is not started: in this case the stdout will not
		// have the expected Prometheus style metrics
		if liveCheckErr != nil || strings.Contains(liveCheckOut.Stderr, "Connection refused") || !strings.Contains(liveCheckOut.Stdout, `test_counter_value{test_label="counter_label"} 1234`) {
			t.Fatal("Json Exporter failed to start")
		}
		// Initially, __address__ is the target[0] which is
		// "http://localhost:8000/data.json";
		// The first relabeling moves __address__ to __param_target, so we have
		// "target=http://localhost:8000/data.json" of the final url;
		// The params: module: [default] adds the module=default so we have
		// "module=default&target=http://localhost:8000/data.json"
		// The metric_path: /probe adds that to url:
		// "probe?module=default&target=http://localhost:8000/data.json";
		// And the last relabeling changes the __address__ to localhost:7979:
		// "http://localhost:7979/probe?module=default&target=http://localhost:8000/data.json"
		// which is the URL needed to query metrics hosted by the JSON exporter
		// This is the usual way of using the exporter as this allows us to
		// specify multiple `targets` within one `scrape_configs`
		config := `metrics:
  receivers:
    prom_app:
      type: prometheus
      config:
        scrape_configs:
        - job_name: json
          metrics_path: /probe
          params:
            module: [default]
          static_configs:
            - targets:
              - http://localhost:8000/data.json 
          relabel_configs:
            - source_labels: [__address__]
              target_label: __param_target
              replacement: '$1'
            - source_labels: [__param_target]
              target_label: instance
              replacement: '$1'
            - target_label: __address__
              replacement: localhost:7979 
  service:
    pipelines:
      prom_pipeline:
        receivers: [prom_app]
`
		if err := agents.SetupOpsAgent(ctx, logger, vm, config); err != nil {
			t.Fatal(err)
		}

		// Wait long enough for the data to percolate through the backends
		// under normal circumstances. Based on some experiments, 2 minutes
		// is normal; wait a bit longer to be on the safe side.
		time.Sleep(3 * time.Minute)
		window := time.Minute

		tests := []prometheusMetricTest{
			{"prometheus.googleapis.com/test_gauge_value/gauge", nil,
				metric.MetricDescriptor_GAUGE, metric.MetricDescriptor_DOUBLE, 789.0},
			// Since we are sending the same number at every time point,
			// the cumulative counter metric will return 0 as no change in values
			{"prometheus.googleapis.com/test_counter_value/counter", nil,
				metric.MetricDescriptor_CUMULATIVE, metric.MetricDescriptor_DOUBLE, 0.0},
			// Untyped type - GCM will have untyped metrics double written as a gauge AND a cumulative
			{"prometheus.googleapis.com/test_untyped_value/unknown", nil,
				metric.MetricDescriptor_GAUGE, metric.MetricDescriptor_DOUBLE, 56.0},
			{"prometheus.googleapis.com/test_untyped_value/unknown:counter", nil,
				metric.MetricDescriptor_CUMULATIVE, metric.MetricDescriptor_DOUBLE, 0.0},
		}

		var multiErr error
		for _, test := range tests {
			multiErr = multierr.Append(multiErr, assertPrometheusMetric(ctx, logger, vm, window, test))
		}
		if multiErr != nil {
			t.Error(multiErr)
		}
	})
}

func TestPrometheusRelabelConfigs(t *testing.T) {
	config := `metrics:
  receivers:
    prom_app:
      type: prometheus
      config:
        scrape_configs:
        - job_name: test
          metrics_path: /data
          scrape_interval: 10s
          static_configs:
            - targets:
              - localhost:8000
          metric_relabel_configs:
          - source_labels: [test_label]
            regex: "(.*)@(.*)"
            target_label: destination
            replacement: "${2}/${1}"
  service:
    pipelines:
      prom_pipeline:
        receivers: [prom_app]
`
	prometheusTestdata := path.Join("testdata", "prometheus")
	remoteWorkDir := path.Join("/opt", "go-http-server")
	testChecks := make([]mockPrometheusCheck, 0)
	testChecks = append(testChecks, mockPrometheusCheck{
		fileToUpload: fileToUpload{
			local:  path.Join(prometheusTestdata, "sample_label_replace"),
			remote: path.Join(remoteWorkDir, "data"),
		},
		check: func(ctx context.Context, logger *log.Logger, vm *gce.VM, window time.Duration) error {
			if pts, err := gce.WaitForMetric(ctx, logger, vm, "prometheus.googleapis.com/test_metric/gauge", window, nil, true); err != nil {
				return err
			} else {
				labelValue, ok := pts.Metric.Labels["test_label"]
				if !ok {
					return errors.New("test_label label not found in metric")
				}
				if labelValue != "group/capture" {
					return fmt.Errorf("Expected test_label to be 'group/capture' but got %s", labelValue)
				}
			}
			return nil
		},
	})
	testPrometheusMetrics(t, config, testChecks)
}

func TestPrometheusUntypedMetrics(t *testing.T) {
	config := `metrics:
  receivers:
    prom_app:
      type: prometheus
      config:
        scrape_configs:
        - job_name: test
          metrics_path: /data
          scrape_interval: 10s
          static_configs:
            - targets:
              - localhost:8000
  service:
    pipelines:
      prom_pipeline:
        receivers: [prom_app]
`
	prometheusTestdata := path.Join("testdata", "prometheus")
	remoteWorkDir := path.Join("/opt", "go-http-server")
	testChecks := make([]mockPrometheusCheck, 0)
	testChecks = append(testChecks, mockPrometheusCheck{
		fileToUpload: fileToUpload{
			local:  path.Join(prometheusTestdata, "sample_untyped"),
			remote: path.Join(remoteWorkDir, "data"),
		},
		check: func(ctx context.Context, logger *log.Logger, vm *gce.VM, window time.Duration) error {
			tests := []prometheusMetricTest{
				{"prometheus.googleapis.com/explicit_untyped_metric/unknown", nil,
					metric.MetricDescriptor_GAUGE, metric.MetricDescriptor_DOUBLE, 1.0},
				{"prometheus.googleapis.com/missing_type_hint_metric/unknown", nil,
					metric.MetricDescriptor_GAUGE, metric.MetricDescriptor_DOUBLE, 10.0},
				// Since we are sending the same number at every time point,
				// the cumulative counter metric will return 0 as no change in values
				{"prometheus.googleapis.com/explicit_untyped_metric/unknown:counter", nil,
					metric.MetricDescriptor_CUMULATIVE, metric.MetricDescriptor_DOUBLE, 0.0},
				{"prometheus.googleapis.com/missing_type_hint_metric/unknown:counter", nil,
					metric.MetricDescriptor_CUMULATIVE, metric.MetricDescriptor_DOUBLE, 0.0},
			}

			var multiErr error
			for _, test := range tests {
				multiErr = multierr.Append(multiErr, assertPrometheusMetric(ctx, logger, vm, window, test))
			}
			if multiErr != nil {
				t.Error(multiErr)
			}

			// Ensure that the gauge-casted metric isn't reported when the unknown typed one is.
			if err := gce.AssertMetricMissing(ctx, logger, vm, "prometheus.googleapis.com/explicit_untyped_metric/gauge", true, window); err != nil {
				t.Error(err)
			}
			if err := gce.AssertMetricMissing(ctx, logger, vm, "prometheus.googleapis.com/missing_type_hint_metric/gauge", true, window); err != nil {
				t.Error(err)
			}
			return multiErr
		},
	})
	testPrometheusMetrics(t, config, testChecks)
}

func TestPrometheusUntypedMetricsReset(t *testing.T) {
	config := `metrics:
  receivers:
    prom_app:
      type: prometheus
      config:
        scrape_configs:
        - job_name: test
          metrics_path: /data
          scrape_interval: 10s
          static_configs:
            - targets:
              - localhost:8000
  service:
    pipelines:
      prom_pipeline:
        receivers: [prom_app]
`
	prometheusTestdata := path.Join("testdata", "prometheus")
	remoteWorkDir := path.Join("/opt", "go-http-server")
	testChecks := make([]mockPrometheusCheck, 0)
	testChecks = append(testChecks, mockPrometheusCheck{
		fileToUpload: fileToUpload{
			local:  path.Join(prometheusTestdata, "sample_untyped_step_1"),
			remote: path.Join(remoteWorkDir, "data"),
		},
		check: func(ctx context.Context, logger *log.Logger, vm *gce.VM, window time.Duration) error {
			tests := []prometheusMetricTest{
				{"prometheus.googleapis.com/untyped_metric/unknown", nil,
					metric.MetricDescriptor_GAUGE, metric.MetricDescriptor_DOUBLE, 10.0},
				{"prometheus.googleapis.com/untyped_metric/unknown:counter", nil,
					metric.MetricDescriptor_CUMULATIVE, metric.MetricDescriptor_DOUBLE, 0.0},
			}

			var multiErr error
			for _, test := range tests {
				multiErr = multierr.Append(multiErr, assertPrometheusMetric(ctx, logger, vm, window, test))
			}
			if multiErr != nil {
				t.Error(multiErr)
			}
			return multiErr
		},
	})

	// The cumulative point reports the delta against the first point that isn't sent to Cloud Monitoring.
	testChecks = append(testChecks, mockPrometheusCheck{
		fileToUpload: fileToUpload{
			local:  path.Join(prometheusTestdata, "sample_untyped_step_2"),
			remote: path.Join(remoteWorkDir, "data"),
		},
		check: func(ctx context.Context, logger *log.Logger, vm *gce.VM, window time.Duration) error {
			tests := []prometheusMetricTest{
				{"prometheus.googleapis.com/untyped_metric/unknown", nil,
					metric.MetricDescriptor_GAUGE, metric.MetricDescriptor_DOUBLE, 100.0},
				{"prometheus.googleapis.com/untyped_metric/unknown:counter", nil,
					metric.MetricDescriptor_CUMULATIVE, metric.MetricDescriptor_DOUBLE, 90.0},
			}

			var multiErr error
			for _, test := range tests {
				multiErr = multierr.Append(multiErr, assertPrometheusMetric(ctx, logger, vm, window, test))
			}
			if multiErr != nil {
				t.Error(multiErr)
			}
			return multiErr
		},
	})

	// Once a lower value is reported for the timeseries, it is treated as a reset point for the
	// counter, with a reset value of 0.
	testChecks = append(testChecks, mockPrometheusCheck{
		fileToUpload: fileToUpload{
			local:  path.Join(prometheusTestdata, "sample_untyped_step_3"),
			remote: path.Join(remoteWorkDir, "data"),
		},
		check: func(ctx context.Context, logger *log.Logger, vm *gce.VM, window time.Duration) error {
			tests := []prometheusMetricTest{
				{"prometheus.googleapis.com/untyped_metric/unknown", nil,
					metric.MetricDescriptor_GAUGE, metric.MetricDescriptor_DOUBLE, 10.0},
				{"prometheus.googleapis.com/untyped_metric/unknown:counter", nil,
					metric.MetricDescriptor_CUMULATIVE, metric.MetricDescriptor_DOUBLE, 0.0},
			}

			var multiErr error
			for _, test := range tests {
				multiErr = multierr.Append(multiErr, assertPrometheusMetric(ctx, logger, vm, window, test))
			}
			if multiErr != nil {
				t.Error(multiErr)
			}
			return multiErr
		},
	})
	testChecks = append(testChecks, mockPrometheusCheck{
		fileToUpload: fileToUpload{
			local:  path.Join(prometheusTestdata, "sample_untyped_step_4"),
			remote: path.Join(remoteWorkDir, "data"),
		},
		check: func(ctx context.Context, logger *log.Logger, vm *gce.VM, window time.Duration) error {
			tests := []prometheusMetricTest{
				{"prometheus.googleapis.com/untyped_metric/unknown", nil,
					metric.MetricDescriptor_GAUGE, metric.MetricDescriptor_DOUBLE, 1000.0},
				{"prometheus.googleapis.com/untyped_metric/unknown:counter", nil,
					metric.MetricDescriptor_CUMULATIVE, metric.MetricDescriptor_DOUBLE, 990.0},
			}

			var multiErr error
			for _, test := range tests {
				multiErr = multierr.Append(multiErr, assertPrometheusMetric(ctx, logger, vm, window, test))
			}
			if multiErr != nil {
				t.Error(multiErr)
			}
			return multiErr
		},
	})
	testPrometheusMetrics(t, config, testChecks)
}

// TestPrometheusHistogramMetrics tests the Histogram metric type using static
// testing files.
func TestPrometheusHistogramMetrics(t *testing.T) {
	config := `metrics:
  receivers:
    prom_app:
      type: prometheus
      config:
        scrape_configs:
        - job_name: test
          metrics_path: /data
          scrape_interval: 10s
          static_configs:
            - targets:
              - localhost:8000
  service:
    pipelines:
      prom_pipeline:
        receivers: [prom_app]
`
	// For Histogram: We use prometheus.LinearBuckets(0, 20, 5), to have
	// buckets w/ le=[0, 20, 40, 60, 80] plus the +inf final bucket
	// For step 1, we observe points [0, 10, 20, 30, 40, 50, 60, 70, 80, 90]
	// And get:
	// Bounds (less than or equal) |0  |20     |40     |60     |80     |+inf
	// Points                      |[0]|[10,20]|[30,40]|[50,60]|[70,80]|[90]
	// Count                       |1  |2      |2      |2      |2      |1
	// And histogram metrics are stored as cumulative type metrics, and
	// histogram metrics get normalized
	// (https://github.com/GoogleCloudPlatform/opentelemetry-operations-go/pull/360)
	// so for this initial step, Count/Mean/SumOfSquaredDeviation are all
	// zeros, and the BucketCounts is nil
	stepOneExpected := &distribution.Distribution{
		Count:                 0,
		Mean:                  0,
		SumOfSquaredDeviation: 0,
		BucketOptions: &distribution.Distribution_BucketOptions{
			Options: &distribution.Distribution_BucketOptions_ExplicitBuckets{
				ExplicitBuckets: &distribution.Distribution_BucketOptions_Explicit{
					Bounds: []float64{0, 20, 40, 60, 80},
				},
			},
		},
	}

	// For step 2, we repeat and observe points
	// [0, 10, 20, 30, 40, 50, 60, 70, 80, 90]
	// And get:
	// Bounds (less than or equal)  |0  |20     |40     |60     |80     |+inf
	// Total Observed in step 1 & 2 |2  |4      |4      |4      |4      |2
	// Delta                        |1  |2      |2      |2      |2      |1
	// Again, histogram metrics are stored as cumulative type metrics, so
	// for this second step, the BucketCounts is the delta value in the
	// above table.
	// Count is the # of new points 10.
	// Mean is (delta of sum) / (# of new points) = (900 - 450) / 10 = 45
	// SumOfSquaredDeviation is not part of the Prometheus histogram. The
	// value is calculated by the googlemanagedprometheusexporter. since the
	// exporter does not have the actual observed values, the calculation
	// assumes all points are in the middle of the bucket, and for +inf
	// the points are at the largest boundary. See:
	// https://github.com/GoogleCloudPlatform/opentelemetry-operations-go/blob/36f91511cfd7be17370e23c36ee839a70cdc914d/exporter/collector/metrics.go#L560
	// So here SumOfSquaredDeviation = (x_i - mean)^2 for x_i in
	// [0, 10, 10, 30, 30, 50, 50, 70, 70, 80] and mean 45
	stepTwoExpected := &distribution.Distribution{
		Count:                 10,
		Mean:                  45,
		SumOfSquaredDeviation: 7450,
		BucketOptions: &distribution.Distribution_BucketOptions{
			Options: &distribution.Distribution_BucketOptions_ExplicitBuckets{
				ExplicitBuckets: &distribution.Distribution_BucketOptions_Explicit{
					Bounds: []float64{0, 20, 40, 60, 80},
				},
			},
		},
		BucketCounts: []int64{1, 2, 2, 2, 2, 1},
	}

	prometheusTestdata := path.Join("testdata", "prometheus")
	remoteWorkDir := path.Join("/opt", "go-http-server")
	testChecks := make([]mockPrometheusCheck, 0)
	testChecks = append(testChecks, mockPrometheusCheck{
		fileToUpload: fileToUpload{
			local:  path.Join(prometheusTestdata, "sample_histogram_step_1"),
			remote: path.Join(remoteWorkDir, "data"),
		},
		check: func(ctx context.Context, logger *log.Logger, vm *gce.VM, window time.Duration) error {
			return assertPrometheusHistogramMetric(ctx, logger, vm, "test_histogram", window, stepOneExpected)
		},
	})
	testChecks = append(testChecks, mockPrometheusCheck{
		fileToUpload: fileToUpload{
			local:  path.Join(prometheusTestdata, "sample_histogram_step_2"),
			remote: path.Join(remoteWorkDir, "data"),
		},
		check: func(ctx context.Context, logger *log.Logger, vm *gce.VM, window time.Duration) error {
			return assertPrometheusHistogramMetric(ctx, logger, vm, "test_histogram", window, stepTwoExpected)
		},
	})

	testPrometheusMetrics(t, config, testChecks)
}

// TestPrometheusSummaryMetrics tests the Summary metric type using static
// testing files.
func TestPrometheusSummaryMetrics(t *testing.T) {
	config := `metrics:
  receivers:
    prom_app:
      type: prometheus
      config:
        scrape_configs:
        - job_name: test
          metrics_path: /data
          scrape_interval: 10s
          static_configs:
            - targets:
              - localhost:8000
  service:
    pipelines:
      prom_pipeline:
        receivers: [prom_app]
`

	// For Summary: We use Objectives [0.5, 0.9, 0.99]
	// For step 1, we observe points [0, 10, 20, 30, 40, 50, 60, 70, 80, 90]
	// And get:
	// Objectives |0.5            |0.9               |0.99
	// Points     |[0,10,20,30,40]|[..., 50,60,70,80]|[...,90]
	// Quantile   |40             |80                |90
	// And summary metrics' quantiles are stored as gauge type metrics, so
	// for this initial step, quantiles have the actual values.
	// But count and sum are stored as cumulative values, and those two get
	// normalized
	// (https://github.com/GoogleCloudPlatform/opentelemetry-operations-go/pull/360)
	// and they are 0s for the first step
	stepOneExpected := prometheusSummaryMetric{
		Quantiles: map[string]float64{
			"0.5":  40,
			"0.9":  80,
			"0.99": 90,
		},
		Count: 0,
		Sum:   0,
	}

	// For step 2, we repeat and observe points
	// [0, 10, 20, 30, 40, 50, 60, 70, 80, 90]
	// And get:
	// Objectives |0.5            |0.9               |0.99
	// Quantile   |40             |80                |90
	// And summary metrics' quantiles are stored as gauge type metrics, so
	// for this second step, quantiles have the actual values.
	// But count and sum are stored as cumulative values, thus:
	// Count = (delta of count) = 20 - 10 = 10
	// Sum = (delta of sum) = 900 - 450 = 450
	stepTwoExpected := prometheusSummaryMetric{
		Quantiles: map[string]float64{
			"0.5":  40,
			"0.9":  80,
			"0.99": 90,
		},
		Count: 10,
		Sum:   450,
	}

	prometheusTestdata := path.Join("testdata", "prometheus")
	remoteWorkDir := path.Join("/opt", "go-http-server")
	testChecks := make([]mockPrometheusCheck, 0)
	testChecks = append(testChecks, mockPrometheusCheck{
		fileToUpload: fileToUpload{
			local:  path.Join(prometheusTestdata, "sample_summary_step_1"),
			remote: path.Join(remoteWorkDir, "data"),
		},
		check: func(ctx context.Context, logger *log.Logger, vm *gce.VM, window time.Duration) error {
			return assertPrometheusSummaryMetric(ctx, logger, vm, "test_summary", window, stepOneExpected)
		},
	})
	testChecks = append(testChecks, mockPrometheusCheck{
		fileToUpload: fileToUpload{
			local:  path.Join(prometheusTestdata, "sample_summary_step_2"),
			remote: path.Join(remoteWorkDir, "data"),
		},
		check: func(ctx context.Context, logger *log.Logger, vm *gce.VM, window time.Duration) error {
			return assertPrometheusSummaryMetric(ctx, logger, vm, "test_summary", window, stepTwoExpected)
		},
	})

	testPrometheusMetrics(t, config, testChecks)
}

func buildGoBinary(ctx context.Context, logger *log.Logger, vm *gce.VM, source, dest string) error {
	_, err := gce.RunRemotely(ctx, logger, vm, fmt.Sprintf("sudo /usr/local/go/bin/go build -o %s/ %s", dest, source))
	return err
}

// testPrometheusMetrics tests different Prometheus metric types using static
// testing files. The files will contain metrics in the right format and hosted
// by a simple HTTP server so that the agent can scrape the metrics
// The test will send two sets of metric points, to verify the metrics are
// correctly received and processed
func testPrometheusMetrics(t *testing.T, opsAgentConfig string, testChecks []mockPrometheusCheck) {
	t.Parallel()
	gce.RunForEachImage(t, func(t *testing.T, imageSpec string) {
		t.Parallel()
		if gce.IsWindows(imageSpec) {
			t.SkipNow()
		}
		ctx, logger, vm := setupMainLogAndVM(t, imageSpec)

		prometheusTestdata := path.Join("testdata", "prometheus")
		remoteWorkDir := path.Join("/opt", "go-http-server")
		serviceFiles := []fileToUpload{
			{local: path.Join(prometheusTestdata, "http_server.go"),
				remote: path.Join(remoteWorkDir, "http_server.go")},
			{local: path.Join(prometheusTestdata, "http-server-for-prometheus-test.service"),
				remote: path.Join("/etc", "systemd", "system", "http-server-for-prometheus-test.service")},
		}

		if len(testChecks) == 0 {
			return
		}

		firstTest := testChecks[0]
		restTests := testChecks[1:]
		// 1. Upload the step one files and files used to setup the http service
		err := uploadFiles(ctx, logger, vm, testdataDir, append(serviceFiles, firstTest.fileToUpload))
		if err != nil {
			t.Fatal(err)
		}

		// 2. Setup the golang
		if err := installGolang(ctx, logger, vm); err != nil {
			t.Fatal(err)
		}

		// 3. Build the http server
		if err := buildGoBinary(ctx, logger, vm, serviceFiles[0].remote, remoteWorkDir); err != nil {
			t.Fatal(err)
		}

		// 4. Start the go http server
		setupScript := `sudo systemctl daemon-reload && sudo systemctl enable http-server-for-prometheus-test && sudo systemctl restart http-server-for-prometheus-test`
		_, err = gce.RunRemotely(ctx, logger, vm, setupScript)
		if err != nil {
			t.Fatalf("failed to start the http server in VM via systemctl with err: %v", err)
		}
		// Wait until the http server is ready
		time.Sleep(20 * time.Second)
		liveCheckOut, liveCheckErr := gce.RunRemotely(ctx, logger, vm, `curl "http://localhost:8000/data"`)
		if liveCheckErr != nil || strings.Contains(liveCheckOut.Stderr, "Connection refused") {
			t.Fatalf("Http server failed to start with stdout %s and stderr %s", liveCheckOut.Stdout, liveCheckOut.Stderr)
		}
		// 3. Config and start the agent
		if err := agents.SetupOpsAgent(ctx, logger, vm, opsAgentConfig); err != nil {
			t.Fatal(err)
		}

		// Wait long enough for the data to percolate through the backends
		// under normal circumstances. Based on some experiments, 2 minutes
		// is normal; wait a bit longer to be on the safe side.
		//
		// We perform the first test seperately since it requires an agent configuration
		//  and setup. All subsequest tests, can just replace the served file and have its
		// own checks.
		time.Sleep(3 * time.Minute)
		window := time.Minute
		var multiErr error
		multiErr = multierr.Append(multiErr, firstTest.check(ctx, logger, vm, window))

		for _, test := range restTests {
			// Replace the text file with the next step metrics
			err = uploadFiles(ctx, logger, vm, testdataDir, []fileToUpload{test.fileToUpload})
			if err != nil {
				t.Fatal(err)
			}

			// Wait until the new points have arrived and complete the check for this test input.
			time.Sleep(3 * time.Minute)
			multiErr = multierr.Append(multiErr, test.check(ctx, logger, vm, window))

			if multiErr != nil {
				t.Error(multiErr)
			}
		}
	})
}

// assertPrometheusHistogramMetric Check if the last point of the time series is
// the expected Prometheus histogram metric point
func assertPrometheusHistogramMetric(ctx context.Context, logger *log.Logger, vm *gce.VM, name string, window time.Duration, expected *distribution.Distribution) error {
	// GCM map Prometheus histogram to cumulative distribution
	test := prometheusMetricTest{
		MetricName:         fmt.Sprintf("prometheus.googleapis.com/%s/histogram", name),
		ExtraFilter:        nil,
		ExpectedMetricKind: metric.MetricDescriptor_CUMULATIVE,
		ExpectedValueType:  metric.MetricDescriptor_DISTRIBUTION,
		ExpectedValue:      expected,
	}
	return assertPrometheusMetric(ctx, logger, vm, window, test)
}

// A sample of the Prometheus summary metric with name 'test_summary':
// # HELP test_summary Test Summary.
// # TYPE test_summary summary
// test_summary{quantile="0.5"} 40
// test_summary{quantile="0.9"} 80
// test_summary{quantile="0.99"} 90
// test_summary_sum 450
// test_summary_count 10
type prometheusSummaryMetric struct {
	Quantiles  map[string]float64
	Count, Sum float64
}

// assertPrometheusSummaryMetric checks if the last point of the time series is
// the expected prometheus summary metric point
func assertPrometheusSummaryMetric(ctx context.Context, logger *log.Logger, vm *gce.VM, name string, window time.Duration, expected prometheusSummaryMetric) error {
	var multiErr error
	// There is no direct mapping of Prometheus summary type. Instead, GCM
	// would store the quantiles into prometheus.googleapis.com/NAME/summary
	// with the actual quantile as a metric label, of type gauge
	for quantile, value := range expected.Quantiles {
		test := prometheusMetricTest{
			MetricName:         fmt.Sprintf("prometheus.googleapis.com/%s/summary", name),
			ExtraFilter:        []string{fmt.Sprintf(`metric.labels.quantile = "%s"`, quantile)},
			ExpectedMetricKind: metric.MetricDescriptor_GAUGE,
			ExpectedValueType:  metric.MetricDescriptor_DOUBLE,
			ExpectedValue:      value,
		}
		multiErr = multierr.Append(multiErr, assertPrometheusMetric(ctx, logger, vm, window, test))
	}
	// The count value in Prometheus summary goes to
	// prometheus.googleapis.com/NAME_count/summary of type cumulative
	testCount := prometheusMetricTest{
		MetricName:         fmt.Sprintf("prometheus.googleapis.com/%s_count/summary", name),
		ExtraFilter:        nil,
		ExpectedMetricKind: metric.MetricDescriptor_CUMULATIVE,
		ExpectedValueType:  metric.MetricDescriptor_DOUBLE,
		ExpectedValue:      expected.Count,
	}
	multiErr = multierr.Append(multiErr, assertPrometheusMetric(ctx, logger, vm, window, testCount))
	// The sum value in Prometheus summary goes to
	// prometheus.googleapis.com/NAME_sum/summary:counter of type cumulative
	testSummary := prometheusMetricTest{
		MetricName:         fmt.Sprintf("prometheus.googleapis.com/%s_sum/summary:counter", name),
		ExtraFilter:        nil,
		ExpectedMetricKind: metric.MetricDescriptor_CUMULATIVE,
		ExpectedValueType:  metric.MetricDescriptor_DOUBLE,
		ExpectedValue:      expected.Sum,
	}
	multiErr = multierr.Append(multiErr, assertPrometheusMetric(ctx, logger, vm, window, testSummary))
	return multiErr
}

// prometheusMetricTest specify a test to use 'MetricName' and 'ExtraFilter' to
// get the metric and compare with the expected kind, type and value
type prometheusMetricTest struct {
	MetricName         string
	ExtraFilter        []string
	ExpectedMetricKind metric.MetricDescriptor_MetricKind
	ExpectedValueType  metric.MetricDescriptor_ValueType
	ExpectedValue      any
}

// assertPrometheusMetric with a given test, wait for the metric, and then use
// the latest point as the actual value and compare with the expected value
func assertPrometheusMetric(ctx context.Context, logger *log.Logger, vm *gce.VM, window time.Duration, test prometheusMetricTest) error {
	var multiErr error
	if pts, err := gce.WaitForMetric(ctx, logger, vm, test.MetricName, window, test.ExtraFilter, true); err != nil {
		multiErr = multierr.Append(multiErr, err)
	} else {
		if pts.MetricKind != test.ExpectedMetricKind {
			multiErr = multierr.Append(multiErr, fmt.Errorf("Metric %s has metric kind %s; expected kind %s", test.MetricName, pts.MetricKind, test.ExpectedMetricKind))
		}
		if pts.ValueType != test.ExpectedValueType {
			multiErr = multierr.Append(multiErr, fmt.Errorf("Metric %s has value type %s; expected type %s", test.MetricName, pts.ValueType, test.ExpectedValueType))
		}
		if len(pts.Points) == 0 {
			multiErr = multierr.Append(multiErr, fmt.Errorf("Metric %s has at least one data points in the time windows", test.MetricName))
		} else {
			// Use the last/latest point
			actual := pts.Points[len(pts.Points)-1]
			switch test.ExpectedValueType {
			case metric.MetricDescriptor_DOUBLE:
				expectedValue := test.ExpectedValue.(float64)
				actualValue := actual.Value.GetDoubleValue()
				if actualValue != expectedValue {
					multiErr = multierr.Append(multiErr, fmt.Errorf("Metric %s has value %f; expected %f", test.MetricName, actualValue, expectedValue))
				}
			case metric.MetricDescriptor_DISTRIBUTION:
				expectedValue := test.ExpectedValue.(*distribution.Distribution)
				actualValue := actual.Value.GetDistributionValue()
				if !slices.Equal(actualValue.GetBucketOptions().GetExplicitBuckets().GetBounds(), expectedValue.GetBucketOptions().GetExplicitBuckets().GetBounds()) {
					multiErr = multierr.Append(multiErr, fmt.Errorf("Metric %s has buckets bounds %v; expected %v",
						test.MetricName, actualValue.GetBucketOptions().GetExplicitBuckets().GetBounds(), expectedValue.GetBucketOptions().GetExplicitBuckets().GetBounds()))
				}
				if !slices.Equal(actualValue.GetBucketCounts(), expectedValue.GetBucketCounts()) {
					multiErr = multierr.Append(multiErr, fmt.Errorf("Metric %s has buckets with counts %v; expected %v",
						test.MetricName, actualValue.GetBucketCounts(), expectedValue.GetBucketCounts()))
				}
				if actualValue.Count != expectedValue.Count {
					multiErr = multierr.Append(multiErr, fmt.Errorf("Metric %s has count %d; expected %d",
						test.MetricName, actualValue.Count, expectedValue.Count))
				}
				if actualValue.Mean != expectedValue.Mean {
					multiErr = multierr.Append(multiErr, fmt.Errorf("Metric %s has mean %f; expected %f",
						test.MetricName, actualValue.Mean, expectedValue.Mean))
				}
				if actualValue.SumOfSquaredDeviation != expectedValue.SumOfSquaredDeviation {
					multiErr = multierr.Append(multiErr, fmt.Errorf("Metric %s has sum of squared deviation %f; expected %f",
						test.MetricName, actualValue.SumOfSquaredDeviation, expectedValue.SumOfSquaredDeviation))
				}
			default:
				multiErr = multierr.Append(multiErr, fmt.Errorf("Value check for metric with type %s is not implementated", test.ExpectedValueType))
			}

		}
	}
	return multiErr
}

type fileToUpload struct {
	local, remote string
}

// mockPrometheusCheck contains input files in the prometheus format that will be served
// by an http server at endpoint localhost:8000/metrics. It also contains a check that
// be used to validate that the ingested metrics are collected correctly.
type mockPrometheusCheck struct {
	fileToUpload fileToUpload
	check        func(ctx context.Context, logger *log.Logger, vm *gce.VM, window time.Duration) error
}

// uploadFiles upload files from fs embedded file system to vm
func uploadFiles(ctx context.Context, logger *log.Logger, vm *gce.VM, fs embed.FS, files []fileToUpload) error {
	for _, upload := range files {
		err := func() error {
			f, err := fs.Open(upload.local)
			if err != nil {
				return err
			}
			defer f.Close()
			err = gce.UploadContent(ctx, logger, vm, f, upload.remote)
			return err
		}()
		if err != nil {
			return err
		}
	}
	return nil
}

func TestExcludeMetrics(t *testing.T) {
	t.Parallel()
	gce.RunForEachImage(t, func(t *testing.T, imageSpec string) {
		t.Parallel()

		ctx, logger, vm := setupMainLogAndVM(t, imageSpec)

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
      - agent.googleapis.com/cpu/load_5m
  service:
    pipelines:
      default_pipeline:
        receivers: [hostmetrics]
        processors: [metrics_filter]
`

		if err := agents.SetupOpsAgent(ctx, logger, vm, excludeConfig); err != nil {
			t.Fatal(err)
		}

		// Wait long enough for the data to percolate through the backends
		// under normal circumstances. Based on some experiments, 2 minutes
		// is normal; wait a bit longer to be on the safe side.
		time.Sleep(3 * time.Minute)

		existingMetric := "agent.googleapis.com/cpu/load_1m"
		excludedIndividualMetric := "agent.googleapis.com/cpu/load_5m"
		excludedWildcardMetric := "agent.googleapis.com/processes/cpu_time"

		window := time.Minute
		if _, err := gce.WaitForMetric(ctx, logger, vm, existingMetric, window, nil, false); err != nil {
			t.Error(err)
		}
		if err := gce.AssertMetricMissing(ctx, logger, vm, excludedIndividualMetric, false, window); err != nil {
			t.Error(err)
		}
		if err := gce.AssertMetricMissing(ctx, logger, vm, excludedWildcardMetric, false, window); err != nil {
			t.Error(err)
		}
	})
}

func TestExcludeWorkloadMetrics(t *testing.T) {
	t.Parallel()
	gce.RunForEachImage(t, func(t *testing.T, imageSpec string) {
		t.Parallel()
		ctx, logger, vm := setupMainLogAndVM(t, imageSpec)
		otlpConfig := `
combined:
  receivers:
    otlp:
      type: otlp
      grpc_endpoint: 0.0.0.0:4317
      metrics_mode: googlecloudmonitoring
metrics:
  processors:
    metrics_filter:
      type: exclude_metrics
      metrics_pattern:
      - workload.googleapis.com/otlp.test.gauge
      - workload.googleapis.com/otlp.test.prefix*
  service:
    pipelines:
      otlp:
        receivers: [otlp]
        processors: [metrics_filter]
traces:
  service:
    pipelines:
`
		if err := agents.SetupOpsAgent(ctx, logger, vm, otlpConfig); err != nil {
			t.Fatal(err)
		}

		// Generate metric traffic with dummy app
		metricFile, err := testdataDir.Open(path.Join("testdata", "otlp", "metrics.go"))
		if err != nil {
			t.Fatal(err)
		}
		defer metricFile.Close()
		if err := installGolang(ctx, logger, vm); err != nil {
			t.Fatal(err)
		}
		if err = runGoCode(ctx, logger, vm, metricFile); err != nil {
			t.Fatal(err)
		}

		if _, err = gce.WaitForMetric(ctx, logger, vm, "workload.googleapis.com/otlp.test.cumulative", time.Hour, nil, false); err != nil {
			t.Error(err)
		}

		// Wait long enough for the data to percolate through the backends
		// under normal circumstances. Based on some experiments, 2 minutes
		// is normal; wait a bit longer to be on the safe side.
		time.Sleep(3 * time.Minute)
		window := time.Minute

		// See testdata/otlp/metrics.go for the metrics we're sending
		for _, m := range []string{
			"workload.googleapis.com/otlp.test.gauge",
			"workload.googleapis.com/otlp.test.prefix3/workload.googleapis.com/abc",
		} {
			if err = gce.AssertMetricMissing(ctx, logger, vm, m, false, window); err != nil {
				t.Error(err)
			}
		}
	})
}

// fetchPID returns the process ID of the process with the given name on the given VM.
func fetchPID(ctx context.Context, logger *log.Logger, vm *gce.VM, processName string) (string, error) {
	var cmd string
	if gce.IsWindows(vm.ImageSpec) {
		cmd = fmt.Sprintf("Get-Process -Name '%s' | Select-Object -Property Id | Format-Wide", processName)
	} else {
		// pgrep has a limit of 15 characters to lookup processes
		// using -f uses the full command line for lookup
		// using the pattern "[p]rocessName" avoids matching the ssh remote shell
		// https://linux.die.net/man/1/pgrep
		if len(processName) > 15 {
			cmd = "sudo pgrep -f " + "[" + processName[:1] + "]" + processName[1:]
		} else {
			cmd = "sudo pgrep " + processName
		}
	}
	output, err := gce.RunRemotely(ctx, logger, vm, cmd)
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
	if gce.IsWindows(vm.ImageSpec) {
		cmd = fmt.Sprintf("Stop-Process -Name '%s' -PassThru -Force", processName)
	} else {
		// pkill has a limit of 15 characters to lookup processes
		// using -f uses the full command line for lookup
		// using the pattern "[p]rocessName" avoids matching the ssh remote shell
		// https://linux.die.net/man/1/pkill
		if len(processName) > 15 {
			cmd = "sudo pkill -SIGABRT -f " + "[" + processName[:1] + "]" + processName[1:]
		} else {
			cmd = "sudo pkill -SIGABRT " + processName
		}
	}
	_, err := gce.RunRemotely(ctx, logger, vm, cmd)
	if err != nil {
		return fmt.Errorf("terminateProcess(%q) failed: %v", processName, err)
	}
	return nil
}

func testAgentCrashRestart(ctx context.Context, t *testing.T, logger *log.Logger, vm *gce.VM, processNames []string, livenessChecker func(context.Context, *log.Logger, *gce.VM) error) {
	if err := agents.SetupOpsAgent(ctx, logger, vm, ""); err != nil {
		t.Fatal(err)
	}

	pidOutputBefore, processName, err := fetchPIDAndProcessName(ctx, logger, vm, processNames)
	if err != nil {
		t.Fatal(err)
	}

	logger.Printf("testAgentCrashRestart: Found %s", processName)

	// Simulate a crash.
	if err := terminateProcess(ctx, logger, vm, processName); err != nil {
		t.Fatal(err)
	}

	if err := livenessChecker(ctx, logger, vm); err != nil {
		t.Fatalf("Liveness checker reported error: %v", err)
	}

	// Consistency check: make sure that the agent's PID actually changed so that
	// we know we crashed it successfully.
	pidOutputAfter, err := fetchPID(ctx, logger, vm, processName)
	if err != nil {
		t.Fatal(err)
	}

	if pidOutputBefore == pidOutputAfter {
		t.Errorf("PID did not change; we failed to crash %v.", processName)
	}
}

func metricsLivenessChecker(ctx context.Context, logger *log.Logger, vm *gce.VM) error {
	time.Sleep(3 * time.Minute)
	// Query for a metric from the last minute. Sleep for 3 minutes first
	// to make sure we aren't picking up metrics from a previous instance
	// of the metrics agent.
	_, err := gce.WaitForMetric(ctx, logger, vm, "agent.googleapis.com/cpu/utilization", time.Minute, nil, false)
	return err
}

func TestMetricsAgentCrashRestart(t *testing.T) {
	t.Parallel()
	gce.RunForEachImage(t, func(t *testing.T, imageSpec string) {
		t.Parallel()
		ctx, logger, vm := setupMainLogAndVM(t, imageSpec)

		testAgentCrashRestart(ctx, t, logger, vm, metricsAgentProcessNamesForImage(vm.ImageSpec), metricsLivenessChecker)
	})
}

func loggingLivenessChecker(ctx context.Context, logger *log.Logger, vm *gce.VM) error {
	msg := uuid.NewString()
	if err := writeToSystemLog(ctx, logger, vm, msg); err != nil {
		return err
	}
	tag := systemLogTagForImage(vm.ImageSpec)
	return gce.WaitForLog(ctx, logger, vm, tag, time.Hour, logMessageQueryForImage(vm.ImageSpec, msg))
}

func TestLoggingAgentCrashRestart(t *testing.T) {
	t.Parallel()
	gce.RunForEachImage(t, func(t *testing.T, imageSpec string) {
		t.Parallel()
		ctx, logger, vm := setupMainLogAndVM(t, imageSpec)

		testAgentCrashRestart(ctx, t, logger, vm, []string{"fluent-bit"}, loggingLivenessChecker)
	})
}

func TestLoggingSelfLogs(t *testing.T) {
	t.Parallel()
	gce.RunForEachImage(t, func(t *testing.T, imageSpec string) {
		t.Parallel()
		ctx, logger, vm := agents.CommonSetup(t, imageSpec)

		if err := agents.SetupOpsAgent(ctx, logger.ToMainLog(), vm, ""); err != nil {
			t.Fatal(err)
		}
		start := time.Now()

		if err := gce.WaitForLog(ctx, logger.ToMainLog(), vm, "ops-agent-fluent-bit", time.Hour, `severity="INFO"`); err != nil {
			t.Error(err)
		}

		queryHealthCheck := fmt.Sprintf(`severity="INFO" AND labels."agent.googleapis.com/health/agentKind"="ops-agent" AND labels."agent.googleapis.com/health/agentVersion"=~"^\d+\.\d+\.\d+.*$" AND labels."agent.googleapis.com/health/schemaVersion"="v1"`)
		if err := gce.WaitForLog(ctx, logger.ToMainLog(), vm, "ops-agent-health", time.Hour, queryHealthCheck); err != nil {
			t.Error(err)
		}

		// Waiting 10 minutes (subtracting current test runtime) after Ops Agent startup for
		// "LogPingOpsAgent" to show. We can remove wait when feature b/319102785 is complete.
		time.Sleep(10*time.Minute - time.Now().Sub(start))
		queryPing := fmt.Sprintf(`severity="DEBUG" AND jsonPayload.code="LogPingOpsAgent" AND labels."agent.googleapis.com/health/agentKind"="ops-agent" AND labels."agent.googleapis.com/health/agentVersion"=~"^\d+\.\d+\.\d+.*$" AND labels."agent.googleapis.com/health/schemaVersion"="v1"`)
		if err := gce.WaitForLog(ctx, logger.ToMainLog(), vm, "ops-agent-health", time.Hour, queryPing); err != nil {
			t.Error(err)
		}
	})
}

func TestLoggingDataprocAttributes(t *testing.T) {
	t.Parallel()
	gce.RunForEachImage(t, func(t *testing.T, imageSpec string) {
		t.Parallel()
		if gce.IsWindows(imageSpec) {
			t.Skip("Dataproc tests aren't supported on Windows")
		}
		ctx, logger, vm := agents.CommonSetupWithExtraCreateArgumentsAndMetadata(t, imageSpec, nil, map[string]string{
			"dataproc-cluster-name": "my-test-cluster",
		})

		if err := agents.SetupOpsAgent(ctx, logger.ToMainLog(), vm, ""); err != nil {
			t.Fatal(err)
		}

		if err := writeToSystemLog(ctx, logger.ToMainLog(), vm, "123456789"); err != nil {
			t.Fatal(err)
		}

		tag := systemLogTagForImage(vm.ImageSpec)
		query := fmt.Sprintf(`labels."compute.googleapis.com/attributes/dataproc-cluster-name"="my-test-cluster" AND %s`, logMessageQueryForImage(vm.ImageSpec, "123456789"))
		if err := gce.WaitForLog(ctx, logger.ToMainLog(), vm, tag, time.Hour, query); err != nil {
			t.Error(err)
		}
	})
}

func diagnosticsLivenessChecker(ctx context.Context, logger *log.Logger, vm *gce.VM) error {
	time.Sleep(3 * time.Minute)
	// Query for a metric sent by the diagnostics service from the last
	// minute. Sleep for 3 minutes first to make sure we aren't picking
	// up metrics from a previous instance of the diagnostics service.
	_, err := gce.WaitForMetric(ctx, logger, vm, "agent.googleapis.com/agent/ops_agent/enabled_receivers", time.Minute, nil, false)
	return err
}

func TestDiagnosticsCrashRestart(t *testing.T) {
	t.Parallel()
	gce.RunForEachImage(t, func(t *testing.T, imageSpec string) {
		t.Parallel()
		ctx, logger, vm := setupMainLogAndVM(t, imageSpec)

		testAgentCrashRestart(ctx, t, logger, vm, diagnosticsProcessNamesForImage(vm.ImageSpec), diagnosticsLivenessChecker)
	})
}

func testWindowsStandaloneAgentConflict(t *testing.T, installStandalone func(ctx context.Context, logger *log.Logger, vm *gce.VM) error, wantError string) {
	t.Parallel()
	gce.RunForEachImage(t, func(t *testing.T, imageSpec string) {
		t.Parallel()
		if !gce.IsWindows(imageSpec) {
			t.SkipNow()
		}
		ctx, logger, vm := setupMainLogAndVM(t, imageSpec)

		// 1. Install the standalone agent.
		if err := installStandalone(ctx, logger, vm); err != nil {
			t.Fatal(err)
		}

		// 2. Install the Ops Agent.  Installation will succeed but log an error.
		if err := agents.InstallOpsAgent(ctx, logger, vm, agents.LocationFromEnvVars()); err != nil {
			t.Fatal(err)
		}

		// 3. Check the error log for a message about Ops Agent conflicting with standalone agent.
		getEvents := `Get-WinEvent -FilterHashtable @{
		  LogName = 'Application'
			ProviderName = 'google-cloud-ops-agent'
		} | Select-Object -ExpandProperty Message`
		out, err := gce.RunRemotely(ctx, logger, vm, getEvents)
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

func opsAgentLivenessChecker(ctx context.Context, logger *log.Logger, vm *gce.VM) error {
	return multierr.Append(
		loggingLivenessChecker(ctx, logger, vm),
		metricsLivenessChecker(ctx, logger, vm))
}

func TestUpgradeOpsAgent(t *testing.T) {
	t.Parallel()
	gce.RunForEachImage(t, func(t *testing.T, imageSpec string) {
		t.Parallel()

		ctx, logger, vm := setupMainLogAndVM(t, imageSpec)

		// This will install the stable Ops Agent (REPO_SUFFIX="").
		if err := agents.SetupOpsAgentFrom(ctx, logger, vm, "", agents.PackageLocation{}); err != nil {
			// Installation from stable may fail before the first release on
			// a newly supported distro.
			if strings.HasPrefix(err.Error(), "InstallOpsAgent() failed to run googet") ||
				strings.HasPrefix(err.Error(), "InstallOpsAgent() error running repo script") {
				t.Skipf("Installing stable agent failed with error %v; assuming first release.", err)
			}
			t.Fatal(err)
		}

		// Wait for the Ops Agent to be active. Make sure that it is working.
		if err := opsAgentLivenessChecker(ctx, logger, vm); err != nil {
			t.Fatal(err)
		}

		// Install the Ops agent from AGENT_PACKAGES_IN_GCS or REPO_SUFFIX.
		secondVersion := agents.LocationFromEnvVars()
		if err := agents.SetupOpsAgentFrom(ctx, logger, vm, "", secondVersion); err != nil {
			t.Fatal(err)
		}

		// Make sure that the newly installed Ops Agent is working.
		if err := opsAgentLivenessChecker(ctx, logger, vm); err != nil {
			t.Fatal(err)
		}
	})
}

func TestResourceDetectorOnGCE(t *testing.T) {
	t.Parallel()
	gce.RunForEachImage(t, func(t *testing.T, imageSpec string) {
		t.Parallel()
		ctx, logger, vm := setupMainLogAndVM(t, imageSpec)

		actual, err := runResourceDetectorCli(ctx, logger, vm)
		if err != nil {
			t.Fatal(err)
		}

		if actual.InstanceName != vm.Name {
			t.Errorf("detector attribute InstanceName has value %q; expected %q", actual.InstanceName, vm.Name)
		}
		if actual.Project != vm.Project {
			t.Errorf("detector attribute Project has value %q; expected %q", actual.Project, vm.Project)
		}
		expectedNetworkURL := regexp.MustCompile(fmt.Sprintf("^projects/[0-9]+/networks/%s$", vm.Network))
		if !expectedNetworkURL.MatchString(actual.Network) {
			t.Errorf("detector attribute Network has value %q; expected %q", actual.Network, expectedNetworkURL.String())
		}
		if actual.Zone != vm.Zone {
			t.Errorf("detector attribute Zone has value %q; expected %q", actual.Zone, vm.Zone)
		}
		expectedMachineType := regexp.MustCompile(fmt.Sprintf("^projects/[0-9]+/machineTypes/%s$", vm.MachineType))
		if !expectedMachineType.MatchString(actual.MachineType) {
			t.Errorf("detector attribute MachineType has value %q; expected %q", actual.MachineType, expectedMachineType.String())
		}
		if actual.InstanceID != fmt.Sprint(vm.ID) {
			t.Errorf("detector attribute InstanceID has value %q; expected %q", actual.InstanceID, fmt.Sprint(vm.ID))
		}
		if len(actual.InterfaceIPv4) == 0 {
			t.Errorf("detector attribute InterfaceIPv4 should have at least one value")
		}
		// Depends on the setup of the integration test, vm.IPAddress can be either the public or the private IP
		if actual.PrivateIP != vm.IPAddress && actual.PublicIP != vm.IPAddress {
			t.Errorf("detector attribute PrivateIP has value %q and PublicIP has value %q; expected at least one to be %q", actual.PrivateIP, actual.PublicIP, vm.IPAddress)
		}
		// For the current integration tests we always attach the following metadata
		if v, ok := actual.Metadata["serial-port-logging-enable"]; ok {
			if v != "true" {
				t.Errorf("detector attribute Metadata has values %v; expected to have %q as %q", actual.Metadata, "serial-port-logging-enable", "true")
			}
		} else {
			t.Errorf("detector attribute Metadata has values %v; expected to have %q", actual.Metadata, "serial-port-logging-enable")
		}
	})
}

// runResourceDetectorCli uploads the resource detector runner and sets up the
// env in the VM. Then run the runner to print out the JSON formatted
// GCEResource and finally unmarshal it back to an instance of GCEResource
func runResourceDetectorCli(ctx context.Context, logger *log.Logger, vm *gce.VM) (*resourcedetector.GCEResource, error) {
	// Update the resourcedetector package and the go.mod and go.sum
	// So that the main function can locate the package from the work directory
	filesToUpload := []struct {
		local, remote string
	}{
		{local: "../cmd/run_resource_detector/run_resource_detector.go",
			remote: "run_resource_detector.go"},
		{local: "../../confgenerator/resourcedetector/detector.go",
			remote: "confgenerator/resourcedetector/detector.go"},
		{local: "../../confgenerator/resourcedetector/gce_detector.go",
			remote: "confgenerator/resourcedetector/gce_detector.go"},
		{local: "../../confgenerator/resourcedetector/gce_metadata_provider.go",
			remote: "confgenerator/resourcedetector/gce_metadata_provider.go"},
		{local: "../../go.mod",
			remote: "go.mod"},
		{local: "../../go.sum",
			remote: "go.sum"},
	}

	// Create the folder structure on the VM
	workDir := path.Join(workDirForImage(vm.ImageSpec), "run_resource_detector")
	packageDir := path.Join(workDir, "confgenerator", "resourcedetector")
	if err := makeDirectory(ctx, logger, vm, packageDir); err != nil {
		return nil, fmt.Errorf("failed to create folder %s in VM: %v", packageDir, err)
	}

	// Upload the files
	for _, file := range filesToUpload {
		f, err := os.Open(file.local)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		err = gce.UploadContent(ctx, logger, vm, f, path.Join(workDir, file.remote))
		if err != nil {
			return nil, err
		}
	}

	// Run the resource detector in the VM
	if err := installGolang(ctx, logger, vm); err != nil {
		return nil, err
	}
	cmd := fmt.Sprintf(`
		%s
		cd %s
		go run run_resource_detector.go`, goPathCommandForImage(vm.ImageSpec), workDir)
	runnerOutput, err := gce.RunRemotely(ctx, logger, vm, cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to run resource detector in VM: %w", err)
	}

	// Parse the output
	d, err := unmarshalResource(runnerOutput.Stdout)
	if err != nil {
		return nil, fmt.Errorf("can't unmarshal a detector from JSON: %v", err)
	}
	return d, nil
}

// unmarshalResource Unmarshal the string to a GCEResource
func unmarshalResource(in string) (*resourcedetector.GCEResource, error) {
	r := regexp.MustCompile("{(\"(Project|Zone|Network|Subnetwork|PublicIP|PrivateIP|InstanceID|InstanceName|Tags|MachineType|Metadata|Label|InterfaceIPv4)\":.*)+}")
	match := r.FindString(in)
	in_byte := []byte(match)
	var resource resourcedetector.GCEResource
	err := json.Unmarshal(in_byte, &resource)
	return &resource, err
}

// uninstallGolang removes the go installation on the VM.
func uninstallGolang(ctx context.Context, logger *log.Logger, vm *gce.VM) error {
	var cmd string
	if gce.IsWindows(vm.ImageSpec) {
		// There is currently no need for this support.
		return nil
	} else {
		cmd = `sudo rm -rf /usr/local/go`
	}
	_, err := gce.RunRemotely(ctx, logger, vm, cmd)
	if err != nil {
		return err
	}
	return nil
}

// installGolang downloads and sets up go on the given VM. The caller is still
// responsible for updating PATH to point to the installed binaries, see
// `goPathCommandForImage`. If go is already installed, uninstall it first.
func installGolang(ctx context.Context, logger *log.Logger, vm *gce.VM) error {
	err := uninstallGolang(ctx, logger, vm)
	if err != nil {
		return err
	}

	// To update this, first run `mirror_content.sh` under `integration_test`. Example:
	//   ./mirror_content.sh https://go.dev/dl/go1.21.4.linux-{amd64,arm64}.tar.gz
	// Then update this version.
	goVersion := "1.22.7"

	goArch := "amd64"
	if gce.IsARM(vm.ImageSpec) {
		goArch = "arm64"
	}
	var installCmd string
	if gce.IsWindows(vm.ImageSpec) {
		// TODO: host go windows installer in GCS if `go.dev` throttles us.
		installCmd = fmt.Sprintf(`
			cd (New-TemporaryFile | %% { Remove-Item $_; New-Item -ItemType Directory -Path $_ })
			Invoke-WebRequest "https://go.dev/dl/go%s.windows-%s.msi" -OutFile golang.msi
			Start-Process msiexec.exe -ArgumentList "/i","golang.msi","/quiet" -Wait `, goVersion, goArch)
	} else {
		installCmd = fmt.Sprintf(`
			set -o pipefail
			gsutil cp \
				"gs://ops-agents-public-buckets-vendored-deps/mirrored-content/go.dev/dl/go%s.linux-%s.tar.gz" - | \
				sudo tar --directory /usr/local -xzf /dev/stdin`, goVersion, goArch)
	}
	_, err = gce.RunRemotely(ctx, logger, vm, installCmd)
	return err
}

func goPathCommandForImage(imageSpec string) string {
	if gce.IsWindows(imageSpec) {
		return `$env:Path="C:\Program Files\Go\bin;$env:Path"`
	}
	return "export PATH=/usr/local/go/bin:$PATH"
}

func runGoCode(ctx context.Context, logger *log.Logger, vm *gce.VM, content io.Reader) error {
	workDir := path.Join(workDirForImage(vm.ImageSpec), "gocode")
	if err := makeDirectory(ctx, logger, vm, workDir); err != nil {
		return err
	}
	if err := gce.UploadContent(ctx, logger, vm, content, path.Join(workDir, "main.go")); err != nil {
		return err
	}
	goInitAndRun := fmt.Sprintf(`
		%s
		cd %s
		go mod init main
		go get ./...
		go run main.go`, goPathCommandForImage(vm.ImageSpec), workDir)
	_, err := gce.RunRemotely(ctx, logger, vm, goInitAndRun)
	return err
}

func TestOTLPMetricsGCM(t *testing.T) {
	t.Parallel()
	gce.RunForEachImage(t, func(t *testing.T, imageSpec string) {
		t.Parallel()
		ctx, logger, vm := setupMainLogAndVM(t, imageSpec)
		otlpConfig := `
combined:
  receivers:
    otlp:
      type: otlp
      grpc_endpoint: 0.0.0.0:4317
      metrics_mode: googlecloudmonitoring
metrics:
  service:
    pipelines:
      otlp:
        receivers:
        - otlp
traces:
  service:
    pipelines:
`
		if err := agents.SetupOpsAgent(ctx, logger, vm, otlpConfig); err != nil {
			t.Fatal(err)
		}

		// Have to wait for startup feature tracking metrics to be sent
		// before we tear down the service.
		time.Sleep(20 * time.Second)

		// Generate metric traffic with dummy app
		metricFile, err := testdataDir.Open(path.Join("testdata", "otlp", "metrics.go"))
		if err != nil {
			t.Fatal(err)
		}
		defer metricFile.Close()
		if err := installGolang(ctx, logger, vm); err != nil {
			t.Fatal(err)
		}
		if err = runGoCode(ctx, logger, vm, metricFile); err != nil {
			t.Fatal(err)
		}

		// See testdata/otlp/metrics.go for the metrics we're sending
		for _, name := range []string{
			"workload.googleapis.com/otlp.test.gauge",
			"workload.googleapis.com/otlp.test.cumulative",
			"workload.googleapis.com/otlp.test.prefix1",
			"workload.googleapis.com/.invalid.googleapis.com/otlp.test.prefix2",
			"workload.googleapis.com/otlp.test.prefix3/workload.googleapis.com/abc",
			"workload.googleapis.com/WORKLOAD.GOOGLEAPIS.COM/otlp.test.prefix4",
			"workload.googleapis.com/WORKLOAD.googleapis.com/otlp.test.prefix5",
		} {
			if _, err = gce.WaitForMetric(ctx, logger, vm, name, time.Hour, nil, false); err != nil {
				t.Error(err)
			}
		}

		expectedFeatures := []*feature_tracking_metadata.FeatureTracking{
			{
				Module:  "logging",
				Feature: "service:pipelines",
				Key:     "default_pipeline_overridden",
				Value:   "false",
			},
			{
				Module:  "metrics",
				Feature: "service:pipelines",
				Key:     "default_pipeline_overridden",
				Value:   "false",
			},
			{
				Module:  "combined",
				Feature: "receivers:otlp",
				Key:     "[0].metrics_mode",
				Value:   "googlecloudmonitoring",
			},
			{
				Module:  "combined",
				Feature: "receivers:otlp",
				Key:     "[0].enabled",
				Value:   "true",
			},
			{
				Module:  "combined",
				Feature: "receivers:otlp",
				Key:     "[0].grpc_endpoint",
				Value:   "endpoint",
			},
		}

		series, err := gce.WaitForMetricSeries(ctx, logger, vm, "agent.googleapis.com/agent/internal/ops/feature_tracking", time.Hour, nil, false, len(expectedFeatures))
		if err != nil {
			t.Error(err)
			return
		}

		err = feature_tracking_metadata.AssertFeatureTrackingMetrics(series, expectedFeatures)
		if err != nil {
			t.Error(err)
			return
		}
	})
}

func TestOTLPMetricsGMP(t *testing.T) {
	t.Parallel()
	gce.RunForEachImage(t, func(t *testing.T, imageSpec string) {
		t.Parallel()
		ctx, logger, vm := setupMainLogAndVM(t, imageSpec)
		otlpConfig := `
combined:
  receivers:
    otlp:
      type: otlp
      grpc_endpoint: 0.0.0.0:4317
metrics:
  service:
    pipelines:
      otlp:
        receivers:
        - otlp
traces:
  service:
    pipelines:
`
		if err := agents.SetupOpsAgent(ctx, logger, vm, otlpConfig); err != nil {
			t.Fatal(err)
		}

		// Have to wait for startup feature tracking metrics to be sent
		// before we tear down the service.
		time.Sleep(20 * time.Second)

		// Generate metric traffic with dummy app
		metricFile, err := testdataDir.Open(path.Join("testdata", "otlp", "metrics.go"))
		if err != nil {
			t.Fatal(err)
		}
		defer metricFile.Close()
		if err := installGolang(ctx, logger, vm); err != nil {
			t.Fatal(err)
		}
		if err = runGoCode(ctx, logger, vm, metricFile); err != nil {
			t.Fatal(err)
		}

		// See testdata/otlp/metrics.go for the metrics we're sending
		for _, name := range []string{
			"prometheus.googleapis.com/otlp_test_gauge/gauge",
			"prometheus.googleapis.com/otlp_test_cumulative/counter",
			"prometheus.googleapis.com/workload_googleapis_com_otlp_test_prefix1/gauge",
			"prometheus.googleapis.com/invalid_googleapis_com_otlp_test_prefix2/gauge",
			"prometheus.googleapis.com/otlp_test_prefix3_workload_googleapis_com_abc/gauge",
			"prometheus.googleapis.com/WORKLOAD_GOOGLEAPIS_COM_otlp_test_prefix4/gauge",
			"prometheus.googleapis.com/WORKLOAD_googleapis_com_otlp_test_prefix5/gauge",
		} {
			if _, err = gce.WaitForMetric(ctx, logger, vm, name, time.Hour, nil, true); err != nil {
				t.Error(err)
			}
		}

		expectedFeatures := []*feature_tracking_metadata.FeatureTracking{
			{
				Module:  "logging",
				Feature: "service:pipelines",
				Key:     "default_pipeline_overridden",
				Value:   "false",
			},
			{
				Module:  "metrics",
				Feature: "service:pipelines",
				Key:     "default_pipeline_overridden",
				Value:   "false",
			},
			{
				Module:  "combined",
				Feature: "receivers:otlp",
				Key:     "[0].enabled",
				Value:   "true",
			},
			{
				Module:  "combined",
				Feature: "receivers:otlp",
				Key:     "[0].grpc_endpoint",
				Value:   "endpoint",
			},
		}

		series, err := gce.WaitForMetricSeries(ctx, logger, vm, "agent.googleapis.com/agent/internal/ops/feature_tracking", time.Hour, nil, false, len(expectedFeatures))
		if err != nil {
			t.Error(err)
			return
		}

		err = feature_tracking_metadata.AssertFeatureTrackingMetrics(series, expectedFeatures)
		if err != nil {
			t.Error(err)
			return
		}
	})
}

func TestOTLPTraces(t *testing.T) {
	t.Parallel()
	gce.RunForEachImage(t, func(t *testing.T, imageSpec string) {
		t.Parallel()
		ctx, logger, vm := setupMainLogAndVM(t, imageSpec)
		otlpConfig := `
combined:
  receivers:
    otlp:
      type: otlp
traces:
  service:
    pipelines:
      otlp:
        receivers:
        - otlp
metrics:
  service:
    pipelines:
`
		if err := agents.SetupOpsAgent(ctx, logger, vm, otlpConfig); err != nil {
			t.Fatal(err)
		}

		// Generate trace traffic with dummy app
		traceFile, err := testdataDir.Open(path.Join("testdata", "otlp", "traces.go"))
		if err != nil {
			t.Fatal(err)
		}
		defer traceFile.Close()
		if err := installGolang(ctx, logger, vm); err != nil {
			t.Fatal(err)
		}
		if err = runGoCode(ctx, logger, vm, traceFile); err != nil {
			t.Fatal(err)
		}

		if _, err := gce.WaitForTrace(ctx, logger, vm, time.Hour); err != nil {
			t.Error(err)
		}
	})
}

func isHealthCheckTestImage(imageSpec string) bool {
	return strings.HasSuffix(imageSpec, "windows-2022") || strings.HasSuffix(imageSpec, "debian-11")
}

func healthCheckResultMessage(name string, result string, code string) string {
	if result == "FAIL" || result == "WARNING" {
		return fmt.Sprintf("[%s Check] Result: %s, Error code: %s", name, result, code)
	}
	return fmt.Sprintf("[%s Check] Result: %s", name, result)
}

func checkExpectedHealthCheckResult(t *testing.T, output string, name string, expected string, code string) {
	if !strings.Contains(output, healthCheckResultMessage(name, expected, code)) {
		t.Errorf("expected %s check to %s", name, expected)
	}
}

func getRecentServiceOutputForImage(imageSpec string) string {
	if gce.IsWindows(imageSpec) {
		cmd := strings.Join([]string{
			"$ServiceStart = (Get-EventLog -LogName 'System' -Source 'Service Control Manager' -EntryType 'Information' -Message '*Google Cloud Ops Agent service entered the running state*' -Newest 1).TimeGenerated",
			"$QueryStart = $ServiceStart - (New-TimeSpan -Seconds 30)",
			"Get-WinEvent -MaxEvents 10 -FilterHashtable @{ Logname='Application'; ProviderName='google-cloud-ops-agent'; StartTime=$QueryStart } | select -ExpandProperty Message",
		}, ";")
		return cmd
	}
	return "sudo systemctl status google-cloud-ops-agent"
}

func listenToPortForImage(vm *gce.VM) string {
	if gce.IsWindows(vm.ImageSpec) {
		cmd := strings.Join([]string{
			`Invoke-WmiMethod -Path 'Win32_Process' -Name Create -ArgumentList 'powershell.exe -Command "$Listener = [System.Net.Sockets.TcpListener]20201; $Listener.Start(); Start-Sleep -Seconds 600"'`,
		}, ";")

		return cmd
	}
	if gce.IsCentOS(vm.ImageSpec) || gce.IsSUSEVM(vm) {
		return "nohup nc -l 20201 1>/dev/null 2>/dev/null &"
	}
	return "nohup nc -l -p 20201 1>/dev/null 2>/dev/null &"
}

func TestPortsAndAPIHealthChecks(t *testing.T) {
	t.Parallel()
	gce.RunForEachImage(t, func(t *testing.T, imageSpec string) {
		t.Parallel()
		if !isHealthCheckTestImage(imageSpec) {
			t.SkipNow()
		}

		customScopes := strings.Join([]string{
			"https://www.googleapis.com/auth/monitoring.read",
			"https://www.googleapis.com/auth/logging.write",
			"https://www.googleapis.com/auth/devstorage.read_write",
		}, ",")
		ctx, dirLog, vm := agents.CommonSetupWithExtraCreateArguments(t, imageSpec, []string{"--scopes", customScopes})
		logger := dirLog.ToMainLog()

		if !gce.IsWindows(vm.ImageSpec) {
			var packages []string
			if gce.IsCentOS(vm.ImageSpec) || gce.IsRHEL(vm.ImageSpec) {
				packages = []string{"nc"}
			} else {
				packages = []string{"netcat"}
			}
			err := agents.InstallPackages(ctx, logger, vm, packages)
			if err != nil {
				t.Fatalf("failed to install %v with err: %s", packages, err)
			}
		}

		if _, err := gce.RunRemotely(ctx, logger, vm, listenToPortForImage(vm)); err != nil {
			t.Fatal(err)
		}
		// Wait for port to be in listen mode.
		time.Sleep(30 * time.Second)

		if err := agents.SetupOpsAgent(ctx, logger, vm, ""); err != nil {
			t.Fatal(err)
		}

		cmdOut, err := gce.RunRemotely(ctx, logger, vm, getRecentServiceOutputForImage(vm.ImageSpec))
		if err != nil {
			t.Fatal(err)
		}

		checkExpectedHealthCheckResult(t, cmdOut.Stdout, "Network", "PASS", "")
		checkExpectedHealthCheckResult(t, cmdOut.Stdout, "Ports", "FAIL", "OtelMetricsPortErr")
		checkExpectedHealthCheckResult(t, cmdOut.Stdout, "API", "FAIL", "MonApiScopeErr")

		query := fmt.Sprintf(`severity="ERROR" AND jsonPayload.code="MonApiScopeErr" AND labels."agent.googleapis.com/health/agentKind"="ops-agent" AND labels."agent.googleapis.com/health/agentVersion"=~"^\d+\.\d+\.\d+.*$" AND labels."agent.googleapis.com/health/schemaVersion"="v1"`)
		if err := gce.WaitForLog(ctx, logger, vm, "ops-agent-health", time.Hour, query); err != nil {
			t.Error(err)
		}
	})
}

func TestNetworkHealthCheck(t *testing.T) {
	t.Parallel()
	gce.RunForEachImage(t, func(t *testing.T, imageSpec string) {
		t.Parallel()
		if !isHealthCheckTestImage(imageSpec) {
			t.SkipNow()
		}

		ctx, logger, vm := setupMainLogAndVM(t, imageSpec)

		if err := agents.SetupOpsAgent(ctx, logger, vm, ""); err != nil {
			t.Fatal(err)
		}

		cmdOut, err := gce.RunRemotely(ctx, logger, vm, getRecentServiceOutputForImage(vm.ImageSpec))
		if err != nil {
			t.Fatal(err)
		}

		checkExpectedHealthCheckResult(t, cmdOut.Stdout, "Network", "PASS", "")
		checkExpectedHealthCheckResult(t, cmdOut.Stdout, "Ports", "PASS", "")
		checkExpectedHealthCheckResult(t, cmdOut.Stdout, "API", "PASS", "")

		if _, err := gce.RunRemotely(ctx, logger, vm, stopCommandForImage(vm.ImageSpec)); err != nil {
			t.Fatal(err)
		}

		// Setting deny egress firewall rule. Waiting to changes to propagate
		if _, err := gce.AddTagToVm(ctx, logger, vm, []string{gce.DenyEgressTrafficTag}); err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Minute)

		if _, err := gce.RunRemotely(ctx, logger, vm, startCommandForImage(vm.ImageSpec)); err != nil {
			t.Fatal(err)
		}

		cmdOut, err = gce.RunRemotely(ctx, logger, vm, getRecentServiceOutputForImage(vm.ImageSpec))
		if err != nil {
			t.Fatal(err)
		}

		checkExpectedHealthCheckResult(t, cmdOut.Stdout, "Network", "FAIL", "LogApiConnErr")
		checkExpectedHealthCheckResult(t, cmdOut.Stdout, "Network", "FAIL", "MonApiConnErr")
		// TODO(b/321220138): restore this once there's a more reliable endpoint.
		// checkExpectedHealthCheckResult(t, cmdOut.Stdout, "Network", "WARNING", "PacApiConnErr")
		checkExpectedHealthCheckResult(t, cmdOut.Stdout, "Network", "WARNING", "DLApiConnErr")
		checkExpectedHealthCheckResult(t, cmdOut.Stdout, "Ports", "PASS", "")
		checkExpectedHealthCheckResult(t, cmdOut.Stdout, "API", "FAIL", "MonApiConnErr")
		checkExpectedHealthCheckResult(t, cmdOut.Stdout, "API", "FAIL", "LogApiConnErr")
	})
}

func TestParsingFailureCheck(t *testing.T) {
	t.Parallel()
	gce.RunForEachImage(t, func(t *testing.T, imageSpec string) {
		t.Parallel()
		if !isHealthCheckTestImage(imageSpec) {
			t.SkipNow()
		}

		ctx, logger, vm := setupMainLogAndVM(t, imageSpec)

		logPath := logPathForImage(vm.ImageSpec)
		config := fmt.Sprintf(`logging:
  receivers:
    mylog_source:
      type: files
      include_paths:
      - %s
  processors:
    json1:
      type: parse_json
      field: message
      time_key: time
      time_format: "%s"
  service:
    pipelines:
      my_pipeline:
        receivers: [mylog_source]
        processors: [json1]
`, logPath, "%Y-%m-%dT%H:%M:.%L%z")

		if err := agents.SetupOpsAgent(ctx, logger, vm, config); err != nil {
			t.Fatal(err)
		}

		line := fmt.Sprintf(`{"log":"{\"level\":\"info\",\"message\":\"start\"}\n","time":"%s"}`, time.Now().UTC().Format(time.RFC3339Nano)) + "\n"
		if err := gce.UploadContent(ctx, logger, vm, strings.NewReader(line), logPath); err != nil {
			t.Fatalf("error writing dummy log line: %v", err)
		}

		query := fmt.Sprintf(`severity="ERROR" AND jsonPayload.code="LogParseErr" AND labels."agent.googleapis.com/health/agentKind"="ops-agent" AND labels."agent.googleapis.com/health/agentVersion"=~"^\d+\.\d+\.\d+.*$" AND labels."agent.googleapis.com/health/schemaVersion"="v1"`)
		if err := gce.WaitForLog(ctx, logger, vm, "ops-agent-health", time.Hour, query); err != nil {
			t.Error(err)
		}

	})
}

func TestNoFluentBitDebugSelfLogs(t *testing.T) {
	t.Parallel()
	gce.RunForEachImage(t, func(t *testing.T, imageSpec string) {
		t.Parallel()
		ctx, logger, vm := agents.CommonSetup(t, imageSpec)

		enableDebugLogLevel := `logging:
  service:
    log_level: debug
`
		if err := agents.SetupOpsAgent(ctx, logger.ToMainLog(), vm, enableDebugLogLevel); err != nil {
			t.Fatal(err)
		}

		// Verifies no fluent-bit debug logs are sent to Cloud Logging due to endless spam.
		// TODO: Remove when b/272779619 is fixed.
		if err := gce.AssertLogMissing(ctx, logger.ToMainLog(), vm, "ops-agent-fluent-bit", time.Hour, `severity="DEBUG"`); err != nil {
			t.Error(err)
		}
	})
}

func TestDisableSelfLogCollection(t *testing.T) {
	t.Parallel()
	gce.RunForEachImage(t, func(t *testing.T, imageSpec string) {
		t.Parallel()
		ctx, logger, vm := agents.CommonSetup(t, imageSpec)

		disableSelfLogCollection := `global:
  default_self_log_file_collection: false
`
		if err := agents.SetupOpsAgent(ctx, logger.ToMainLog(), vm, disableSelfLogCollection); err != nil {
			t.Fatal(err)
		}

		if _, err := gce.RunRemotely(ctx, logger.ToMainLog(), vm, stopCommandForImage(vm.ImageSpec)); err != nil {
			t.Fatal(err)
		}

		time.Sleep(2 * time.Minute)

		if _, err := gce.RunRemotely(ctx, logger.ToMainLog(), vm, startCommandForImage(vm.ImageSpec)); err != nil {
			t.Fatal(err)
		}

		if err := gce.AssertLogMissing(ctx, logger.ToMainLog(), vm, "ops-agent-fluent-bit", 2*time.Minute, `severity="INFO"`); err != nil {
			t.Error(err)
		}

		query := fmt.Sprintf(`severity="INFO" AND labels."agent.googleapis.com/health/agentKind"="ops-agent" AND labels."agent.googleapis.com/health/agentVersion"=~"^\d+\.\d+\.\d+.*$" AND labels."agent.googleapis.com/health/schemaVersion"="v1"`)
		if err := gce.WaitForLog(ctx, logger.ToMainLog(), vm, "ops-agent-health", 3*time.Minute, query); err != nil {
			t.Error(err)
		}
	})
}

func TestBufferLimitSizeOpsAgent(t *testing.T) {
	t.Parallel()
	gce.RunForEachImage(t, func(t *testing.T, imageSpec string) {
		t.Parallel()
		if gce.IsWindows(imageSpec) {
			t.SkipNow()
		}
		ctx, dirLog, vm := agents.CommonSetupWithExtraCreateArguments(t, imageSpec, []string{"--boot-disk-size", "100G"})
		logger := dirLog.ToMainLog()

		logPath := logPathForImage(vm.ImageSpec)
		logsPerSecond := 100000
		config := fmt.Sprintf(`logging:
  receivers:
    log_syslog:
      type: files
      include_paths:
      - %s
      - /var/log/messages
      - /var/log/syslog
  service:
    pipelines:
      default_pipeline:
        receivers: [log_syslog]
        processors: []`, logPath)

		if err := agents.SetupOpsAgent(ctx, logger, vm, config); err != nil {
			t.Fatal(err)
		}
		var bufferDir string
		generateLogPerSecondFile := fmt.Sprintf(`
        x=1
        while [ $x -le %d ]
        do
          echo "Hello world! $x" >> ~/log_%d.log
          ((x++))
        done`, logsPerSecond, logsPerSecond)
		_, err := gce.RunRemotely(ctx, logger, vm, generateLogPerSecondFile)
		if err != nil {
			t.Fatal(err)
		}

		bufferDir = "/var/lib/google-cloud-ops-agent/fluent-bit/buffers/tail.1/"

		generateLogsScript := fmt.Sprintf(`
			mkdir -p %s
			x=1
			while [ $x -le 420 ]
			do
			  cp ~/log_%d.log %s/$x.log
			  ((x++))

			  sleep 1
			done
			du -c %s | cut -f 1 | tail -n 1`, logPath, logsPerSecond, logPath, bufferDir)

		if _, err := gce.AddTagToVm(ctx, dirLog.ToFile("firewall_setup.txt"), vm, []string{gce.DenyEgressTrafficTag}); err != nil {
			t.Fatal(err)
		}

		output, err := gce.RunRemotely(ctx, logger, vm, generateLogsScript)
		if err != nil {
			t.Fatal(err)
		}

		byteCount, err := strconv.Atoi(strings.TrimSuffix(output.Stdout, "\n"))
		if err != nil {
			t.Fatal(err)
		}

		// Threshhold of ~6.5GiB since du returns size in KB
		threshold := 6500000
		if byteCount > threshold {
			t.Fatalf("%d is greater than the allowed threshold %d", byteCount, threshold)
		}

		if _, err := gce.RemoveTagFromVm(ctx, dirLog.ToFile("firewall_setup.txt"), vm, []string{gce.DenyEgressTrafficTag}); err != nil {
			t.Fatal(err)
		}

	})
}

// TestNoNvmlOtelReceiverWithoutGpu check to make sure that Ops Agent's
// hostmetrics receiver does not generate a nvml otel receiver on VMs without
// GPU, by checking the absence of nvml logs from otel
func TestNoNvmlOtelReceiverWithoutGpu(t *testing.T) {
	t.Parallel()
	gce.RunForEachImage(t, func(t *testing.T, imageSpec string) {
		t.Parallel()

		ctx, logger, vm := setupMainLogAndVM(t, imageSpec)
		if err := agents.SetupOpsAgent(ctx, logger, vm, ""); err != nil {
			t.Fatal(err)
		}

		otelConfig, err := retrieveOtelConfig(ctx, logger, vm)
		if err != nil {
			t.Fatal(err)
		}

		if strings.Contains(otelConfig, "nvml") {
			t.Error("expects the nvml receiver not in the Otel configuration for VMs without GPU. Otel config: " + otelConfig)
		}
	})
}

func TestPartialSuccess(t *testing.T) {
	t.Parallel()
	gce.RunForEachImage(t, func(t *testing.T, imageSpec string) {
		t.Parallel()
		ctx, logger, vm := setupMainLogAndVM(t, imageSpec)
		logPath := logPathForImage(vm.ImageSpec)
		config := fmt.Sprintf(`logging:
  receivers:
    files_1:
      type: files
      include_paths: [%s]
  service:
    pipelines:
      p1:
        receivers: [files_1]`, logPath)
		testFile, err := os.Open(path.Join("testdata", "partial_success", "test.log"))
		if err != nil {
			t.Fatal(err)
		}
		defer testFile.Close()
		if err = gce.UploadContent(ctx, logger, vm, testFile, logPath); err != nil {
			t.Fatal(err)
		}
		if err = agents.SetupOpsAgent(ctx, logger, vm, config); err != nil {
			t.Fatal(err)
		}

		// partialSuccess behaviour is documented at: https://cloud.google.com/logging/docs/reference/v2/rest/v2/entries/write
		if err := gce.WaitForLog(ctx, logger, vm, "files_1", time.Hour, `jsonPayload.message="google"`); err != nil {
			t.Error(err)
		}
		if err = gce.WaitForLog(ctx, logger, vm, "files_1", time.Hour, `jsonPayload.message="foo"`); err != nil {
			t.Error(err)
		}
		if err = gce.WaitForLog(ctx, logger, vm, "files_1", time.Hour, `jsonPayload.message="goo"`); err != nil {
			t.Error(err)
		}
	})
}

func TestRestartVM(t *testing.T) {
	t.Parallel()
	gce.RunForEachImage(t, func(t *testing.T, imageSpec string) {
		t.Parallel()

		ctx, dirLog, vm := agents.CommonSetup(t, imageSpec)
		logger := dirLog.ToMainLog()

		if err := agents.SetupOpsAgent(ctx, logger, vm, ""); err != nil {
			t.Fatal(err)
		}

		cmdOut, err := gce.RunRemotely(ctx, logger, vm, getRecentServiceOutputForImage(vm.ImageSpec))
		if err != nil {
			t.Fatal(err)
		}

		// Ensure sure all healthchecks pass before the restart
		checkExpectedHealthCheckResult(t, cmdOut.Stdout, "Network", "PASS", "")
		checkExpectedHealthCheckResult(t, cmdOut.Stdout, "Ports", "PASS", "")
		checkExpectedHealthCheckResult(t, cmdOut.Stdout, "API", "PASS", "")

		logger.Printf(`Restarting instance. For details, see "VM_restart.txt".`)
		if err := gce.RestartInstance(ctx, dirLog.ToFile("VM_restart.txt"), vm); err != nil {
			t.Fatal(err)
		}

		cmdOut, err = gce.RunRemotely(ctx, logger, vm, getRecentServiceOutputForImage(vm.ImageSpec))
		if err != nil {
			t.Fatal(err)
		}

		checkExpectedHealthCheckResult(t, cmdOut.Stdout, "Network", "PASS", "")
		checkExpectedHealthCheckResult(t, cmdOut.Stdout, "Ports", "PASS", "")
		checkExpectedHealthCheckResult(t, cmdOut.Stdout, "API", "PASS", "")
	})
}

func TestLogCompression(t *testing.T) {
	t.Parallel()
	gce.RunForEachImage(t, func(t *testing.T, imageSpec string) {
		t.Parallel()
		ctx, logger, vm := setupMainLogAndVM(t, imageSpec)
		file1 := fmt.Sprintf("%s_1", logPathForImage(vm.ImageSpec))
		config := fmt.Sprintf(`logging:
  receivers:
    f1:
      type: files
      include_paths:
        - %s
  service:
    pipelines:
      p1:
        receivers:
          - f1
`, file1)

		if err := agents.SetupOpsAgent(ctx, logger, vm, config); err != nil {
			t.Fatal(err)
		}

		line := `google` + "\n"
		if err := gce.UploadContent(ctx, logger, vm, strings.NewReader(line), file1); err != nil {
			t.Fatalf("error uploading log: %v", err)
		}

		// Expect to see the log with the modifications applied
		if err := gce.WaitForLog(ctx, logger, vm, "f1", time.Hour, `jsonPayload.message="google"`); err != nil {
			t.Error(err)
		}
	})
}

func TestMain(m *testing.M) {
	code := m.Run()
	gce.CleanupKeysOrDie()
	os.Exit(code)
}

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

package confgenerator

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
	"github.com/GoogleCloudPlatform/ops-agent/internal/healthchecks"
	"github.com/GoogleCloudPlatform/ops-agent/internal/logs"
	"github.com/GoogleCloudPlatform/ops-agent/internal/platform"
	"github.com/GoogleCloudPlatform/ops-agent/internal/version"
)

var (
	agentKind     string = "ops-agent"
	schemaVersion string = "v1"
)

const (
	opsAgentLogsMatch    string = "ops-agent-*"
	fluentBitSelfLogsTag string = "ops-agent-fluent-bit"
	healthLogsTag        string = "ops-agent-health"
	agentVersionKey      string = "agent.googleapis.com/health/agentVersion"
	agentKindKey         string = "agent.googleapis.com/health/agentKind"
	schemaVersionKey     string = "agent.googleapis.com/health/schemaVersion"
)

func fluentbitSelfLogsPath(p platform.Platform) string {
	loggingModule := "logging-module.log"
	if p.Type == platform.Windows {
		return path.Join("${logs_dir}", loggingModule)
	}
	return path.Join("${logs_dir}", "subagents", loggingModule)
}

func healthChecksLogsPath() string {
	return path.Join("${logs_dir}", "health-checks.log")
}

func generateInputHealthLoggingPingComponent(ctx context.Context) []fluentbit.Component {
	return []fluentbit.Component{
		{
			Kind: "INPUT",
			Config: map[string]string{
				"Name":          "dummy",
				"Tag":           healthLogsTag,
				"Dummy":         `{"code": "LogPingOpsAgent", "severity": "DEBUG"}`,
				"Interval_Sec":  "600",
				"Interval_NSec": "0",
			},
		},
	}
}

// This method creates a file input for the `health-checks.log` file, a json parser for the
// structured logs and a grep filter to avoid ingesting previous content of the file.
func generateInputHealthChecksLogsComponents(ctx context.Context) []fluentbit.Component {
	out := make([]fluentbit.Component, 0)
	out = append(out, LoggingReceiverFilesMixin{
		IncludePaths:   []string{healthChecksLogsPath()},
		BufferInMemory: true,
	}.Components(ctx, healthLogsTag)...)
	out = append(out, LoggingProcessorParseJson{
		// TODO(b/282754149): Remove TimeKey and TimeFormat when feature gets implemented.
		ParserShared: ParserShared{
			TimeKey:    logs.TimeZapKey,
			TimeFormat: "%Y-%m-%dT%H:%M:%S%z",
		},
	}.Components(ctx, healthLogsTag, "health-checks-json")...)
	out = append(out, []fluentbit.Component{
		// This is used to exclude any previous content of the `health-checks.log` file that does not contain
		// the `jsonPayload.severity` field. Due to `https://github.com/fluent/fluent-bit/issues/7092` the
		// filtering can't be done directly to the `logging.googleapis.com/severity` field.
		// We cannot use `LoggingProcessorExcludeLogs` here since it doesn't exclude when the field is missing.
		{
			Kind: "FILTER",
			Config: map[string]string{
				"Name":  "grep",
				"Match": healthLogsTag,
				"Regex": fmt.Sprintf("%s INFO|ERROR|WARNING|DEBUG|info|error|warning|debug", logs.SeverityZapKey),
			},
		},
	}...)
	return out
}

// This method creates a file input for the `logging-module.log` file, a regex parser for the
// fluent-bit self logs and a translator of severity to the logging api format.
func generateInputFluentBitSelfLogsComponents(ctx context.Context, logLevel string) []fluentbit.Component {
	out := make([]fluentbit.Component, 0)
	out = append(out, LoggingReceiverFilesMixin{
		IncludePaths: []string{fluentbitSelfLogsPath(platform.FromContext(ctx))},
		//Following: b/226668416 temporarily set storage.type to "memory"
		//to prevent chunk corruption errors
		BufferInMemory: true,
	}.Components(ctx, fluentBitSelfLogsTag)...)
	out = append(out, LoggingProcessorParseRegex{
		Regex:       `(?<message>\[[ ]*(?<time>\d+\/\d+\/\d+ \d+:\d+:\d+)] \[[ ]*(?<severity>[a-z]+)\].*)`,
		PreserveKey: true,
		ParserShared: ParserShared{
			TimeKey:    "time",
			TimeFormat: "%Y/%m/%d %H:%M:%S",
			Types: map[string]string{
				"severity": "string",
			},
		},
	}.Components(ctx, fluentBitSelfLogsTag, "fluent-bit-self-log-regex-parsing")...)
	// Disables sending fluent-bit debug logs to Cloud Logging due to endless spam.
	// TODO: Remove when b/272779619 is fixed.
	if logLevel == "debug" {
		out = append(out, []fluentbit.Component{
			{
				Kind: "FILTER",
				Config: map[string]string{
					"Name":    "grep",
					"Match":   fluentBitSelfLogsTag,
					"Exclude": "severity debug",
				},
			},
		}...)
	}
	return out
}

func generateFilterSelfLogsSamplingComponents(ctx context.Context) []fluentbit.Component {
	out := make([]fluentbit.Component, 0)

	for _, m := range healthchecks.FluentBitSelfLogTranslationList {
		// This filter samples specific fluent-bit logs by matching with regex and re-emits
		// an `ops-agent-health` log.
		out = append(out, fluentbit.Component{
			Kind: "FILTER",
			Config: map[string]string{
				"Name":  "rewrite_tag",
				"Match": fluentBitSelfLogsTag,
				"Rule":  fmt.Sprintf(`message %s %s true`, m.RegexMatch, healthLogsTag),
			},
		})
		// This filter sets the appropiate health code and message to the previously sampled logs.
		out = append(out, fluentbit.Component{
			Kind: "FILTER",
			OrderedConfig: [][2]string{
				{"Name", "modify"},
				{"Match", healthLogsTag},
				{"Condition", fmt.Sprintf(`Key_value_matches message %s`, m.RegexMatch)},
				{"Set", fmt.Sprintf(`code %s`, m.Code)},
				{"Set", fmt.Sprintf(`message "%s"`, m.Message)},
			},
		})
	}

	return out
}

// This method creates a component that enforces the `Structured Health Logs` format to
// all `ops-agent-health` logs. It sets `agentKind`, `agentVersion` and `schemaVersion`.
func generateFilterStructuredHealthLogsComponents(ctx context.Context) []fluentbit.Component {
	return LoggingProcessorModifyFields{
		Fields: map[string]*ModifyField{
			fmt.Sprintf(`labels."%s"`, agentKindKey): {
				StaticValue: &agentKind,
			},
			fmt.Sprintf(`labels."%s"`, agentVersionKey): {
				StaticValue: &version.Version,
			},
			fmt.Sprintf(`labels."%s"`, schemaVersionKey): {
				StaticValue: &schemaVersion,
			},
		},
	}.Components(ctx, healthLogsTag, "set-structured-health-logs")
}

// This method processes all self logs to set the severity field correctly before reaching the output plugin.
func generateFilterMapSeverityFieldComponent(ctx context.Context) []fluentbit.Component {
	return LoggingProcessorModifyFields{
		Fields: map[string]*ModifyField{
			"severity": {
				MoveFrom: "jsonPayload.severity",
				MapValues: map[string]string{
					"error": "ERROR",
					"warn":  "WARNING",
					"info":  "INFO",
					"debug": "DEBUG",
				},
				MapValuesExclusive: false,
			},
		},
	}.Components(ctx, opsAgentLogsMatch, "self-logs-processing")
}

// This method creates a component that outputs all ops-agent self logs to Cloud Logging.
func generateOutputSelfLogsComponent(ctx context.Context, userAgent string, ingestSelfLogs bool) fluentbit.Component {
	outputLogNames := []string{healthLogsTag}
	if ingestSelfLogs {
		// Ingest fluent-bit logs to Cloud Logging if enabled.
		outputLogNames = append(outputLogNames, fluentBitSelfLogsTag)
	}
	return stackdriverOutputComponent(ctx, strings.Join(outputLogNames, "|"), userAgent, "", "")
}

func (uc *UnifiedConfig) generateSelfLogsComponents(ctx context.Context, userAgent string) []fluentbit.Component {
	out := make([]fluentbit.Component, 0)
	out = append(out, generateInputHealthLoggingPingComponent(ctx)...)
	out = append(out, generateInputFluentBitSelfLogsComponents(ctx, uc.Logging.Service.LogLevel)...)
	out = append(out, generateInputHealthChecksLogsComponents(ctx)...)
	out = append(out, generateFilterSelfLogsSamplingComponents(ctx)...)
	out = append(out, generateFilterStructuredHealthLogsComponents(ctx)...)
	out = append(out, generateFilterMapSeverityFieldComponent(ctx)...)
	out = append(out, generateOutputSelfLogsComponent(ctx, userAgent, uc.Global.GetDefaultSelfLogFileCollection()))

	return out
}

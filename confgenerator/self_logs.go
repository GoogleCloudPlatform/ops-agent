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
	"github.com/GoogleCloudPlatform/ops-agent/internal/logs"
	"github.com/GoogleCloudPlatform/ops-agent/internal/platform"
	"github.com/GoogleCloudPlatform/ops-agent/internal/version"
)

var (
	agentKind     string = "ops-agent"
	schemaVersion string = "v1"
)

const (
	opsAgentLogsMatch       string = "ops-agent-*"
	fluentBitSelfLogsTag    string = "ops-agent-fluent-bit"
	healthLogsTag           string = "ops-agent-health"
	severityKey             string = "logging.googleapis.com/severity"
	sourceLocationKey       string = "logging.googleapis.com/sourceLocation"
	agentVersionKey         string = "agent.googleapis.com/health/agentVersion"
	agentKindKey            string = "agent.googleapis.com/health/agentKind"
	schemaVersionKey        string = "agent.googleapis.com/health/schemaVersion"
	troubleshootFindInfoURL string = "https://cloud.google.com/stackdriver/docs/solutions/agents/ops-agent/troubleshoot-find-info"
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

func generateHealthLoggingPingComponent(ctx context.Context) []fluentbit.Component {
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
func generateHealthChecksLogsComponents(ctx context.Context) []fluentbit.Component {
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
func generateFluentBitSelfLogsComponents(ctx context.Context) []fluentbit.Component {
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
	return out
}

type selfLogTranslationEntry struct {
	regexMatch string
	message    string
	code       string
}

var selfLogTranslationList = []selfLogTranslationEntry{
	{
		regexMatch: `\[error\]\s\[lib\]\sbackend\sfailed`,
		message:    fmt.Sprintf("Ops Agent logging pipeline failed, Code: LogPipelineErr, Documentation: %s", troubleshootFindInfoURL),
		code:       "LogPipelineErr",
	},
	{
		regexMatch: `\[error\]\s\[parser\]\scannot\sparse`,
		message:    fmt.Sprintf("Ops Agent failed to parse logs, Code: LogParseErr, Documentation: %s", troubleshootFindInfoURL),
		code:       "LogParseErr",
	},
	{
		regexMatch: `\[ warn\].*\serror\sparsing\slog\smessage\swith\sparser.*`,
		message:    fmt.Sprintf("Ops Agent failed to parse logs, Code: LogParseErr, Documentation: %s", troubleshootFindInfoURL),
		code:       "LogParseErr",
	},
	{
		regexMatch: `\[error\].*\sparsers\sreturned\san\serror.*`,
		message:    fmt.Sprintf("Ops Agent failed to parse logs, Code: LogParseErr, Documentation: %s", troubleshootFindInfoURL),
		code:       "LogParseErr",
	},
	{
		regexMatch: `\[error\].*\sNo\ssuch\sfile\sor\sdirectory.*`,
		message:    fmt.Sprintf("Ops Agent not found path or directory, Code: LogPathNotFound, Documentation: %s", troubleshootFindInfoURL),
		code:       "LogPathNotFound",
	},
}

func generateSelfLogsSamplingComponents(ctx context.Context) []fluentbit.Component {
	out := make([]fluentbit.Component, 0)

	for _, m := range selfLogTranslationList {
		// This filter samples specific fluent-bit logs by matching with regex and re-emits
		// an `ops-agent-health` log.
		out = append(out, fluentbit.Component{
			Kind: "FILTER",
			Config: map[string]string{
				"Name":  "rewrite_tag",
				"Match": fluentBitSelfLogsTag,
				"Rule":  fmt.Sprintf(`message %s %s true`, m.regexMatch, healthLogsTag),
			},
		})
		// This filter sets the appropiate health code to the previously sampled logs. The `code` is also
		// set to the `message` field for later translation in the pipeline.
		// The current fluent-bit submodule doesn't accept whitespaces in the `Set` values, so `code` is
		// used as a placeholder. This can be updated when the fix arrives to the current fluent-bit submodule
		// `https://github.com/fluent/fluent-bit/issues/4286`.
		out = append(out, fluentbit.Component{
			Kind: "FILTER",
			OrderedConfig: [][2]string{
				{"Name", "modify"},
				{"Match", healthLogsTag},
				{"Condition", fmt.Sprintf(`Key_value_matches message %s`, m.regexMatch)},
				{"Set", fmt.Sprintf(`message %s`, m.code)},
				{"Set", fmt.Sprintf(`code %s`, m.code)},
			},
		})
	}

	return out
}

// This method creates a component that enforces the `Structured Health Logs` format to
// all `ops-agent-health` logs. It sets `agentKind`, `agentVersion` and `schemaVersion`.
// It also translates `code` to the rich text message from the `selfLogTranslationList`.
func generateStructuredHealthLogsComponents(ctx context.Context) []fluentbit.Component {
	// Convert translation list to map.
	mapMessageFromCode := make(map[string]string)
	for _, m := range selfLogTranslationList {
		mapMessageFromCode[m.code] = m.message
	}

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
			"jsonPayload.message": {
				MapValues:          mapMessageFromCode,
				MapValuesExclusive: false,
			},
		},
	}.Components(ctx, healthLogsTag, "set-structured-health-logs")
}

// This method processes all self logs to set the fields correctly before reaching the output plugin.
func generateSelfLogsProcessingComponents(ctx context.Context) []fluentbit.Component {
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

func (uc *UnifiedConfig) generateSelfLogsComponents(ctx context.Context, userAgent string) []fluentbit.Component {
	out := make([]fluentbit.Component, 0)
	out = append(out, generateHealthLoggingPingComponent(ctx)...)
	out = append(out, generateFluentBitSelfLogsComponents(ctx)...)
	out = append(out, generateHealthChecksLogsComponents(ctx)...)
	out = append(out, generateSelfLogsSamplingComponents(ctx)...)
	out = append(out, generateStructuredHealthLogsComponents(ctx)...)
	out = append(out, generateSelfLogsProcessingComponents(ctx)...)

	outputLogNames := []string{healthLogsTag}
	if uc.Global.GetDefaultSelfLogFileCollection() {
		// Ingest fluent-bit logs to Cloud Logging if enabled.
		outputLogNames = append(outputLogNames, fluentBitSelfLogsTag)
	}
	out = append(out, stackdriverOutputComponent(ctx, strings.Join(outputLogNames, "|"), userAgent, "", ""))
	return out
}

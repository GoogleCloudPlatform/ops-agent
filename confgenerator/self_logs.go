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
	"github.com/GoogleCloudPlatform/ops-agent/internal/platform"
	"github.com/GoogleCloudPlatform/ops-agent/internal/version"
)

const (
	opsAgentLogsMatch    string = "ops-agent-*"
	fluentBitSelfLogsTag string = "ops-agent-fluent-bit"
)

func fluentbitSelfLogsPath(p platform.Platform) string {
	loggingModule := "logging-module.log"
	if p.Type == platform.Windows {
		return path.Join("${logs_dir}", loggingModule)
	}
	return path.Join("${logs_dir}", "subagents", loggingModule)
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
		Regex:       `(?<message>\[[ ]*(?<time>\d+\/\d+\/\d+ \d+:\d+:\d+)(?:\.\d+)?\] \[[ ]*(?<severity>[a-z]+)\].*)`,
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
	out = append(out, generateInputFluentBitSelfLogsComponents(ctx, uc.Logging.Service.LogLevel)...)
	out = append(out, generateFilterSelfLogsSamplingComponents(ctx)...)
	out = append(out, generateFilterStructuredHealthLogsComponents(ctx)...)
	out = append(out, generateFilterMapSeverityFieldComponent(ctx)...)
	out = append(out, generateOutputSelfLogsComponent(ctx, userAgent, uc.Global.GetDefaultSelfLogFileCollection()))

	return out
}

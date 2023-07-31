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
	fluentBitSelfLogsTag string = "ops-agent-fluent-bit"
	healthLogsTag        string = "ops-agent-health"
	severityKey          string = "logging.googleapis.com/severity"
	sourceLocationKey    string = "logging.googleapis.com/sourceLocation"
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
	out = append(out, LoggingProcessorExcludeLogs{
		// This is used to exclude any previous content of the health-checks file that
		// does not contain the `severity` field.
		MatchAny: []string{`severity !~ "INFO|ERROR|WARNING|DEBUG"`},
	}.Components(ctx, healthLogsTag, "health-checks-exclude")...)
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
	}.Components(ctx, fluentBitSelfLogsTag, "self-logs-severity")...)
	out = append(out, LoggingProcessorModifyFields{
		Fields: map[string]*ModifyField{
			"severity": {
				MoveFrom: "jsonPayload.severity",
				MapValues: map[string]string{
					"error": "ERROR",
					"warn":  "WARNING",
					"info":  "INFO",
					"debug": "DEBUG",
				},
				MapValuesExclusive: true,
			},
		},
	}.Components(ctx, fluentBitSelfLogsTag, "mapseverityvalues")...)
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
		message:    "Ops Agent Logging Pipeline Failed, Documentation: https://cloud.google.com/logging/docs/agent/ops-agent/troubleshoot-find-info#health-checks",
		code:       "LogPipelineErr",
	},
	{
		regexMatch: `\[error\]\s\[parser\]\scannot\sparse`,
		message:    "Ops Agent Failed to Parse Logs, Documentation: https://cloud.google.com/logging/docs/agent/ops-agent/troubleshoot-find-info#health-checks",
		code:       "LogParseErr",
	},
}

func generateSelfLogsSamplingComponents(ctx context.Context) []fluentbit.Component {
	out := make([]fluentbit.Component, 0)
	mapMessageFromCode := make(map[string]string)

	// This filters sample specific fluent-bit logs by matching with regex, reemits
	// as an `ops-agent-health` log and modifies them to follow the required structure.
	for _, m := range selfLogTranslationList {
		out = append(out, fluentbit.Component{
			Kind: "FILTER",
			Config: map[string]string{
				"Name":  "rewrite_tag",
				"Match": fluentBitSelfLogsTag,
				"Rule":  fmt.Sprintf(`message %s %s true`, m.regexMatch, healthLogsTag),
			},
		})
		out = append(out, fluentbit.Component{
			Kind: "FILTER",
			OrderedConfig: [][2]string{
				{"Name", "modify"},
				{"Match", healthLogsTag},
				{"Condition", fmt.Sprintf(`Key_value_matches message %s`, m.regexMatch)},
				{"Set", fmt.Sprintf(`code %s`, m.code)},
			},
		})
		mapMessageFromCode[m.code] = m.message
	}
	out = append(out, LoggingProcessorModifyFields{
		Fields: map[string]*ModifyField{
			"jsonPayload.message": {
				CopyFrom: "jsonPayload.code",
				MapValues: mapMessageFromCode,
				MapValuesExclusive: false,
			},
		},
	}.Components(ctx, healthLogsTag, "mapmessagesfromcode")...)

	return out
}

// This method creates a component adds metadata labels to all ops agent health logs.
func generateStructuredHealthLogsComponents(ctx context.Context) []fluentbit.Component {
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
	}.Components(ctx, healthLogsTag, "setstructuredhealthlogs")
}

func generateSelfLogsComponents(ctx context.Context, userAgent string) []fluentbit.Component {
	out := make([]fluentbit.Component, 0)
	out = append(out, generateFluentBitSelfLogsComponents(ctx)...)
	out = append(out, generateHealthChecksLogsComponents(ctx)...)
	out = append(out, generateSelfLogsSamplingComponents(ctx)...)
	out = append(out, generateStructuredHealthLogsComponents(ctx)...)
	out = append(out, stackdriverOutputComponent(strings.Join([]string{fluentBitSelfLogsTag, healthLogsTag}, "|"), userAgent, ""))
	return out
}

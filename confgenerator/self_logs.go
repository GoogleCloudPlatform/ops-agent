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
		MatchAny: []string{`severity!~INFO|ERROR|WARNING|DEBUG`},
	}.Components(ctx, healthLogsTag, "health-checs-exclude")...)
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

type healthLogsMatch struct {
	key     string
	match   string
	message string
	code    string
}

var healthLogsMatchList = []healthLogsMatch{
	{
		key:     "message",
		match:   `\[error\]\s\[lib\]\sbackend\sfailed`,
		message: "LogggingPipelineStoppedWorking-PleaseVerifyYourOpsAgentConfig",
		code:    "LogPipelineErr",
	},
	{
		key:     "message",
		match:   `\[error\]\s\[parser\]\scannot\sparse`,
		message: "IncorrectParsingConfiguration-PleaseVerifyYourOpsAgentConfig",
		code:    "LogParseErr",
	},
}

func generateSampleSelfLogsComponents(ctx context.Context) []fluentbit.Component {
	out := make([]fluentbit.Component, 0)

	// This filter will throttle all `ops-agent-health` logs by limiting the processing
	// and ingestiong to 10 logs every 30 minutes. 
	out = append(out, fluentbit.Component{
		Kind: "FILTER",
		Config: map[string]string{
			"Name":     "throttle",
			"Match":    healthLogsTag,
			"Rate":     "10",
			"Window":   "1800", // 30 minutes
			"Interval": "1s",
		},
	})

	// This filters sample specific fluent-bit logs by matching with regex, reemits
	// as an `ops-agent-health` log and modifies them to follow the required structure.
	for _, m := range healthLogsMatchList {
		out = append(out, fluentbit.Component{
			Kind: "FILTER",
			Config: map[string]string{
				"Name":  "rewrite_tag",
				"Match": fluentBitSelfLogsTag,
				"Rule":  fmt.Sprintf(`%s %s %s true`, m.key, m.match, healthLogsTag),
			},
		})
		out = append(out, fluentbit.Component{
			Kind: "FILTER",
			OrderedConfig: [][2]string{
				{"Name", "modify"},
				{"Match", healthLogsTag},
				{"Condition", fmt.Sprintf(`Key_value_matches %s %s`, m.key, m.match)},
				{"Set", fmt.Sprintf(`message %s`, m.message)},
				{"Set", fmt.Sprintf(`code %s`, m.code)},
			},
		})
	}

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
	out = append(out, generateSampleSelfLogsComponents(ctx)...)
	out = append(out, generateStructuredHealthLogsComponents(ctx)...)
	out = append(out, stackdriverOutputComponent(strings.Join([]string{fluentBitSelfLogsTag, healthLogsTag}, "|"), userAgent, ""))
	return out
}

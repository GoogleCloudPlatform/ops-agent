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

// Package confgenerator represents the Ops Agent configuration and provides functions to generate subagents configuration from unified agent.
package confgenerator

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
	"github.com/GoogleCloudPlatform/ops-agent/internal/platform"
	"github.com/GoogleCloudPlatform/ops-agent/internal/version"
)

const (
	fluentBitSelfLogsTag = "ops-agent-fluent-bit"
	healthLogsTag       = "ops-agent-health"
	severityKey         = "logging.googleapis.com/severity"
	sourceLocationKey   = "logging.googleapis.com/sourceLocation"
	agentVersionKey     = "agent.googleapis.com/health/agentVersion"
	agentKindKey        = "agent.googleapis.com/health/agentKind"
	schemaVersionKey    = "agent.googleapis.com/health/schemaVersion"
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
			TimeKey:    "time",
			TimeFormat: "%Y-%m-%dT%H:%M:%S%z",
		},
	}.Components(ctx, healthLogsTag, "health-checks-json")...)
	out = append(out, []fluentbit.Component{
		// This is used to exclude any previous content of the health-checks file that
		// does not contain the `severity` field.
		{
			Kind: "FILTER",
			Config: map[string]string{
				"Name":  "grep",
				"Match": healthLogsTag,
				"Regex": "severity INFO|ERROR|WARNING|DEBUG|DEFAULT",
			},
		},
		{
			Kind: "FILTER",
			OrderedConfig: [][2]string{
				{"Name", "modify"},
				{"Match", healthLogsTag},
				{"Rename", fmt.Sprintf("severity %s", severityKey)},
				{"Rename", fmt.Sprintf("sourceLocation %s", sourceLocationKey)},
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
	}.Components(ctx, fluentBitSelfLogsTag, "self-logs-severity")...)
	out = append(out, fluentbit.TranslationComponents(fluentBitSelfLogsTag, "severity", severityKey, true,
		[]struct{ SrcVal, DestVal string }{
			{"debug", "DEBUG"},
			{"error", "ERROR"},
			{"info", "INFO"},
			{"warn", "WARNING"},
		})...,
	)
	return out
}

func structuredHealthLogsLabels() map[string]string {
	return map[string]string{
		agentKindKey: "ops-agent",
		agentVersionKey: version.Version,
		schemaVersionKey: "v1",
	}
}

func generateSelfLogsComponents(ctx context.Context, userAgent string) []fluentbit.Component {
	out := make([]fluentbit.Component, 0)

	out = append(out, generateFluentBitSelfLogsComponents(ctx)...)
	out = append(out, generateHealthChecksLogsComponents(ctx)...)

	outputTags := strings.Join([]string{fluentBitSelfLogsTag, healthLogsTag}, "|")
	out = append(out, stackdriverOutputComponent(outputTags, userAgent, "", structuredHealthLogsLabels()))
	return out
}

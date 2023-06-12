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

// Package confgenerator provides functions to generate subagents configuration from unified agent.
package confgenerator

import (
	"context"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
	"github.com/GoogleCloudPlatform/ops-agent/internal/platform"
)

const selfLogsTag = "ops-agent-health"

func generateHealthChecksLogsParser(ctx context.Context) []fluentbit.Component {
	out := make([]fluentbit.Component, 0)
	out = append(out, LoggingProcessorParseJson{
		// TODO(b/282754149): Remove TimeKey and TimeFormat when feature gets implemented.
		ParserShared: ParserShared{
			TimeKey:    "time",
			TimeFormat: "%Y-%m-%dT%H:%M:%S%z",
		},
	}.Components(ctx, selfLogsTag, "health-logs-version")...)
	out = append(out, []fluentbit.Component{
		// This is used to exclude any previous content of the health-checks file that
		// does not contain the ops-agent-version field.
		{
			Kind: "FILTER",
			Config: map[string]string{
				"Name":  "grep",
				"Match": selfLogsTag,
				"Regex": "agent-version ^.*",
			},
		},
	}...)
	return out
}

func generateFluentBitSelfLogsParser(ctx context.Context) []fluentbit.Component {
	out := make([]fluentbit.Component, 0)

	parser := LoggingProcessorParseRegex{
		Regex:       `(?<message>\[[ ]*(?<time>\d+\/\d+\/\d+ \d+:\d+:\d+)] \[[ ]*(?<severity>[a-z]+)\].*)`,
		PreserveKey: true,
		ParserShared: ParserShared{
			TimeKey:    "time",
			TimeFormat: "%Y/%m/%d %H:%M:%S",
			Types: map[string]string{
				"severity": "string",
			},
		},
	}.Components(ctx, selfLogsTag, "self-logs-severity")

	out = append(out, parser...)

	out = append(out, fluentbit.TranslationComponents(selfLogsTag, "severity", "logging.googleapis.com/severity", true,
		[]struct{ SrcVal, DestVal string }{
			{"debug", "DEBUG"},
			{"error", "ERROR"},
			{"info", "INFO"},
			{"warn", "WARNING"},
		})...,
	)
	return out
}

func generateSelfLogComponents(ctx context.Context, userAgent string) []fluentbit.Component {
	out := make([]fluentbit.Component, 0)
	out = append(out, LoggingReceiverFilesMixin{
		IncludePaths: []string{fluentbitSelfLogsPath(platform.FromContext(ctx))},
		//Following: b/226668416 temporarily set storage.type to "memory"
		//to prevent chunk corruption errors
		BufferInMemory: true,
	}.Components(ctx, selfLogsTag)...)

	out = append(out, LoggingReceiverFilesMixin{
		IncludePaths:   []string{healthChecksLogsPath()},
		BufferInMemory: true,
	}.Components(ctx, selfLogsTag)...)

	out = append(out, generateFluentBitSelfLogsParser(ctx)...)
	out = append(out, generateHealthChecksLogsParser(ctx)...)

	out = append(out, stackdriverOutputComponent(selfLogsTag, userAgent, ""))

	return out
}
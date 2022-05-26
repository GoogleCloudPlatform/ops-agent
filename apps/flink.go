// Copyright 2021 Google LLC
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

package apps

import (
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
)

type LoggingProcessorFlink struct {
	confgenerator.ConfigComponent `yaml:",inline"`
}

func (LoggingProcessorFlink) Type() string {
	return "flink"
}

func (p LoggingProcessorFlink) Components(tag string, uid string) []fluentbit.Component {
	c := confgenerator.LoggingProcessorParseMultilineRegex{
		LoggingProcessorParseRegexComplex: confgenerator.LoggingProcessorParseRegexComplex{
			Parsers: []confgenerator.RegexParser{
				{
					// Standalone session example
					// Sample line: 2022-04-22 11:51:35,718 INFO  org.apache.flink.runtime.jobmaster.JobMaster                 [] - Close ResourceManager connection 668abb5d496646a153262b5896fd935d: Stopping JobMaster for job 'Streaming WordCount' (2538c8dff66c8cf6ec58ad32b149e23f).

					// Taskexecutor example
					// 2022-04-23 16:13:05,459 INFO  org.apache.flink.runtime.taskexecutor.TaskExecutor           [] - Could not resolve ResourceManager address akka.tcp://flink@localhost:6123/user/rpc/resourcemanager_*, retrying in 10000 ms: Could not connect to rpc endpoint under address akka.tcp://flink@localhost:6123/user/rpc/resourcemanager_*.

					// Client example
					// Sample line: 2022-04-22 11:51:32,901 INFO  org.apache.flink.client.program.rest.RestClusterClient       [] - Submitting job 'Streaming WordCount' (2538c8dff66c8cf6ec58ad32b149e23f).

					Regex: `^(?<time>\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2},\d+)\s+(?<level>[A-Z]+)\s+(?<source>[^ ]*)(?<message>[\s\S]*)`,
					Parser: confgenerator.ParserShared{
						TimeKey:    "time",
						TimeFormat: "%Y-%m-%d %H:%M:%S,%L",
					},
				},
			},
		},
		Rules: []confgenerator.MultilineRule{
			{
				StateName: "start_state",
				NextState: "cont",
				Regex:     `\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2},\d+`,
			},
			{
				StateName: "cont",
				NextState: "cont",
				Regex:     `^(?!\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2},\d+)`,
			},
		},
	}.Components(tag, uid)

	// Log levels are just log4j log levels
	// https://logging.apache.org/log4j/2.x/log4j-api/apidocs/org/apache/logging/log4j/Level.html
	c = append(c,
		fluentbit.TranslationComponents(tag, "level", "logging.googleapis.com/severity", true,
			[]struct{ SrcVal, DestVal string }{
				{"TRACE", "TRACE"},
				{"DEBUG", "DEBUG"},
				{"INFO", "INFO"},
				{"ERROR", "ERROR"},
				{"WARN", "WARNING"},
				{"FATAL", "CRITICAL"},
			},
		)...,
	)

	return c
}

type LoggingReceiverFlink struct {
	LoggingProcessorFlink                   `yaml:",inline"`
	confgenerator.LoggingReceiverFilesMixin `yaml:",inline" validate:"structonly"`
}

func (r LoggingReceiverFlink) Components(tag string) []fluentbit.Component {
	if len(r.IncludePaths) == 0 {
		r.IncludePaths = []string{
			"/opt/flink/log/flink-*-standalonesession-*.log",
			"/opt/flink/log/flink-*-taskexecutor-*.log",
			"/opt/flink/log/flink-*-client-*.log",
		}
	}
	c := r.LoggingReceiverFilesMixin.Components(tag)
	c = append(c, r.LoggingProcessorFlink.Components(tag, "flink")...)
	return c
}

func init() {
	confgenerator.LoggingProcessorTypes.RegisterType(func() confgenerator.Component { return &LoggingProcessorFlink{} })
	confgenerator.LoggingReceiverTypes.RegisterType(func() confgenerator.Component { return &LoggingReceiverFlink{} })
}

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
	"fmt"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
)

type MetricsReceiverHbase struct {
	confgenerator.ConfigComponent                 `yaml:",inline"`
	confgenerator.MetricsReceiverSharedJVM        `yaml:",inline"`
	confgenerator.MetricsReceiverSharedCollectJVM `yaml:",inline"`
}

const defaultHbaseEndpoint = "localhost:10101"

func (r MetricsReceiverHbase) Type() string {
	return "hbase"
}

func (r MetricsReceiverHbase) Pipelines() []otel.Pipeline {
	targetSystem := "hbase"
	if r.MetricsReceiverSharedCollectJVM.ShouldCollectJVMMetrics() {
		targetSystem = fmt.Sprintf("%s,%s", targetSystem, "jvm")
	}

	return r.MetricsReceiverSharedJVM.
		WithDefaultEndpoint(defaultHbaseEndpoint).
		ConfigurePipelines(
			targetSystem,
			[]otel.Component{
				otel.NormalizeSums(),
				otel.MetricsTransform(
					otel.AddPrefix("workload.googleapis.com"),
					otel.UpdateMetric("hbase.region_server.*",
						otel.AggregateLabels("max", "state"),
					),
				),
			},
		)
}

func init() {
	confgenerator.MetricsReceiverTypes.RegisterType(func() confgenerator.Component { return &MetricsReceiverHbase{} })
}

type LoggingProcessorHbaseSystem struct {
	confgenerator.ConfigComponent `yaml:",inline"`
}

func (LoggingProcessorHbaseSystem) Type() string {
	return "hbase_system"
}

func (p LoggingProcessorHbaseSystem) Components(tag string, uid string) []fluentbit.Component {
	c := confgenerator.LoggingProcessorParseMultilineRegex{
		LoggingProcessorParseRegexComplex: confgenerator.LoggingProcessorParseRegexComplex{
			Parsers: []confgenerator.RegexParser{
				{
					// Sample line: 2022-01-20 20:38:18,856 INFO  [main] master.HMaster: STARTING service HMaster
					// Sample line: 2022-01-20 20:38:20,304 INFO  [main] metrics.MetricRegistries: Loaded MetricRegistries class org.apache.hadoop.hbase.metrics.impl.MetricRegistriesImpl
					// Sample line: 2022-01-20 20:38:20,385 WARN  [main] util.NativeCodeLoader: Unable to load native-hadoop library for your platform... using builtin-java classes where applicable
					Regex: `^(?<time>\d{4}-\d{2}-\d{2}\s\d{2}:\d{2}:\d{2}\,\d{3,6})\s(?<level>[A-Z]+)\s{2}\[(?<module>[^\]]+)\]\s(?<message>(?<source>[\w\.]+)[^\n]+)`,
					Parser: confgenerator.ParserShared{
						TimeKey: "time",
						//
						TimeFormat: "%Y-%m-%d %H:%M:%S,%L",
					},
				},
			},
		},
		Rules: []confgenerator.MultilineRule{
			{
				StateName: "start_state",
				NextState: "cont",
				Regex:     `\d{4}-\d{2}-\d{2}\s\d{2}:\d{2}:\d{2}\,\d{3,6}`,
			},
			{
				StateName: "cont",
				NextState: "cont",
				Regex:     `^(?!\d{4}-\d{2}-\d{2}\s\d{2}:\d{2}:\d{2}\,\d{3,6})`,
			},
		},
	}.Components(tag, uid)

	// https://hadoop.apache.org/docs/r2.7.0/hadoop-project-dist/hadoop-common/CommandsManual.html
	c = append(c,
		fluentbit.TranslationComponents(tag, "level", "logging.googleapis.com/severity", true,
			[]struct{ SrcVal, DestVal string }{
				{"TRACE", "DEBUG"},
				{"DEBUG", "DEBUG"},
				{"INFO", "INFO"},
				{"WARN", "WARNING"},
				{"ERROR", "ERROR"},
				{"FATAL", "CRITICAL"},
			},
		)...,
	)
	return c
}

type SystemLoggingReceiverHbase struct {
	LoggingProcessorHbaseSystem             `yaml:",inline"`
	confgenerator.LoggingReceiverFilesMixin `yaml:",inline" validate:"structonly"`
}

func (r SystemLoggingReceiverHbase) Components(tag string) []fluentbit.Component {
	if len(r.IncludePaths) == 0 {
		r.IncludePaths = []string{
			"/opt/hbase/logs/hbase-*-regionserver-*.log",
			"/opt/hbase/logs/hbase-*-master-*.log",
		}
	}
	c := r.LoggingReceiverFilesMixin.Components(tag)
	c = append(c, r.LoggingProcessorHbaseSystem.Components(tag, "hbase_system")...)
	return c
}

func init() {
	confgenerator.LoggingProcessorTypes.RegisterType(func() confgenerator.Component { return &LoggingProcessorHbaseSystem{} })
	confgenerator.LoggingReceiverTypes.RegisterType(func() confgenerator.Component { return &SystemLoggingReceiverHbase{} })
}

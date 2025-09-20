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
	"context"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
)

type MetricsReceiverSolr struct {
	confgenerator.ConfigComponent `yaml:",inline"`

	confgenerator.MetricsReceiverSharedJVM `yaml:",inline"`
}

const defaultSolrEndpoint = "localhost:18983"

func (r MetricsReceiverSolr) Type() string {
	return "solr"
}

func (r MetricsReceiverSolr) Pipelines(_ context.Context) ([]otel.ReceiverPipeline, error) {
	targetSystem := "solr"

	return r.MetricsReceiverSharedJVM.
		WithDefaultEndpoint(defaultSolrEndpoint).
		WithServiceName(targetSystem).
		ConfigurePipelines(
			targetSystem,
			[]otel.Component{
				otel.NormalizeSums(),
				otel.MetricsTransform(
					otel.AddPrefix("workload.googleapis.com"),
				),
				otel.TransformationMetrics(
					otel.SetScopeName("agent.googleapis.com/"+r.Type()),
					otel.SetScopeVersion("1.0"),
				),
			},
		)
}

func init() {
	confgenerator.MetricsReceiverTypes.RegisterType(func() confgenerator.MetricsReceiver { return &MetricsReceiverSolr{} })
}

type LoggingProcessorMacroSolrSystem struct {
	confgenerator.ConfigComponent `yaml:",inline"`
}

func (LoggingProcessorMacroSolrSystem) Type() string {
	return "solr_system"
}

func (p LoggingProcessorMacroSolrSystem) Expand(ctx context.Context) []confgenerator.InternalLoggingProcessor {
	return []confgenerator.InternalLoggingProcessor{
		confgenerator.LoggingProcessorParseMultilineRegex{
			LoggingProcessorParseRegexComplex: confgenerator.LoggingProcessorParseRegexComplex{
				Parsers: []confgenerator.RegexParser{
					{
						// Sample line: 2022-01-06 04:16:08.794 INFO  (qtp1489933928-64) [   x:gettingstarted] o.a.s.c.S.Request [gettingstarted]  webapp=/solr path=/get params={q=*:*&_=1641440398872} status=0 QTime=2
						Regex: `^(?<timestamp>\d{4}-\d{2}-\d{2}\s\d{2}:\d{2}:\d{2}\.\d{3,6})\s(?<level>[A-z]+)\s{1,5}\((?<thread>[^\)]+)\)\s\[c?:?(?<collection>[^\s]*)\ss?:?(?<shard>[^\s]*)\sr?:?(?<replica>[^\s]*)\sx?:?(?<core>[^\]]*)\]\s(?<source>[^\s]+)\s(?<message>(?:(?!\s\=\>)[\s\S])+)\s?=?>?(?<exception>[\s\S]*)`,
						Parser: confgenerator.ParserShared{
							TimeKey:    "timestamp",
							TimeFormat: "%Y-%m-%d %H:%M:%S.%L",
						},
					},
				},
			},
			Rules: []confgenerator.MultilineRule{
				{
					StateName: "start_state",
					NextState: "cont",
					Regex:     `^\d{4}-\d{2}-\d{2}\s\d{2}:\d{2}:\d{2}\.\d{3}\s[A-z]+\s{1,5}`,
				},
				{
					StateName: "cont",
					NextState: "cont",
					Regex:     `^(?!\d{4}-\d{2}-\d{2}\s\d{2}:\d{2}:\d{2}\.\d{3}\s[A-z]+\s{1,5})`,
				},
			},
		},
		// https://solr.apache.org/guide/6_6/configuring-logging.html
		confgenerator.LoggingProcessorModifyFields{
			Fields: map[string]*confgenerator.ModifyField{
				"severity": {
					CopyFrom: "jsonPayload.level",
					MapValues: map[string]string{
						"TRACE": "DEBUG",
						"DEBUG": "DEBUG",
						"INFO":  "INFO",
						"WARN":  "WARNING",
						"ERROR": "ERROR",
						"FATAL": "CRITICAL",
					},
					MapValuesExclusive: true,
				},
				InstrumentationSourceLabel: instrumentationSourceValue(p.Type()),
			},
		},
	}
}

func loggingReceiverFilesMixinSolrSystem() confgenerator.LoggingReceiverFilesMixin {
	return confgenerator.LoggingReceiverFilesMixin{
		IncludePaths: []string{
			"/var/solr/logs/solr.log",
		},
	}
}

func init() {
	confgenerator.RegisterLoggingFilesProcessorMacro[LoggingProcessorMacroSolrSystem](loggingReceiverFilesMixinSolrSystem)
}

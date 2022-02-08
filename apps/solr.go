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
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
)

type MetricsReceiverSolr struct {
	confgenerator.ConfigComponent `yaml:",inline"`

	confgenerator.MetricsReceiverShared `yaml:",inline"`

	confgenerator.MetricsReceiverSharedJVM `yaml:",inline"`
}

const defaultSolrEndpoint = "localhost:18983"

func (r MetricsReceiverSolr) Type() string {
	return "solr"
}

func (r MetricsReceiverSolr) Pipelines() []otel.Pipeline {
	targetSystem := "solr"

	return r.MetricsReceiverSharedJVM.JVMConfig(
		targetSystem,
		defaultSolrEndpoint,
		r.CollectionIntervalString(),
		[]otel.Component{
			otel.NormalizeSums(),
			otel.MetricsTransform(
				otel.AddPrefix("workload.googleapis.com"),
			),
		},
	)
}

func init() {
	confgenerator.MetricsReceiverTypes.RegisterType(func() confgenerator.Component { return &MetricsReceiverSolr{} })
}

type LoggingProcessorSolrSystem struct {
	confgenerator.ConfigComponent `yaml:",inline"`
}

func (LoggingProcessorSolrSystem) Type() string {
	return "solr_system"
}

func (p LoggingProcessorSolrSystem) Components(tag string, uid string) []fluentbit.Component {
	c := confgenerator.LoggingProcessorParseMultilineRegex{
		LoggingProcessorParseRegexComplex: confgenerator.LoggingProcessorParseRegexComplex{
			Parsers: []confgenerator.RegexParser{
				{
					// Sample line: 2022-01-06 04:16:08.794 INFO  (qtp1489933928-64) [   x:gettingstarted] o.a.s.c.S.Request [gettingstarted]  webapp=/solr path=/get params={q=*:*&_=1641440398872} status=0 QTime=2
					Regex: `^(?<timestamp>\d{4}-\d{2}-\d{2}\s\d{2}:\d{2}:\d{2}\.\d{3,6})\s(?<level>[A-z]+)\s{2}\((?<thread>[^\)]+)\)\s\[c?:?(?<collection>[^\s]*)\ss?:?(?<shard>[^\s]*)\sr?:?(?<replica>[^\s]*)\sx?:?(?<core>[^\]]*)\]\s(?<source>[^\s]+)\s(?<message>(?:(?!\s\=\>)[\s\S])+)\s?=?>?(?<exception>[\s\S]*)`,
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
				Regex:     `\d{4}-\d{2}-\d{2}\s\d{2}:\d{2}:\d{2}\.\d{3}\s[A-z]+\s{2}`,
			},
			{
				StateName: "cont",
				NextState: "cont",
				Regex:     `^(?!\d{4}-\d{2}-\d{2}\s\d{2}:\d{2}:\d{2}\.\d{3}\s[A-z]+\s{2})`,
			},
		},
	}.Components(tag, uid)

	// https://solr.apache.org/guide/6_6/configuring-logging.html
	c = append(c,
		fluentbit.TranslationComponents(tag, "level", "logging.googleapis.com/severity", true,
			[]struct{ SrcVal, DestVal string }{
				{"FINEST", "DEBUG"},
				{"FINE", "DEBUG"},
				{"CONFIG", "ERROR"},
				{"INFO", "INFO"},
				{"WARN", "WARNING"},
				{"SEVERE", "CRITICAL"},
			},
		)...,
	)
	return c
}

type LoggingReceiverSolrSystem struct {
	LoggingProcessorSolrSystem              `yaml:",inline"`
	confgenerator.LoggingReceiverFilesMixin `yaml:",inline" validate:"structonly"`
}

func (r LoggingReceiverSolrSystem) Components(tag string) []fluentbit.Component {
	if len(r.IncludePaths) == 0 {
		r.IncludePaths = []string{
			"/var/solr/logs/solr.log",
		}
	}
	c := r.LoggingReceiverFilesMixin.Components(tag)
	c = append(c, r.LoggingProcessorSolrSystem.Components(tag, "solr_system")...)
	return c
}

func init() {
	confgenerator.LoggingProcessorTypes.RegisterType(func() confgenerator.Component { return &LoggingProcessorSolrSystem{} })
	confgenerator.LoggingReceiverTypes.RegisterType(func() confgenerator.Component { return &LoggingReceiverSolrSystem{} })
}

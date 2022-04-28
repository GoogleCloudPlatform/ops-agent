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

type LoggingProcessorJettySystem struct {
	confgenerator.ConfigComponent `yaml:",inline"`
}

func (LoggingProcessorJettySystem) Type() string {
	return "jetty_system"
}

func (p LoggingProcessorJettySystem) Components(tag string, uid string) []fluentbit.Component {
	c := confgenerator.LoggingProcessorParseMultilineRegex{
		LoggingProcessorParseRegexComplex: confgenerator.LoggingProcessorParseRegexComplex{
			Parsers: []confgenerator.RegexParser{
				{
					// Sample Line: 2022-04-28 15:09:11.851:INFO :oejs.RequestLogWriter:main: Opened /opt/logs/2022_04_28.request.log
					// Sample Line: 2022-04-28 15:09:11.892:INFO :oejs.AbstractConnector:main: Started ServerConnector@1ebd5336{HTTP/1.1, (http/1.1)}{0.0.0.0:8080}
					// Sample Line: 2022-04-28 15:09:11.897:INFO :oejs.Server:main: Started Server@3232a28a{STARTING}[11.0.9,sto=5000] @1657ms
					Regex: `^(?<timestamp>\d{4}-\d{2}-\d{2}\s\d{2}:\d{2}:\d{2}.\d{3}):(?<level>\w*)\s:(?<source>\w*.\w*):(?<module>\w*):\s(?<message>.*)`,
					Parser: confgenerator.ParserShared{
						TimeKey: "timestamp",
						//   2022-04-28 15:09:11.897
						TimeFormat: "%Y-%m-%d %H:%M:%S.%L",
						Types: map[string]string{
							"lineNumber": "integer",
						},
					},
				},
			},
		},
		Rules: []confgenerator.MultilineRule{
			{
				StateName: "start_state",
				NextState: "cont",
				Regex:     `\d{4}-\d{2}-\d{2}\s\d{2}:\d{2}:\d{2}.\d{3}`,
			},
			{
				StateName: "cont",
				NextState: "cont",
				Regex:     `^(?!\d{4}-\d{2}-\d{2}\s\d{2}:\d{2}:\d{2}.\d{3})`,
			},
		},
	}.Components(tag, uid)

	// https://jetty.apache.org/jetty-10.0-doc/logging.html
	c = append(c,
		fluentbit.TranslationComponents(tag, "level", "logging.googleapis.com/severity", false,
			[]struct{ SrcVal, DestVal string }{
				{"ALL", "DEBUG"},
				{"DEBUG", "DEBUG"},
				{"INFO", "INFO"},
				{"WARN", "WARNING"},
				{"ERROR", "ERROR"},
			},
		)...,
	)
	return c
}

type LoggingProcessorJettyAccess struct {
	confgenerator.ConfigComponent `yaml:",inline"`
}

func (p LoggingProcessorJettyAccess) Components(tag string, uid string) []fluentbit.Component {
	return genericAccessLogParser(tag, uid)
}

func (LoggingProcessorJettyAccess) Type() string {
	return "jetty_access"
}

type LoggingReceiverJettyAccess struct {
	LoggingProcessorJettyAccess             `yaml:",inline"`
	confgenerator.LoggingReceiverFilesMixin `yaml:",inline" validate:"structonly"`
}

func (r LoggingReceiverJettyAccess) Components(tag string) []fluentbit.Component {
	if len(r.IncludePaths) == 0 {
		r.IncludePaths = []string{
			"/opt/logs/*.request.log",
		}
	}
	c := r.LoggingReceiverFilesMixin.Components(tag)
	c = append(c, r.LoggingProcessorJettyAccess.Components(tag, "jetty_access")...)
	return c
}

func init() {
	confgenerator.LoggingProcessorTypes.RegisterType(func() confgenerator.Component { return &LoggingProcessorJettySystem{} })
	confgenerator.LoggingProcessorTypes.RegisterType(func() confgenerator.Component { return &LoggingProcessorJettyAccess{} })
	confgenerator.LoggingReceiverTypes.RegisterType(func() confgenerator.Component { return &LoggingReceiverJettyAccess{} })
}

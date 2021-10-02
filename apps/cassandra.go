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
)

type LoggingProcessorCassandraSystem struct {
	confgenerator.ConfigComponent `yaml:",inline"`
}

func (LoggingProcessorCassandraSystem) Type() string {
	return "cassandra_system"
}

func (p LoggingProcessorCassandraSystem) Components(tag string, uid string) []fluentbit.Component {
	c := confgenerator.LoggingProcessorParseMultiline{
		// Sample line: DEBUG [main] 2021-10-01 20:15:36,385 InternalLoggerFactory.java:63 - Using SLF4J as the default logging framework
		Regex: `(?<level>[A-Z]+)\s+\[(?<type>[^\]]+)\]\s+(?<time>\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2},\d+)\s+(?<extendedMessage>(?<message>(?:(?<javaClass>[\w\.]+):(?<lineNumber>\d+))?.+)[\S\s]+)`,
		LoggingProcessorParseShared: confgenerator.LoggingProcessorParseShared{
			TimeKey:    "time",
			TimeFormat: "%Y-%m-%d %H:%M:%S,%L",
			Types: map[string]string{
				"lineNumber": "integer",
			},
		},
		Rules: []confgenerator.MultilineRule{
			confgenerator.MultilineRule{
				StateName: "start_state",
				NextState: "cont",
				Regex:     `[A-Z]+\s+\[[^\]]+\] \d+`,
			},
			confgenerator.MultilineRule{
				StateName: "cont",
				NextState: "cont",
				Regex:     `^(?![A-Z]+\s+\[[^\]]+\] \d+)`,
			},
		},
	}.Components(tag, uid)

	for _, l := range []struct{ level, severity string }{
		{"TRACE", "TRACE"},
		{"DEBUG", "DEBUG"},
		{"INFO", "INFO"},
		{"ERROR", "ERROR"},
		{"WARN", "WARNING"},
	} {
		c = append(c, fluentbit.Component{
			Kind: "FILTER",
			Config: map[string]string{
				"Name":      "modify",
				"Match":     tag,
				"Condition": fmt.Sprintf("Key_Value_Equals level %s", l.level),
				"Add":       fmt.Sprintf("logging.googleapis.com/severity %s", l.severity),
			},
		})
	}

	return c
}

type LoggingReceiverCassandraSystem struct {
	LoggingProcessorCassandraSystem         `yaml:",inline"`
	confgenerator.LoggingReceiverFilesMixin `yaml:",inline" validate:"structonly"`
}

func (r LoggingReceiverCassandraSystem) Components(tag string) []fluentbit.Component {
	if len(r.IncludePaths) == 0 {
		r.IncludePaths = []string{"/var/log/cassandra/system*.log"}
	}
	c := r.LoggingReceiverFilesMixin.Components(tag)
	c = append(c, r.LoggingProcessorCassandraSystem.Components(tag, "cassandra_system")...)
	return c
}

func init() {
	confgenerator.LoggingProcessorTypes.RegisterType(func() confgenerator.Component { return &LoggingProcessorCassandraSystem{} })
	confgenerator.LoggingReceiverTypes.RegisterType(func() confgenerator.Component { return &LoggingReceiverCassandraSystem{} })
}

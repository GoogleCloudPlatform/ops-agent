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

type MetricsReceiverTomcat struct {
	confgenerator.ConfigComponent `yaml:",inline"`

	confgenerator.MetricsReceiverShared `yaml:",inline"`

	confgenerator.MetricsReceiverSharedJVM `yaml:",inline"`

	confgenerator.MetricsReceiverSharedCollectJVM `yaml:",inline"`
}

const defaultTomcatEndpoint = "localhost:8050"

func (r MetricsReceiverTomcat) Type() string {
	return "tomcat"
}

func (r MetricsReceiverTomcat) Pipelines() []otel.Pipeline {
	targetSystem := "tomcat"

	return r.MetricsReceiverSharedJVM.JVMConfig(
		r.TargetSystemString(targetSystem),
		defaultTomcatEndpoint,
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
	confgenerator.MetricsReceiverTypes.RegisterType(func() confgenerator.Component { return &MetricsReceiverTomcat{} })
}

type LoggingProcessorTomcatSystem struct {
	confgenerator.ConfigComponent `yaml:",inline"`
}

func (LoggingProcessorTomcatSystem) Type() string {
	return "tomcat_system"
}

func (p LoggingProcessorTomcatSystem) Components(tag string, uid string) []fluentbit.Component {
	c := confgenerator.LoggingProcessorParseMultilineRegex{
		LoggingProcessorParseRegexComplex: confgenerator.LoggingProcessorParseRegexComplex{
			Parsers: []confgenerator.RegexParser{
				{
					// Sample line: 11-Jan-2022 20:41:58.279 INFO [main] org.apache.catalina.startup.VersionLoggerListener.log Command line argument: -Djava.io.tmpdir=/opt/tomcat/temp
					// Sample line: 11-Jan-2022 20:41:58.283 INFO [main] org.apache.catalina.core.AprLifecycleListener.lifecycleEvent The Apache Tomcat Native library which allows using OpenSSL was not found on the java.library.path: [/usr/java/packages/lib:/usr/lib/x86_64-linux-gnu/jni:/lib/x86_64-linux-gnu:/usr/lib/x86_64-linux-gnu:/usr/lib/jni:/lib:/usr/lib]
					// Sample line: 13-Jan-2022 16:10:27.715 SEVERE [main] org.apache.catalina.core.ContainerBase.removeChild Error destroying child
					// Sample line: org.apache.catalina.LifecycleException: An invalid Lifecycle transition was attempted ([before_destroy]) for component [StandardEngine[Catalina].StandardHost[localhost].StandardContext[/examples]] in state [STARTED]
					// Sample line:         at org.apache.catalina.util.LifecycleBase.invalidTransition(LifecycleBase.java:430)
					// Sample line:         at org.apache.catalina.util.LifecycleBase.destroy(LifecycleBase.java:316)
					Regex: `^(?<time>\d{2}-[A-Z]{1}[a-z]{2}-\d{4}\s\d{2}:\d{2}:\d{2}.\d{3})\s(?<level>[A-Z]+)\s\[(?<module>[^\]]+)\]\s(?<message>(?<source>[\w\.]+)[\S\s]+)`,
					Parser: confgenerator.ParserShared{
						TimeKey: "time",
						//   13-Jan-2022 16:10:27.715
						TimeFormat: "%d-%b-%Y %H:%M:%S.%L",
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
				Regex:     `\d{2}-[A-Z]{1}[a-z]{2}-\d{4}\s\d{2}:\d{2}:\d{2}.\d{3}`,
			},
			{
				StateName: "cont",
				NextState: "cont",
				Regex:     `^(?!\d{2}-[A-Z]{1}[a-z]{2}-\d{4}\s\d{2}:\d{2}:\d{2}.\d{3})`,
			},
		},
	}.Components(tag, uid)

	// https://tomcat.apache.org/tomcat-10.0-doc/logging.html
	c = append(c,
		fluentbit.TranslationComponents(tag, "level", "logging.googleapis.com/severity", false,
			[]struct{ SrcVal, DestVal string }{
				{"FINEST", "DEBUG"},
				{"FINER", "DEBUG"},
				{"FINE", "DEBUG"},
				{"INFO", "INFO"},
				{"WARNING", "WARNING"},
				{"SEVERE", "CRITICAL"},
			},
		)...,
	)

	c = append(c, fluentbit.Component{
		Kind: "FILTER",
		Config: map[string]string{
			"Name":   "modify",
			"Match":  tag,
			"Remove": "level",
		},
	})
	return c
}

type SystemLoggingReceiverTomcat struct {
	LoggingProcessorTomcatSystem            `yaml:",inline"`
	confgenerator.LoggingReceiverFilesMixin `yaml:",inline" validate:"structonly"`
}

func (r SystemLoggingReceiverTomcat) Components(tag string) []fluentbit.Component {
	if len(r.IncludePaths) == 0 {
		r.IncludePaths = []string{
			"/opt/tomcat/logs/catalina.out",
			"/var/log/tomcat*/catalina.out",
		}
	}
	c := r.LoggingReceiverFilesMixin.Components(tag)
	c = append(c, r.LoggingProcessorTomcatSystem.Components(tag, "tomcat_system")...)
	return c
}

type LoggingProcessorTomcatAccess struct {
	confgenerator.ConfigComponent `yaml:",inline"`
}

func (p LoggingProcessorTomcatAccess) Components(tag string, uid string) []fluentbit.Component {
	return genericAccessLogParser(tag, uid)
}

func (LoggingProcessorTomcatAccess) Type() string {
	return "tomcat_access"
}

type AccessSystemLoggingReceiverTomcat struct {
	LoggingProcessorTomcatAccess            `yaml:",inline"`
	confgenerator.LoggingReceiverFilesMixin `yaml:",inline" validate:"structonly"`
}

func (r AccessSystemLoggingReceiverTomcat) Components(tag string) []fluentbit.Component {
	if len(r.IncludePaths) == 0 {
		r.IncludePaths = []string{
			"/opt/tomcat/logs/localhost_access_log.*.txt",
			"/var/log/tomcat*/localhost_access_log.*.txt",
		}
	}
	c := r.LoggingReceiverFilesMixin.Components(tag)
	c = append(c, r.LoggingProcessorTomcatAccess.Components(tag, "tomcat_access")...)
	return c
}

func init() {
	confgenerator.LoggingProcessorTypes.RegisterType(func() confgenerator.Component { return &LoggingProcessorTomcatAccess{} })
	confgenerator.LoggingProcessorTypes.RegisterType(func() confgenerator.Component { return &LoggingProcessorTomcatSystem{} })
	confgenerator.LoggingReceiverTypes.RegisterType(func() confgenerator.Component { return &AccessSystemLoggingReceiverTomcat{} })
	confgenerator.LoggingReceiverTypes.RegisterType(func() confgenerator.Component { return &SystemLoggingReceiverTomcat{} })
}

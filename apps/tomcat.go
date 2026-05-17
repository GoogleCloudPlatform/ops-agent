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

type MetricsReceiverTomcat struct {
	confgenerator.ConfigComponent `yaml:",inline"`

	confgenerator.MetricsReceiverSharedJVM `yaml:",inline"`

	confgenerator.MetricsReceiverSharedCollectJVM `yaml:",inline"`
}

const defaultTomcatEndpoint = "localhost:8050"

func (r MetricsReceiverTomcat) Type() string {
	return "tomcat"
}

func (r MetricsReceiverTomcat) Pipelines(_ context.Context) ([]otel.ReceiverPipeline, error) {
	targetSystem := "tomcat"

	return r.MetricsReceiverSharedJVM.
		WithDefaultEndpoint(defaultTomcatEndpoint).
		ConfigurePipelines(
			r.TargetSystemString(targetSystem),
			[]otel.Component{
				otel.NormalizeSums(),
				otel.MetricsTransform(
					otel.AddPrefix("workload.googleapis.com"),
				),
				otel.TransformationMetrics(
					otel.SetScopeName("agent.googleapis.com/"+r.Type()),
					otel.SetScopeVersion("1.0"),
				),
				otel.MetricsRemoveServiceAttributes(),
			},
		)
}

func init() {
	confgenerator.MetricsReceiverTypes.RegisterType(func() confgenerator.MetricsReceiver { return &MetricsReceiverTomcat{} })
}

type LoggingProcessorMacroTomcatSystem struct {
}

func (LoggingProcessorMacroTomcatSystem) Type() string {
	return "tomcat_system"
}

func (p LoggingProcessorMacroTomcatSystem) Expand(ctx context.Context) []confgenerator.InternalLoggingProcessor {
	return []confgenerator.InternalLoggingProcessor{
		confgenerator.LoggingProcessorParseMultilineRegex{
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
					Regex:     `^\d{2}-[A-Z]{1}[a-z]{2}-\d{4}\s\d{2}:\d{2}:\d{2}.\d{3}`,
				},
				{
					StateName: "cont",
					NextState: "cont",
					Regex:     `^(?!\d{2}-[A-Z]{1}[a-z]{2}-\d{4}\s\d{2}:\d{2}:\d{2}.\d{3})`,
				},
			},
		},
		// https://tomcat.apache.org/tomcat-10.0-doc/logging.html
		confgenerator.LoggingProcessorModifyFields{
			Fields: map[string]*confgenerator.ModifyField{
				"severity": {
					CopyFrom: "jsonPayload.level",
					MapValues: map[string]string{
						"FINEST":  "DEBUG",
						"FINER":   "DEBUG",
						"FINE":    "DEBUG",
						"INFO":    "INFO",
						"WARNING": "WARNING",
						"SEVERE":  "CRITICAL",
					},
					MapValuesExclusive: true,
				},
				InstrumentationSourceLabel: instrumentationSourceValue(p.Type()),
			},
		},
	}
}

func loggingReceiverFilesMixinTomcatSystem() confgenerator.LoggingReceiverFilesMixin {
	return confgenerator.LoggingReceiverFilesMixin{
		IncludePaths: []string{
			"/opt/tomcat/logs/catalina.out",
			"/var/log/tomcat*/catalina.out",
			"/var/log/tomcat*/catalina.*.log",
		},
	}
}

type LoggingProcessorMacroTomcatAccess struct {
}

func (p LoggingProcessorMacroTomcatAccess) Expand(ctx context.Context) []confgenerator.InternalLoggingProcessor {
	return genericAccessLogParser(ctx, p.Type())
}

func (LoggingProcessorMacroTomcatAccess) Type() string {
	return "tomcat_access"
}

func loggingReceiverFilesMixinTomcatAccess() confgenerator.LoggingReceiverFilesMixin {
	return confgenerator.LoggingReceiverFilesMixin{
		IncludePaths: []string{
			"/opt/tomcat/logs/localhost_access_log*.txt",
			"/var/log/tomcat*/localhost_access_log*.txt",
		},
	}
}

func init() {
	confgenerator.RegisterLoggingFilesProcessorMacro[LoggingProcessorMacroTomcatAccess](loggingReceiverFilesMixinTomcatAccess)
	confgenerator.RegisterLoggingFilesProcessorMacro[LoggingProcessorMacroTomcatSystem](loggingReceiverFilesMixinTomcatSystem)
}

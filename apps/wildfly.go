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
	"strings"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
)

type MetricsReceiverWildfly struct {
	confgenerator.ConfigComponent          `yaml:",inline"`
	confgenerator.MetricsReceiverSharedJVM `yaml:",inline"`
}

const defaultWildflyEndpoint = "service:jmx:remote+http://localhost:9990"
const defaultAdditionalJar = "/opt/wildfly/bin/client/jboss-client.jar"

func (r MetricsReceiverWildfly) Type() string {
	return "wildfly"
}

func (r MetricsReceiverWildfly) Pipelines() []otel.Pipeline {
	targetSystem := "wildfly"

	if r.MetricsReceiverSharedJVM.Endpoint != "" && !strings.HasPrefix(r.MetricsReceiverSharedJVM.Endpoint, "service:jmx") {
		r.MetricsReceiverSharedJVM.Endpoint = "service:jmx:remote+http://" + r.MetricsReceiverSharedJVM.Endpoint
	}

	return r.MetricsReceiverSharedJVM.
		WithDefaultEndpoint(defaultWildflyEndpoint).
		WithDefaultAdditionalJars(defaultAdditionalJar).
		ConfigurePipelines(
			targetSystem,
			[]otel.Component{
				otel.NormalizeSums(),
				otel.MetricsTransform(
					otel.AddPrefix("workload.googleapis.com"),
				),
			},
		)
}

func init() {
	confgenerator.MetricsReceiverTypes.RegisterType(func() confgenerator.Component { return &MetricsReceiverWildfly{} })
}

type LoggingProcessorWildflySystem struct {
	confgenerator.ConfigComponent `yaml:",inline"`
}

func (LoggingProcessorWildflySystem) Type() string {
	return "wildfly_system"
}

func (p LoggingProcessorWildflySystem) Components(tag string, uid string) []fluentbit.Component {
	c := confgenerator.LoggingProcessorParseRegex{
		// Logging documentation: https://docs.wildfly.org/26/Admin_Guide.html#Logging
		// Sample line: 2022-01-18 13:44:35,372 INFO  [org.wildfly.security] (ServerService Thread Pool -- 27) ELY00001: WildFly Elytron version 1.18.1.Final
		// Sample line: 2022-02-03 15:38:01,509 DEBUG [org.jboss.as.config] (MSC service thread 1-1) VM Arguments: -D[Standalone] -Xms64m -Xmx512m -XX:MetaspaceSize=96M ...Dlogging.configuration=file:/opt/wildfly/standalone/configuration/logging.properties
		// Sample line: 2022-02-03 15:38:03,548 INFO  [org.jboss.as.server] (Controller Boot Thread) WFLYSRV0039: Creating http management service using socket-binding (management-http)
		// Sample line: 2022-02-11 14:29:56,734 INFO  [org.jboss.as.process.Server:server-one.status] (ProcessController-threads - 3) WFLYPC0018: Starting process 'Server:server-one'
		// Sample line: 2022-02-11 14:29:52,217 INFO  [org.jboss.as.process.Host Controller.status] (main) WFLYPC0018: Starting process 'Host Controller'
		// Sample line: 2022-02-03 15:38:01,506 DEBUG [org.jboss.as.config] (MSC service thread 1-1) Configured system properties:
		//                   [Standalone] =
		//                   awt.toolkit = sun.awt.X11.XToolkit
		//                   file.encoding = UTF-8
		//                   file.separator = /
		Regex: `^(?<time>\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2},\d{3})\s+(?<level>\w+)(?:\s+\[(?<source>.+?)\])?(?:\s+\((?<thread>.+?)\))?\s+(?<message>(?:(?<messageCode>[\d\w]+):)?[\s\S]*)`,
		ParserShared: confgenerator.ParserShared{
			TimeKey:    "time",
			TimeFormat: "%Y-%m-%d %H:%M:%S,%L",
		},
	}.Components(tag, uid)

	c = append(c,
		fluentbit.TranslationComponents(tag, "level", "logging.googleapis.com/severity", false,
			[]struct{ SrcVal, DestVal string }{
				{"TRACE", "TRACE"},
				{"DEBUG", "DEBUG"},
				{"INFO", "INFO"},
				{"ERROR", "ERROR"},
				{"WARN", "WARNING"},
			},
		)...,
	)

	return c
}

type LoggingReceiverWildflySystem struct {
	LoggingProcessorWildflySystem           `yaml:",inline"`
	confgenerator.LoggingReceiverFilesMixin `yaml:",inline" validate:"structonly"`
}

func (r LoggingReceiverWildflySystem) Components(tag string) []fluentbit.Component {
	if len(r.IncludePaths) == 0 {
		r.IncludePaths = []string{
			// no package installers, default installation usually provides the following
			// Standalone server log
			"/opt/wildfly/standalone/log/server.log",
			// Managed Domain server log(s)
			"/opt/wildfly/domain/servers/*/log/server.log",
			// Managed Domain controller log(s)
			"/opt/wildfly/domain/log/*.log",
		}
	}

	r.MultilineRules = []confgenerator.MultilineRule{
		{
			StateName: "start_state",
			NextState: "cont",
			Regex:     `\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2},\d{3}`,
		},
		{
			StateName: "cont",
			NextState: "cont",
			Regex:     `^(?!\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2},\d{3})`,
		},
	}

	c := r.LoggingReceiverFilesMixin.Components(tag)
	c = append(c, r.LoggingProcessorWildflySystem.Components(tag, "wildfly_system")...)
	return c
}

func init() {
	confgenerator.LoggingProcessorTypes.RegisterType(func() confgenerator.Component { return &LoggingProcessorWildflySystem{} })
	confgenerator.LoggingReceiverTypes.RegisterType(func() confgenerator.Component { return &LoggingReceiverWildflySystem{} })
}

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

type LoggingProcessorTomcat struct {
	confgenerator.ConfigComponent `yaml:",inline"`
}

func (LoggingProcessorTomcat) Type() string {
	return "tomcat"
}

func (p LoggingProcessorTomcat) Components(tag string, uid string) []fluentbit.Component {
	c := confgenerator.LoggingProcessorParseRegex{
		// Sample line: 11-Jan-2022 20:41:58.279 INFO [main] org.apache.catalina.startup.VersionLoggerListener.log Command line argument: -Djava.io.tmpdir=/opt/tomcat/temp
		// Sample line: 11-Jan-2022 20:41:58.283 INFO [main] org.apache.catalina.core.AprLifecycleListener.lifecycleEvent The Apache Tomcat Native library which allows using OpenSSL was not found on the java.library.path: [/usr/java/packages/lib:/usr/lib/x86_64-linux-gnu/jni:/lib/x86_64-linux-gnu:/usr/lib/x86_64-linux-gnu:/usr/lib/jni:/lib:/usr/lib]
		// Sample line: 11-Jan-2022 20:41:58.988 INFO [main] org.apache.catalina.core.StandardService.startInternal Starting service [Catalina]
		Regex: `(?<time>\d{2}-[A-Z]{1}[a-z]{2}-\d{4}\s\d{2}:\d{2}:\d{2}.\d{3})\s(?<level>[A-Z]+)\s\[(?<module>[^\]]+)\]\s(?<message>(?:(?<source>[\w\.]+):(?<lineNumber>\d+))?[\S\s]+)`,
		ParserShared: confgenerator.ParserShared{
			TimeKey:    "time",
			TimeFormat: "%d %b %Y %H:%M:%S.%L",
			Types: map[string]string{
				"lineNumber": "integer",
			},
		},
	}.Components(tag, uid)

	// https://tomcat.apache.org/tomcat-10.0-doc/logging.html
	c = append(c,
		fluentbit.TranslationComponents(tag, "level", "logging.googleapis.com/severity",
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
	return c
}

type LoggingReceiverTomcat struct {
	LoggingProcessorTomcat                  `yaml:",inline"`
	confgenerator.LoggingReceiverFilesMixin `yaml:",inline" validate:"structonly"`
}

func (r LoggingReceiverTomcat) Components(tag string) []fluentbit.Component {
	if len(r.IncludePaths) == 0 {
		r.IncludePaths = []string{
			// Default log path on Ubuntu / Debian
			"/opt/tomcat/logs/catalina.out",
		}
	}
	c := r.LoggingReceiverFilesMixin.Components(tag)
	c = append(c, r.LoggingProcessorTomcat.Components(tag, "tomcat")...)
	return c
}

type AccessLoggingReceiverTomcat struct {
	LoggingProcessorAccess                  `yaml:",inline"`
	confgenerator.LoggingReceiverFilesMixin `yaml:",inline" validate:"structonly"`
}

func (r AccessLoggingReceiverTomcat) Components(tag string) []fluentbit.Component {
	if len(r.IncludePaths) == 0 {
		r.IncludePaths = []string{
			// Default log path on Ubuntu / Debian
			"/opt/tomcat/logs/localhost_access_log.*.txt ",
		}
	}
	c := r.LoggingReceiverFilesMixin.Components(tag)
	c = append(c, r.LoggingProcessorAccess.Components(tag, "tomcat_access")...)
	return c
}

func init() {
	confgenerator.LoggingReceiverTypes.RegisterType(func() confgenerator.Component { return &AccessLoggingReceiverTomcat{} })
	confgenerator.LoggingProcessorTypes.RegisterType(func() confgenerator.Component { return &LoggingProcessorTomcat{} })
	confgenerator.LoggingReceiverTypes.RegisterType(func() confgenerator.Component { return &LoggingReceiverTomcat{} })
}

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

	"strings"
)

type MetricsReceiverPostgresql struct {
	confgenerator.ConfigComponent `yaml:",inline"`

	confgenerator.MetricsReceiverShared    `yaml:",inline"`
	confgenerator.MetricsReceiverSharedTLS `yaml:",inline"`

	Endpoint string `yaml:"endpoint" validate:"omitempty,hostname_port|startswith=/"`

	Password  string   `yaml:"password" validate:"omitempty"`
	Username  string   `yaml:"username" validate:"omitempty"`
	Databases []string `yaml:"databases" validate:"omitempty"`
}

// Actual socket is /var/run/postgresql/.s.PGSQL.5432 but the lib/pq go module used by
// the underlying receiver expects it like this
const defaultPostgresqlUnixEndpoint = "var/run/postgresql/:5432"

func (r MetricsReceiverPostgresql) Type() string {
	return "postgresql"
}

func (r MetricsReceiverPostgresql) Pipelines() []otel.Pipeline {
	transport := "tcp"
	if r.Endpoint == "" {
		transport = "unix"
		r.Endpoint = defaultPostgresqlUnixEndpoint
	} else if strings.HasPrefix(r.Endpoint, "/") {
		transport = "unix"
		endpointParts := strings.Split(r.Endpoint, ".")
		r.Endpoint = strings.TrimLeft(endpointParts[0], "/") + ":" + endpointParts[len(endpointParts)-1]
	}

	cfg := map[string]interface{}{
		"collection_interval": r.CollectionIntervalString(),
		"endpoint":            r.Endpoint,
		"username":            r.Username,
		"password":            r.Password,
		"transport":           transport,
	}

	if transport == "tcp" {
		cfg["tls"] = r.TLSConfig(true)
	}

	return []otel.Pipeline{{
		Receiver: otel.Component{
			Type:   "postgresql",
			Config: cfg,
		},
		Processors: []otel.Component{
			otel.NormalizeSums(),
			otel.MetricsTransform(
				otel.AddPrefix("workload.googleapis.com"),
			),
		},
	}}
}

func init() {
	confgenerator.MetricsReceiverTypes.RegisterType(func() confgenerator.Component { return &MetricsReceiverPostgresql{} })
}

type LoggingProcessorPostgresql struct {
	confgenerator.ConfigComponent `yaml:",inline"`
}

func (LoggingProcessorPostgresql) Type() string {
	return "postgresql_general"
}

func (p LoggingProcessorPostgresql) Components(tag string, uid string) []fluentbit.Component {
	c := confgenerator.LoggingProcessorParseMultilineRegex{
		LoggingProcessorParseRegexComplex: confgenerator.LoggingProcessorParseRegexComplex{
			Parsers: []confgenerator.RegexParser{
				{
					// Limited logging documentation: https://www.postgresql.org/docs/10/runtime-config-logging.html
					// Sample line: 2022-01-12 20:57:58.378 UTC [26241] LOG:  starting PostgreSQL 14.1 (Debian 14.1-1.pgdg100+1) on x86_64-pc-linux-gnu, compiled by gcc (Debian 8.3.0-6) 8.3.0, 64-bit
					// Sample line: 2022-01-12 20:59:25.169 UTC [27445] postgres@postgres FATAL:  Peer authentication failed for user "postgres"
					// Sample line: 2022-01-12 21:49:13.989 UTC [27836] postgres@postgres LOG:  duration: 1.074 ms  statement: select *
					//    from pg_database
					//    where 1=1;
					Regex: `^(?<time>\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}.\d{3,} \w+)\s*\[(?<tid>\d+)\](?:\s+(?<role>\S*)@(?<user>\S*))?\s*(?<level>\w+):\s+(?<message>[\s\S]*)`,
					Parser: confgenerator.ParserShared{
						TimeKey:    "time",
						TimeFormat: "%Y-%m-%d %H:%M:%S.%L %z",
						Types: map[string]string{
							"tid": "integer",
						},
					},
				},
			},
		},
		Rules: []confgenerator.MultilineRule{
			{
				StateName: "start_state",
				NextState: "cont",
				Regex:     `\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}.\d{3,} \w+`,
			},
			{
				StateName: "cont",
				NextState: "cont",
				Regex:     `^(?!\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}.\d{3,} \w+)`,
			},
		},
	}.Components(tag, uid)

	// https://www.postgresql.org/docs/10/runtime-config-logging.html#RUNTIME-CONFIG-SEVERITY-LEVELS
	c = append(c,
		fluentbit.TranslationComponents(tag, "level", "logging.googleapis.com/severity", false,
			[]struct{ SrcVal, DestVal string }{
				{"DEBUG1", "DEBUG"},
				{"DEBUG2", "DEBUG"},
				{"DEBUG3", "DEBUG"},
				{"DEBUG4", "DEBUG"},
				{"DEBUG5", "DEBUG"},
				{"DETAIL", "DEBUG"},
				{"STATEMENT", "DEBUG"},
				{"INFO", "INFO"},
				{"LOG", "INFO"},
				{"NOTICE", "INFO"},
				{"ERROR", "ERROR"},
				{"WARNING", "WARNING"},
				{"FATAL", "CRITICAL"},
				{"PANIC", "CRITICAL"},
			},
		)...,
	)

	return c
}

type LoggingReceiverPostgresql struct {
	LoggingProcessorPostgresql              `yaml:",inline"`
	confgenerator.LoggingReceiverFilesMixin `yaml:",inline" validate:"structonly"`
}

func (r LoggingReceiverPostgresql) Components(tag string) []fluentbit.Component {
	if len(r.IncludePaths) == 0 {
		r.IncludePaths = []string{
			// Default log paths for Debain / Ubuntu
			"/var/log/postgresql/postgresql*.log",
			// Default log paths for SLES
			"/var/lib/pgsql/data/log/postgresql*.log",
			// Default log paths for CentOS / RHEL
			"/var/lib/pgsql/*/data/log/postgresql*.log",
		}
	}
	c := r.LoggingReceiverFilesMixin.Components(tag)
	c = append(c, r.LoggingProcessorPostgresql.Components(tag, "postgresql")...)
	return c
}

func init() {
	confgenerator.LoggingProcessorTypes.RegisterType(func() confgenerator.Component { return &LoggingProcessorPostgresql{} })
	confgenerator.LoggingReceiverTypes.RegisterType(func() confgenerator.Component { return &LoggingReceiverPostgresql{} })
}

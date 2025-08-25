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
	"strings"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
	"github.com/GoogleCloudPlatform/ops-agent/internal/secret"
)

type MetricsReceiverMongoDB struct {
	confgenerator.ConfigComponent          `yaml:",inline"`
	confgenerator.MetricsReceiverSharedTLS `yaml:",inline"`
	confgenerator.MetricsReceiverShared    `yaml:",inline"`
	Endpoint                               string        `yaml:"endpoint,omitempty"`
	Username                               string        `yaml:"username,omitempty"`
	Password                               secret.String `yaml:"password,omitempty"`
}

type MetricsReceiverMongoDBHosts struct {
	Endpoint  string `yaml:"endpoint"`
	Transport string `yaml:"transport"`
}

const defaultMongodbEndpoint = "localhost:27017"

func (r MetricsReceiverMongoDB) Type() string {
	return "mongodb"
}

func (r MetricsReceiverMongoDB) Pipelines(_ context.Context) ([]otel.ReceiverPipeline, error) {
	transport := "tcp"
	if r.Endpoint == "" {
		r.Endpoint = defaultMongodbEndpoint
	} else if strings.HasSuffix(r.Endpoint, ".sock") {
		transport = "unix"
	}

	hosts := []MetricsReceiverMongoDBHosts{
		{
			r.Endpoint,
			transport,
		},
	}

	config := map[string]interface{}{
		"hosts":               hosts,
		"username":            r.Username,
		"password":            r.Password.SecretValue(),
		"collection_interval": r.CollectionIntervalString(),
	}

	if transport != "unix" {
		config["tls"] = r.TLSConfig(false)
	}

	return []otel.ReceiverPipeline{{
		Receiver: otel.Component{
			Type:   r.Type(),
			Config: config,
		},
		Processors: map[string][]otel.Component{"metrics": {
			otel.NormalizeSums(),
			otel.MetricsTransform(
				otel.AddPrefix("workload.googleapis.com"),
			),
			otel.TransformationMetrics(
				otel.SetScopeName("agent.googleapis.com/"+r.Type()),
				otel.SetScopeVersion("1.0"),
			),
		}},
	}}, nil
}

func init() {
	confgenerator.MetricsReceiverTypes.RegisterType(func() confgenerator.MetricsReceiver { return &MetricsReceiverMongoDB{} })
}

type LoggingProcessorMacroMongodb struct {
}

func (LoggingProcessorMacroMongodb) Type() string {
	return "mongodb"
}

func (p LoggingProcessorMacroMongodb) Expand(ctx context.Context) []confgenerator.InternalLoggingProcessor {
	c := []confgenerator.InternalLoggingProcessor{}

	c = append(c, p.JsonLogComponents(ctx)...)
	c = append(c, p.RegexLogComponents()...)
	c = append(c, p.severityParser()...)

	return c
}

// JsonLogComponents are the fluentbit components for parsing log messages that are json formatted.
// these are generally messages from mongo with versions greater than or equal to 4.4
// documentation: https://docs.mongodb.com/v4.4/reference/log-messages/#log-message-format
func (p LoggingProcessorMacroMongodb) JsonLogComponents(ctx context.Context) []confgenerator.InternalLoggingProcessor {
	c := p.jsonParserWithTimeKey()

	c = append(c, p.promoteWiredTiger()...)
	c = append(c, p.renames()...)

	return c
}

// jsonParserWithTimeKey requires promotion of the nested timekey for the json parser so we must
// first promote the $date field from the "t" field before declaring the parser
func (p LoggingProcessorMacroMongodb) jsonParserWithTimeKey() []confgenerator.InternalLoggingProcessor {
	c := []confgenerator.InternalLoggingProcessor{}

	c = append(c, &confgenerator.LoggingProcessorParseJson{
		ParserShared: confgenerator.ParserShared{
			Types: map[string]string{
				"id":      "integer",
				"message": "string",
			},
		},
	})

	// have to bring $date to top level in order for it to be parsed as timeKey
	// see https://github.com/fluent/fluent-bit/issues/1013
	c = append(c, &confgenerator.LoggingProcessorModifyFields{
		Fields: map[string]*confgenerator.ModifyField{
			"jsonPayload.time": {
				MoveFrom: "jsonPayload.t.$date",
			},
		},
	})

	c = append(c, &confgenerator.LoggingProcessorRemoveField{
		Field: "t",
	})

	// IMPORTANT: now that we have lifted the json to top level
	// we need to re-parse in order to properly set time at the
	// parser level
	c = append(c, &confgenerator.LoggingProcessorParseTimestamp{
		ParserShared: confgenerator.ParserShared{
			TimeKey:    "time",
			TimeFormat: "%Y-%m-%dT%H:%M:%S.%L%z",
		},
	})

	return c
}

// severityParser is used by both regex and json parser to ensure an "s" field on the entry gets translated
// to a valid logging.googleapis.com/seveirty field
func (p LoggingProcessorMacroMongodb) severityParser() []confgenerator.InternalLoggingProcessor {
	return []confgenerator.InternalLoggingProcessor{
		&confgenerator.LoggingProcessorModifyFields{
			Fields: map[string]*confgenerator.ModifyField{
				"jsonPayload.severity": {
					MoveFrom: "jsonPayload.s",
				},
				"severity": {
					CopyFrom: "jsonPayload.s",
					MapValues: map[string]string{
						"D":  "DEBUG",
						"D1": "DEBUG",
						"D2": "DEBUG",
						"D3": "DEBUG",
						"D4": "DEBUG",
						"D5": "DEBUG",
						"I":  "INFO",
						"E":  "ERROR",
						"F":  "FATAL",
						"W":  "WARNING",
					},
					MapValuesExclusive: true,
				},
				InstrumentationSourceLabel: instrumentationSourceValue(p.Type()),
			},
		},
	}
}

func (p LoggingProcessorMacroMongodb) renames() []confgenerator.InternalLoggingProcessor {
	r := []confgenerator.InternalLoggingProcessor{}
	renames := []struct {
		src  string
		dest string
	}{
		{"jsonPayload.c", "jsonPayload.component"},
		{"jsonPayload.ctx", "jsonPayload.context"},
		{"jsonPayload.msg", "jsonPayload.message"},
		{"jsonPayload.attr", "jsonPayload.attributes"},
	}

	for _, rename := range renames {
		r = append(r, &confgenerator.LoggingProcessorModifyFields{
			Fields: map[string]*confgenerator.ModifyField{
				rename.dest: {
					MoveFrom: rename.src,
				},
			},
		})
	}

	return r
}

func (p LoggingProcessorMacroMongodb) promoteWiredTiger() []confgenerator.InternalLoggingProcessor {
	// promote messages that are WiredTiger messages and are nested in attr.message
	c := []confgenerator.InternalLoggingProcessor{}

	addPrefix := "temp_attributes_"

	c = append(c, &confgenerator.LoggingProcessorModifyFields{
		Fields: map[string]*confgenerator.ModifyField{
			"jsonPayload.temp_attributes_message": {
				MoveFrom: "jsonPayload.attr.message",
			},
		},
	})

	c = append(c, &confgenerator.LoggingProcessorHardRename{
		Field:   addPrefix + "message",
		NewName: "msg",
	})

	return c
}

func (p LoggingProcessorMacroMongodb) RegexLogComponents() []confgenerator.InternalLoggingProcessor {
	c := []confgenerator.InternalLoggingProcessor{}

	c = append(c, &confgenerator.LoggingProcessorParseRegex{
		ParserShared: confgenerator.ParserShared{
			TimeKey:    "timestamp",
			TimeFormat: "%Y-%m-%dT%H:%M:%S.%L%z",
			Types: map[string]string{
				"message":   "string",
				"id":        "integer",
				"s":         "string",
				"component": "string",
				"context":   "string",
			},
		},
		Regex: `^(?<timestamp>[^ ]*)\s+(?<s>\w)\s+(?<component>[^ ]+)\s+\[(?<context>[^\]]+)]\s+(?<message>.*?) *(?<ms>(\d+))?(:?ms)?$`,
		Field: "message",
	})

	return c
}

func loggingReceiverFilesMixinMongodb() confgenerator.LoggingReceiverFilesMixin {
	return confgenerator.LoggingReceiverFilesMixin{
		IncludePaths: []string{"/var/log/mongodb/mongod.log*"},
	}
}

func init() {
	confgenerator.RegisterLoggingFilesProcessorMacro[LoggingProcessorMacroMongodb](loggingReceiverFilesMixinMongodb)
}

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
	"fmt"
	"strings"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit/modify"
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
	return []confgenerator.InternalLoggingProcessor{MongodbProcessors{}}
}

type MongodbProcessors struct{}

func (MongodbProcessors) Type() string {
	return "mongodb"
}

func (p MongodbProcessors) Components(ctx context.Context, tag, uid string) []fluentbit.Component {
	c := []fluentbit.Component{}

	c = append(c, p.FluentBitJsonLogComponents(ctx, tag, uid)...)
	c = append(c, p.FluentBitRegexLogComponents(tag, uid)...)
	c = append(c, p.severityParser().Components(ctx, tag, uid)...)

	return c
}

func (p MongodbProcessors) Processors(ctx context.Context) ([]otel.Component, error) {
	processors := []confgenerator.InternalLoggingProcessor{}
	processors = append(processors, p.JsonLogComponents()...)
	processors = append(processors, p.RegexLogComponents())
	processors = append(processors, p.severityParser())
	processors = append(processors, p.PostProcessing())

	c := []otel.Component{}
	for _, p := range processors {
		op, ok := p.(confgenerator.InternalOTelProcessor)
		if !ok {
			continue
		}
		components, err := op.Processors(ctx)
		if err != nil {
			return nil, err
		}
		c = append(c, components...)
	}

	return c, nil
}

// FluentBitJsonLogComponents are the fluentbit components for parsing log messages that are json formatted.
// these are generally messages from mongo with versions greater than or equal to 4.4
// documentation: https://docs.mongodb.com/v4.4/reference/log-messages/#log-message-format
func (p MongodbProcessors) FluentBitJsonLogComponents(ctx context.Context, tag, uid string) []fluentbit.Component {
	c := p.FluentbitJsonParserWithTimeKey(ctx, tag, uid)

	c = append(c, p.FluentbitPromoteWiredTiger(tag, uid)...)
	c = append(c, p.FluentbitRenames(tag, uid)...)

	return c
}

// JsonLogComponents are the components for parsing log messages that are json formatted.
// these are generally messages from mongo with versions greater than or equal to 4.4
// documentation: https://docs.mongodb.com/v4.4/reference/log-messages/#log-message-format
func (p MongodbProcessors) JsonLogComponents() []confgenerator.InternalLoggingProcessor {
	c := p.jsonParserWithTimeKey()

	c = append(c, p.promoteWiredTiger()...)
	c = append(c, p.renames()...)

	return c
}

// FluentbitJsonParserWithTimeKey requires promotion of the nested timekey for the json parser so we must
// first promote the $date field from the "t" field before declaring the parser
func (p MongodbProcessors) FluentbitJsonParserWithTimeKey(ctx context.Context, tag, uid string) []fluentbit.Component {
	c := []fluentbit.Component{}

	jsonParser := &confgenerator.LoggingProcessorParseJson{
		ParserShared: confgenerator.ParserShared{
			TimeKey:    "time",
			TimeFormat: "%Y-%m-%dT%H:%M:%S.%L%z",
			Types: map[string]string{
				"id":      "integer",
				"message": "string",
			},
		},
	}
	jpComponents := jsonParser.Components(ctx, tag, uid)

	// The parserFilterComponent is the actual filter component that configures and defines
	// which parser to use. We need the component to determine which parser to use when
	// re-parsing below. Each time a parser filter is used, there are 2 filter components right
	// before it to account for the nest lua script (see confgenerator/fluentbit/parse_deduplication.go).
	// Therefore, the parse filter component is actually the third component in the list.
	parserFilterComponent := jpComponents[2]
	c = append(c, jpComponents...)

	tempPrefix := "temp_ts_"
	timeKey := "time"
	// have to bring $date to top level in order for it to be parsed as timeKey
	// see https://github.com/fluent/fluent-bit/issues/1013
	liftTs := fluentbit.Component{
		Kind: "FILTER",
		Config: map[string]string{
			"Name":         "nest",
			"Match":        tag,
			"Operation":    "lift",
			"Nested_under": "t",
			"Add_prefix":   tempPrefix,
		},
	}

	renameTsOption := modify.NewHardRenameOptions(fmt.Sprintf("%s$date", tempPrefix), timeKey)
	renameTs := renameTsOption.Component(tag)

	c = append(c, liftTs, renameTs)

	// IMPORTANT: now that we have lifted the json to top level
	// we need to re-parse in order to properly set time at the
	// parser level
	nestFilters := fluentbit.LuaFilterComponents(tag, fluentbit.ParserNestLuaFunction, fmt.Sprintf(fluentbit.ParserNestLuaScriptContents, "message"))
	parserFilter := fluentbit.Component{
		Kind: "FILTER",
		Config: map[string]string{
			"Name":         "parser",
			"Match":        tag,
			"Key_Name":     "message",
			"Reserve_Data": "True",
			"Parser":       parserFilterComponent.OrderedConfig[0][1],
		},
	}
	mergeFilters := fluentbit.LuaFilterComponents(tag, fluentbit.ParserMergeLuaFunction, fluentbit.ParserMergeLuaScriptContents)
	c = append(c, nestFilters...)
	c = append(c, parserFilter)
	c = append(c, mergeFilters...)

	removeTimestamp := fluentbit.Component{
		Kind: "FILTER",
		Config: map[string]string{
			"Name":   "modify",
			"Match":  tag,
			"Remove": timeKey,
		},
	}
	c = append(c, removeTimestamp)

	return c
}

// jsonParserWithTimeKey requires promotion of the nested timekey for the json parser so we must
// first promote the $date field from the "t" field before declaring the parser
func (p MongodbProcessors) jsonParserWithTimeKey() []confgenerator.InternalLoggingProcessor {
	c := []confgenerator.InternalLoggingProcessor{}

	c = append(c, &confgenerator.LoggingProcessorParseJson{
		ParserShared: confgenerator.ParserShared{
			Types: map[string]string{
				"id":      "integer",
				"message": "string",
			},
		},
	})

	// bring $date to top level in order for it to parse as timeKey
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
	c = append(c, &confgenerator.LoggingProcessorParseRegex{
		ParserShared: confgenerator.ParserShared{
			TimeKey:    "time",
			TimeFormat: "%Y-%m-%dT%H:%M:%S.%L%z",
		},
		Regex: `^(?<time>.*)$`,
		Field: "time",
	})

	return c
}

// severityParser is used by both regex and json parser to ensure an "s" field on the entry gets translated
// to a valid logging.googleapis.com/seveirty field
func (p MongodbProcessors) severityParser() confgenerator.InternalLoggingProcessor {
	return confgenerator.LoggingProcessorModifyFields{
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
	}
}

func (p MongodbProcessors) FluentbitRenames(tag, uid string) []fluentbit.Component {
	r := []fluentbit.Component{}
	renames := []struct {
		src  string
		dest string
	}{
		{"c", "component"},
		{"ctx", "context"},
		{"msg", "message"},
	}

	for _, rename := range renames {
		rename := modify.NewRenameOptions(rename.src, rename.dest)
		r = append(r, rename.Component(tag))
	}

	return r
}

func (p MongodbProcessors) renames() []confgenerator.InternalLoggingProcessor {
	r := []confgenerator.InternalLoggingProcessor{}
	renames := []struct {
		src  string
		dest string
	}{
		{"jsonPayload.c", "jsonPayload.component"},
		{"jsonPayload.ctx", "jsonPayload.context"},
		{"jsonPayload.msg", "jsonPayload.json_message"},
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

func (p MongodbProcessors) FluentbitPromoteWiredTiger(tag, uid string) []fluentbit.Component {
	// promote messages that are WiredTiger messages and are nested in attr.message
	addPrefix := "temp_attributes_"
	upNest := fluentbit.Component{
		Kind: "FILTER",
		Config: map[string]string{
			"Name":         "nest",
			"Match":        tag,
			"Operation":    "lift",
			"Nested_under": "attr",
			"Add_prefix":   addPrefix,
		},
	}

	hardRenameMessage := modify.NewHardRenameOptions(fmt.Sprintf("%smessage", addPrefix), "msg")
	wiredTigerRename := hardRenameMessage.Component(tag)

	renameRemainingAttributes := fluentbit.Component{
		Kind: "FILTER",
		Config: map[string]string{
			"Name":          "nest",
			"Wildcard":      fmt.Sprintf("%s*", addPrefix),
			"Match":         tag,
			"Operation":     "nest",
			"Nest_under":    "attributes",
			"Remove_prefix": addPrefix,
		},
	}

	return []fluentbit.Component{upNest, wiredTigerRename, renameRemainingAttributes}
}

func (p MongodbProcessors) promoteWiredTiger() []confgenerator.InternalLoggingProcessor {
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

	c = append(c, &confgenerator.LoggingProcessorRenameIfExists{
		Field:   addPrefix + "message",
		NewName: "msg",
	})

	return c
}

func (p MongodbProcessors) FluentBitRegexLogComponents(tag, uid string) []fluentbit.Component {
	c := []fluentbit.Component{}
	parseKey := "message"
	parser, parserName := fluentbit.ParserComponentBase("%Y-%m-%dT%H:%M:%S.%L%z", "timestamp", map[string]string{
		"message":   "string",
		"id":        "integer",
		"s":         "string",
		"component": "string",
		"context":   "string",
	}, fmt.Sprintf("%s_regex", tag), uid)
	parser.Config["Format"] = "regex"
	parser.Config["Regex"] = `^(?<timestamp>[^ ]*)\s+(?<s>\w)\s+(?<component>[^ ]+)\s+\[(?<context>[^\]]+)]\s+(?<message>.*?) *(?<ms>(\d+))?(:?ms)?$`
	parser.Config["Key_Name"] = parseKey

	nestFilters := fluentbit.LuaFilterComponents(tag, fluentbit.ParserNestLuaFunction, fmt.Sprintf(fluentbit.ParserNestLuaScriptContents, parseKey))
	parserFilter := fluentbit.Component{
		Kind: "FILTER",
		Config: map[string]string{
			"Match":        tag,
			"Name":         "parser",
			"Parser":       parserName,
			"Reserve_Data": "True",
			"Key_Name":     parseKey,
		},
	}
	mergeFilters := fluentbit.LuaFilterComponents(tag, fluentbit.ParserMergeLuaFunction, fluentbit.ParserMergeLuaScriptContents)

	c = append(c, parser)
	c = append(c, nestFilters...)
	c = append(c, parserFilter)
	c = append(c, mergeFilters...)

	return c
}

func (p MongodbProcessors) RegexLogComponents() confgenerator.InternalLoggingProcessor {
	return confgenerator.LoggingProcessorParseRegex{
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
	}
}

func (p MongodbProcessors) PostProcessing() confgenerator.InternalLoggingProcessor {
	return confgenerator.LoggingProcessorModifyFields{
		Fields: map[string]*confgenerator.ModifyField{
			`jsonPayload.message`: {
				MoveFrom: `jsonPayload.json_message`,
			},
			`jsonPayload.attributes`: {
				OmitIf: `jsonPayload.attributes = {}`,
			},
		},
	}
}

func loggingReceiverFilesMixinMongodb() confgenerator.LoggingReceiverFilesMixin {
	return confgenerator.LoggingReceiverFilesMixin{
		IncludePaths: []string{"/var/log/mongodb/mongod.log*"},
	}
}

func init() {
	confgenerator.RegisterLoggingFilesProcessorMacro[LoggingProcessorMacroMongodb](loggingReceiverFilesMixinMongodb)
}

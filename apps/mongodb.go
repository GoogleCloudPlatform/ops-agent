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
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit/modify"
)

type LoggingProcessorMongodb struct {
	confgenerator.ConfigComponent `yaml:",inline"`
}

func (*LoggingProcessorMongodb) Type() string {
	return "mongodb"
}

func (p *LoggingProcessorMongodb) Components(tag, uid string) []fluentbit.Component {
	c := []fluentbit.Component{}

	c = append(c, p.JsonLogComponents(tag, uid)...)
	c = append(c, p.RegexLogComponents(tag, uid)...)
	c = append(c, p.severityParser(tag, uid)...)

	return c
}

// JsonLogComponents are the fluentbit components for parsing log messages that are json formatted.
// these are generally messages from mongo with versions greater than or equal to 4.4
// documentation: https://docs.mongodb.com/v4.4/reference/log-messages/#log-message-format
func (p *LoggingProcessorMongodb) JsonLogComponents(tag, uid string) []fluentbit.Component {
	c := p.jsonParserWithTimeKey(tag, uid)

	c = append(c, p.promoteWiredTiger(tag, uid)...)
	c = append(c, p.renames(tag, uid)...)

	return c
}

// jsonParserWithTimeKey requires promotion of the nested timekey for the json parser so we must
// first promote the $date field from the "t" field before declaring the parser
func (p *LoggingProcessorMongodb) jsonParserWithTimeKey(tag, uid string) []fluentbit.Component {
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
	jpComponents := jsonParser.Components(tag, uid)

	parserComponent, filterComponent := jpComponents[0], jpComponents[1]

	c = append(c, parserComponent, filterComponent)

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
	c = append(c, fluentbit.Component{
		Kind: "FILTER",
		Config: map[string]string{
			"Name":     "parser",
			"Match":    tag,
			"Key_Name": "message",
			"Parser":   parserComponent.OrderedConfig[0][1],
		},
	})

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

// severityParser is used by both regex and json parser to ensure an "s" field on the entry gets translated
// to a valid logging.googleapis.com/seveirty field
func (p *LoggingProcessorMongodb) severityParser(tag, uid string) []fluentbit.Component {
	severityComponents := []fluentbit.Component{}
	severityKey := "logging.googleapis.com/severity"

	severityComponents = append(severityComponents, fluentbit.Component{
		Kind: "FILTER",
		Config: map[string]string{
			"Match":  tag,
			"Name":   "modify",
			"Rename": "s severity",
		},
	})

	severityComponents = append(severityComponents, fluentbit.TranslationComponents(tag, "severity", severityKey, true, []struct {
		SrcVal  string
		DestVal string
	}{
		{"D", "DEBUG"},
		{"D1", "DEBUG"},
		{"D2", "DEBUG"},
		{"D3", "DEBUG"},
		{"D4", "DEBUG"},
		{"D5", "DEBUG"},
		{"I", "INFO"},
		{"E", "ERROR"},
		{"F", "FATAL"},
		{"W", "WARNING"},
	})...)

	return severityComponents
}

func (p *LoggingProcessorMongodb) renames(tag, uid string) []fluentbit.Component {
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

func (p *LoggingProcessorMongodb) promoteWiredTiger(tag, uid string) []fluentbit.Component {
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

func (p *LoggingProcessorMongodb) RegexLogComponents(tag, uid string) []fluentbit.Component {
	c := []fluentbit.Component{}
	parser, parserName := fluentbit.ParserComponentBase("%Y-%m-%dT%H:%M:%S.%L%z", "timestamp", map[string]string{
		"message":   "string",
		"id":        "integer",
		"severity":  "string",
		"component": "string",
		"context":   "string",
	}, fmt.Sprintf("%s_regex", tag), uid)
	parser.Config["Format"] = "regex"
	parser.Config["Regex"] = `^(?<timestamp>[^ ]*)\s+(?<s>\w)\s+(?<component>[^ ]+)\s+\[(?<context>[^\]]+)]\s+(?<message>.*?) *(?<ms>(\d+))?(:?ms)?$`
	parser.Config["Key_Name"] = "message"

	parserFilter := fluentbit.Component{
		Kind: "FILTER",
		Config: map[string]string{
			"Match":    tag,
			"Name":     "parser",
			"Parser":   parserName,
			"Key_Name": "message",
		},
	}
	c = append(c, parser, parserFilter)

	return c
}

type LoggingReceiverMongodb struct {
	LoggingProcessorMongodb                 `yaml:",inline"`
	confgenerator.LoggingReceiverFilesMixin `yaml:",inline" validate:"structonly"`
}

func (r *LoggingReceiverMongodb) Components(tag string) []fluentbit.Component {
	if len(r.IncludePaths) == 0 {
		r.IncludePaths = []string{
			// default logging location
			"/var/log/mongodb/mongod.log*",
		}
	}
	c := r.LoggingReceiverFilesMixin.Components(tag)
	c = append(c, r.LoggingProcessorMongodb.Components(tag, "mongodb")...)
	return c
}

func init() {
	confgenerator.LoggingReceiverTypes.RegisterType(func() confgenerator.Component { return &LoggingReceiverMongodb{} })
}

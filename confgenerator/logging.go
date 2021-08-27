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

package confgenerator

import (
	"fmt"
	"sort"
	"strings"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
)

// setLogNameComponents generates a series of components that rewrites the tag on log entries tagged `tag` to be `logName`.
func setLogNameComponents(tag, logName string) []fluentbit.Component {
	// TODO: Can we just set log_name_key in the output plugin and avoid this mess?
	return []fluentbit.Component{
		{
			Kind: "FILTER",
			Config: map[string]string{
				"Match": tag,
				"Add":   fmt.Sprintf("logName %s", logName),
				"Name":  "modify",
			},
		},
		{
			Kind: "FILTER",
			Config: map[string]string{
				"Emitter_Mem_Buf_Limit": "10M",
				"Emitter_Storage.type":  "filesystem",
				"Match":                 tag,
				"Name":                  "rewrite_tag",
				"Rule":                  "$logName .* $logName false",
			},
		},
		{
			Kind: "FILTER",
			Config: map[string]string{
				"Match":  logName,
				"Name":   "modify",
				"Remove": "logName",
			},
		},
	}
}

// stackdriverOutputComponent generates a component that outputs logs matching the regex `match` using `userAgent`.
func stackdriverOutputComponent(match, userAgent string) fluentbit.Component {
	return fluentbit.Component{
		Kind: "OUTPUT",
		Config: map[string]string{
			// https://docs.fluentbit.io/manual/pipeline/outputs/stackdriver
			"Name":              "stackdriver",
			"Match_Regex":       fmt.Sprintf("^(%s)$", match),
			"resource":          "gce_instance",
			"stackdriver_agent": userAgent,

			// https://docs.fluentbit.io/manual/administration/scheduling-and-retries
			// After 3 retries, a given chunk will be discarded. So bad entries don't accidentally stay around forever.
			"Retry_Limit": "3",

			// https://docs.fluentbit.io/manual/administration/security
			// Enable TLS support.
			"tls": "On",
			// Do not force certificate validation.
			"tls.verify": "Off",

			"workers": "8",
		},
	}
}

// LoggingProcessorParseShared holds common parameters that are used by all processors that are implemented with fluentbit's "parser" filter.
type LoggingProcessorParseShared struct {
	Field      string `yaml:"field,omitempty"`       // default to "message"
	TimeKey    string `yaml:"time_key,omitempty"`    // by default does not parse timestamp
	TimeFormat string `yaml:"time_format,omitempty"` // must be provided if time_key is present
	// Types allows parsing the extracted fields.
	// Not exposed to users for now, but can be used by app receivers.
	// Documented at https://docs.fluentbit.io/manual/v/1.3/parser
	Types map[string]string `yaml:"-" validate:"dive,oneof=string integer bool float hex"`
}

// Components returns a filter and parser component for this parse processor.
// The parser component is incomplete and needs (at a minimum) the "Format" key to be set.
func (p LoggingProcessorParseShared) Components(tag, uid string) (fluentbit.Component, fluentbit.Component) {
	parserName := fmt.Sprintf("%s.%s", tag, uid)
	filter := fluentbit.Component{
		Kind: "FILTER",
		Config: map[string]string{
			"Match":    tag,
			"Name":     "parser",
			"Parser":   parserName,
			"Key_Name": "message", // Required
		},
	}
	if p.Field != "" {
		filter.Config["Key_Name"] = p.Field
	}
	parser := fluentbit.Component{
		Kind: "PARSER",
		Config: map[string]string{
			"Name": parserName,
		},
	}
	if p.TimeFormat != "" {
		parser.Config["Time_Format"] = p.TimeFormat
	}
	if p.TimeKey != "" {
		parser.Config["Time_Key"] = p.TimeKey
	}
	if len(p.Types) > 0 {
		var types []string
		for k, v := range p.Types {
			types = append(types, fmt.Sprintf("%s:%s", k, v))
		}
		sort.Strings(types)
		parser.Config["Types"] = strings.Join(types, " ")
	}
	return filter, parser
}

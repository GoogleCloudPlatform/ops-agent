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
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
)

type LoggingProcessorParseJson struct {
	ConfigComponent             `yaml:",inline"`
	LoggingProcessorParseShared `yaml:",inline"`
}

func (p LoggingProcessorParseJson) Type() string {
	return "parse_json"
}

func (p LoggingProcessorParseJson) Components(tag string, id string) []fluentbit.Component {
	parser := fmt.Sprintf("%s.%s", tag, id)
	keyName := "message"
	if p.Field != "" {
		keyName = p.Field
	}
	return []fluentbit.Component{
		fluentbit.FilterParser{
			Match:   tag,
			KeyName: keyName,
			Parser:  parser,
		}.Component(),
		fluentbit.ParserJSON{
			Name:       parser,
			TimeKey:    p.TimeKey,
			TimeFormat: p.TimeFormat,
		}.Component(),
	}
}

func init() {
	loggingProcessorTypes.registerType(func() component { return &LoggingProcessorParseJson{} })
}

type LoggingProcessorParseRegex struct {
	ConfigComponent             `yaml:",inline"`
	LoggingProcessorParseShared `yaml:",inline"`

	Regex string `yaml:"regex,omitempty" validate:"required"`
}

func (p LoggingProcessorParseRegex) Type() string {
	return "parse_regex"
}

func (p LoggingProcessorParseRegex) Components(tag string, id string) []fluentbit.Component {
	parser := fmt.Sprintf("%s.%s", tag, id)
	keyName := "message"
	if p.Field != "" {
		keyName = p.Field
	}
	return []fluentbit.Component{
		fluentbit.FilterParser{
			Match:   tag,
			KeyName: keyName,
			Parser:  parser,
		}.Component(),
		fluentbit.ParserRegex{
			Name:       parser,
			Regex:      p.Regex,
			TimeKey:    p.TimeKey,
			TimeFormat: p.TimeFormat,
		}.Component(),
	}
}

func init() {
	loggingProcessorTypes.registerType(func() component { return &LoggingProcessorParseRegex{} })
}

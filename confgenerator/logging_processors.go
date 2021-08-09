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

type LoggingProcessorParseJson struct {
	configComponent             `yaml:",inline"`
	loggingProcessorParseShared `yaml:",inline"`
}

func (r LoggingProcessorParseJson) Type() string {
	return "parse_json"
}

func (p *LoggingProcessorParseJson) ValidateParameters(subagent string, kind string, id string) error {
	return validateParameters(*p, subagent, kind, id, p.Type())
}

func init() {
	loggingProcessorTypes.registerType(func() component { return &LoggingProcessorParseJson{} })
}

type LoggingProcessorParseRegex struct {
	configComponent             `yaml:",inline"`
	loggingProcessorParseShared `yaml:",inline"`

	Regex string `yaml:"regex,omitempty" validate:"required"`
}

func (r LoggingProcessorParseRegex) Type() string {
	return "parse_regex"
}

func (p *LoggingProcessorParseRegex) ValidateParameters(subagent string, kind string, id string) error {
	return validateParameters(*p, subagent, kind, id, p.Type())
}

func init() {
	loggingProcessorTypes.registerType(func() component { return &LoggingProcessorParseRegex{} })
}

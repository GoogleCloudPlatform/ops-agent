// Copyright 2026 Google LLC
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

package fluentbit

// Component is a dummy placeholder to satisfy compiler references in legacy receivers.
type Component struct {
	Kind          string
	Config        map[string]string
	OrderedConfig [][2]string
}

const MetricsPort = 20202
const ExperimentalMetricsPortEnv = "EXPERIMENTAL_METRICS_PORT"

type Service struct {
	LogLevel string
}

func (s Service) Component() Component {
	return Component{}
}

type ModularConfig struct {
	Variables  map[string]string
	Components []Component
}

func (c ModularConfig) Generate() (map[string]string, error) {
	return nil, nil
}

func MetricsInputComponent() Component {
	return Component{}
}

func MetricsOutputComponent(port int) Component {
	return Component{}
}

func LuaFilterComponents(tag, function, script string) []Component {
	return nil
}

func ParserFilterComponents(tag string, key string, parsers []string, preserve bool) []Component {
	return nil
}

func ParserComponentBase(format, key string, types map[string]string, tag, uid string) (Component, string) {
	return Component{
		Config: map[string]string{},
	}, ""
}

type MultilineRule struct {
	StateName string
	Regex     string
}

func ParseMultilineComponent(name string, rules []MultilineRule) Component {
	return Component{}
}

func TranslationComponents(tag, field, newKey string, preserve bool, rules any) []Component {
	return nil
}

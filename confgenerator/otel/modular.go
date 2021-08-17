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

// Package otel provides data structures to represent and generate otel configuration.
package otel

import (
	"fmt"

	yaml "github.com/goccy/go-yaml"
	"github.com/mitchellh/mapstructure"
)

// Pipeline represents a single OT receiver and zero or more processors that must be chained after that receiver.
type Pipeline struct {
	Receiver   Component
	Processors []Component
}

// Component represents a single OT component (receiver, processor, exporter, etc.)
type Component struct {
	// Type is the string type needed to instantiate the OT component (e.g. "windowsperfcounters")
	Type string
	// Config is an object which can be serialized by mapstructure into the configuration for the component.
	// This can either be a map[string]interface{} or a Config struct from OT.
	Config interface{}
}

func (c Component) name(suffix string) string {
	if suffix != "" {
		return fmt.Sprintf("%s/%s", c.Type, suffix)
	}
	return c.Type
}

// configToYaml converts a tree of structss into a YAML file.
// To match OT's built-in config parsing, we use mapstructure to convert the tree of structs into a tree of maps.
// This allows the direct use of OT's config types at any level of the hierarchy.
func configToYaml(config interface{}) ([]byte, error) {
	outMap := make(map[string]interface{})
	if err := mapstructure.Decode(config, &outMap); err != nil {
		return nil, err
	}
	return yaml.Marshal(outMap)
}

type ModularConfig struct {
	Pipelines map[string]Pipeline
	// GlobalProcessors and Exporter are added at the end of every pipeline.
	// Only one instance of each will be created regardless of how many pipelines are defined.
	GlobalProcessors []Component
	Exporter         Component
}

// Generate an OT YAML config file for c.
// Each pipeline gets generated as a receiver, per-pipeline processors, global processors, and then global exporter.
// For example:
// metrics/mypipe:
//   receivers: [hostmetrics/mypipe]
//   processors: [filter/mypipe_1, metrics_filter/mypipe_2, resourcedetection/_global_0]
//   exporters: [googlecloud]
func (c ModularConfig) Generate() (string, error) {
	receivers := map[string]interface{}{}
	processors := map[string]interface{}{}
	exporters := map[string]interface{}{}
	pipelines := map[string]interface{}{}

	configMap := map[string]interface{}{
		"receivers":  receivers,
		"processors": processors,
		"exporters":  exporters,
		"service": map[string]interface{}{
			"pipelines": pipelines,
		},
	}
	exporterName := c.Exporter.name("")
	exporters[exporterName] = c.Exporter.Config

	var globalProcessorNames []string
	for i, processor := range c.GlobalProcessors {
		name := processor.name(fmt.Sprintf("_global_%d", i))
		globalProcessorNames = append(globalProcessorNames, name)
		processors[name] = processor.Config
	}

	for prefix, pipeline := range c.Pipelines {
		receiverName := pipeline.Receiver.name(prefix)
		receivers[receiverName] = pipeline.Receiver.Config
		var processorNames []string
		for i, processor := range pipeline.Processors {
			name := processor.name(fmt.Sprintf("%s_%d", prefix, i))
			processorNames = append(processorNames, name)
			processors[name] = processor.Config
		}
		processorNames = append(processorNames, globalProcessorNames...)
		// For now, we always generate pipelines of type "metrics".
		pipelines["metrics/"+prefix] = map[string]interface{}{
			"receivers":  []string{receiverName},
			"processors": processorNames,
			"exporters":  exporterName,
		}
	}

	out, err := configToYaml(configMap)
	// TODO: Return []byte
	if err != nil {
		return "", err
	}
	return string(out), nil
}

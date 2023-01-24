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

const MetricsPort = 20201

// ReceiverPipeline represents a single OT receiver and zero or more processors that must be chained after that receiver.
type ReceiverPipeline struct {
	Receiver Component
	// Processors is a map with processors for each pipeline type ("metrics" or "traces").
	// If a key is not in the map, the receiver pipeline will not be used for that pipeline type.
	Processors map[string][]Component
	// GMP indicates that the pipeline outputs Prometheus metrics.
	GMP bool
}

// Pipeline represents one (of potentially many) pipelines consuming data from a ReceiverPipeline.
type Pipeline struct {
	// Type is "metrics" or "traces".
	Type                 string
	ReceiverPipelineName string
	Processors           []Component
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

// configToYaml converts a tree of structs into a YAML file.
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
	LogLevel          string
	ReceiverPipelines map[string]ReceiverPipeline
	Pipelines         map[string]Pipeline

	// GlobalProcessors and Exporter are added at the end of every pipeline.
	// Only one instance of each will be created regardless of how many pipelines are defined.
	//
	// Note: GlobalProcessors are not applied to pipelines with GMP = true.
	GlobalProcessors                []Component
	GoogleCloudExporter             Component
	GoogleManagedPrometheusExporter Component
}

// Generate an OT YAML config file for c.
// Each pipeline gets generated as a receiver, per-pipeline processors, global processors, and then global exporter.
// For example:
// metrics/mypipe:
//
//	receivers: [hostmetrics/mypipe]
//	processors: [filter/mypipe_1, metrics_filter/mypipe_2, resourcedetection/_global_0]
//	exporters: [googlecloud]
func (c ModularConfig) Generate() (string, error) {
	receivers := map[string]interface{}{}
	processors := map[string]interface{}{}
	exporters := map[string]interface{}{}
	googleCloudExporter := c.GoogleCloudExporter.name("")
	googleManagedPrometheusExporter := c.GoogleManagedPrometheusExporter.name("")
	pipelines := map[string]interface{}{}
	service := map[string]map[string]interface{}{
		"pipelines": pipelines,
		"telemetry": {
			"metrics": map[string]interface{}{
				"address": fmt.Sprintf("0.0.0.0:%d", MetricsPort),
			},
		},
	}
	if c.LogLevel != "info" {
		service["telemetry"]["logs"] = map[string]interface{}{
			"level": c.LogLevel,
		}
	}

	configMap := map[string]interface{}{
		"receivers":  receivers,
		"processors": processors,
		"exporters":  exporters,
		"service":    service,
	}
	exporters[googleCloudExporter] = c.GoogleCloudExporter.Config

	// Check if there are any prometheus receivers in the pipelines.
	// If so, add the googlemanagedprometheus exporter.
	for _, r := range c.ReceiverPipelines {
		if r.GMP {
			exporters[googleManagedPrometheusExporter] = c.GoogleManagedPrometheusExporter.Config

			// Add the groupbyattrs processor so prometheus pipelines can use it.
			processors["groupbyattrs/custom_prometheus"] = gceGroupByAttrs().Config
		}
	}

	var globalProcessorNames []string
	for i, processor := range c.GlobalProcessors {
		name := processor.name(fmt.Sprintf("_global_%d", i))
		globalProcessorNames = append(globalProcessorNames, name)
		processors[name] = processor.Config
	}

	for prefix, pipeline := range c.Pipelines {
		// Receiver pipelines need to be instantiated once, since they might have more than one type.
		// We do this work more than once if it's in more than one pipeline, but it should just overwrite the same names.
		receiverPipeline := c.ReceiverPipelines[pipeline.ReceiverPipelineName]
		receiverName := receiverPipeline.Receiver.name(pipeline.ReceiverPipelineName)
		var receiverProcessorNames []string
		p, ok := receiverPipeline.Processors[pipeline.Type]
		if !ok {
			// This receiver pipeline isn't for this data type.
			continue
		}
		for i, processor := range p {
			name := processor.name(fmt.Sprintf("%s_%d", pipeline.ReceiverPipelineName, i))
			receiverProcessorNames = append(receiverProcessorNames, name)
			processors[name] = processor.Config
		}
		receivers[receiverName] = receiverPipeline.Receiver.Config

		// Everything else in the pipeline is specific to this Type.
		exporter := googleCloudExporter
		var processorNames []string
		processorNames = append(processorNames, receiverProcessorNames...)
		for i, processor := range pipeline.Processors {
			name := processor.name(fmt.Sprintf("%s_%d", prefix, i))
			processorNames = append(processorNames, name)
			processors[name] = processor.Config
		}

		// TODO: Should globalProcessorNames be appended for non-metrics receivers?
		if receiverPipeline.GMP {
			exporter = googleManagedPrometheusExporter
			processorNames = append(processorNames, "groupbyattrs/custom_prometheus")
		} else {
			processorNames = append(processorNames, globalProcessorNames...)
		}

		pipelines[pipeline.Type+"/"+prefix] = map[string]interface{}{
			"receivers":  []string{receiverName},
			"processors": processorNames,
			"exporters":  []string{exporter},
		}
	}

	out, err := configToYaml(configMap)
	// TODO: Return []byte
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func contains(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}

	return false
}

func gceGroupByAttrs() Component {
	return Component{
		Type: "groupbyattrs",
		Config: map[string]interface{}{
			"keys": []string{"namespace", "cluster", "location"},
		},
	}
}

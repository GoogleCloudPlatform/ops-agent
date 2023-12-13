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
	"context"
	"fmt"

	"github.com/GoogleCloudPlatform/ops-agent/internal/platform"
	yaml "github.com/goccy/go-yaml"
	"github.com/mitchellh/mapstructure"
	commonconfig "github.com/prometheus/common/config"
)

const MetricsPort = 20201

type ExporterType int
type ResourceDetectionMode int

const (
	// N.B. Every ExporterType increases the QPS and thus quota
	// consumption in consumer projects; think hard before adding
	// another exporter type.
	OTel ExporterType = iota
	System
	GMP
)
const (
	Override ResourceDetectionMode = iota
	SetIfMissing
	None
)

func (t ExporterType) Name() string {
	if t == System || t == GMP {
		// The collector's OTel and GMP exporters have different types so can share the empty string.
		return ""
	} else if t == OTel {
		return "otel"
	} else {
		panic("unknown ExporterType")
	}
}

// ReceiverPipeline represents a single OT receiver and zero or more processors that must be chained after that receiver.
type ReceiverPipeline struct {
	Receiver Component
	// Processors is a map with processors for each pipeline type ("metrics" or "traces").
	// If a key is not in the map, the receiver pipeline will not be used for that pipeline type.
	Processors map[string][]Component
	// ExporterTypes indicates if the pipeline outputs special data (either Prometheus or system metrics) that need to be handled with a special exporter.
	ExporterTypes map[string]ExporterType
	// ResourceDetectionModes indicates whether the resource should be forcibly set, set only if not already present, or never set.
	// If a data type is not present, it will assume the zero value (Override).
	ResourceDetectionModes map[string]ResourceDetectionMode
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
	return yaml.MarshalWithOptions(
		outMap,
		yaml.CustomMarshaler[commonconfig.Secret](func(s commonconfig.Secret) ([]byte, error) {
			return []byte(s), nil
		}),
	)
}

type ModularConfig struct {
	LogLevel          string
	ReceiverPipelines map[string]ReceiverPipeline
	Pipelines         map[string]Pipeline

	Exporters map[ExporterType]Component
}

// Generate an OT YAML config file for c.
// Each pipeline gets generated as a receiver, per-pipeline processors, global processors, and then global exporter.
// For example:
// metrics/mypipe:
//
//	receivers: [hostmetrics/mypipe]
//	processors: [filter/mypipe_1, metrics_filter/mypipe_2, resourcedetection/_global_0]
//	exporters: [googlecloud]
func (c ModularConfig) Generate(ctx context.Context) (string, error) {
	pl := platform.FromContext(ctx)
	receivers := map[string]interface{}{}
	processors := map[string]interface{}{}
	exporters := map[string]interface{}{}
	exporterNames := map[ExporterType]string{}
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

	resourceProcessorFunc := GCPResourceDetector

	resource, autodetected, err := pl.GetResource()
	if err != nil {
		return "", fmt.Errorf("can't get resource metadata: %w", err)
	}
	if !autodetected {
		resourceProcessorFunc = func(override bool) Component {
			return ResourceTransform(resource.OTelResourceAttributes(), override)
		}
	}

	resourceDetectionProcessors := map[ResourceDetectionMode]Component{
		Override:     resourceProcessorFunc(true),
		SetIfMissing: resourceProcessorFunc(false),
	}

	resourceDetectionProcessorNames := map[ResourceDetectionMode]string{
		Override:     resourceDetectionProcessors[Override].name("_global_0"),
		SetIfMissing: resourceDetectionProcessors[SetIfMissing].name("_global_1"),
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
		var processorNames []string
		processorNames = append(processorNames, receiverProcessorNames...)
		for i, processor := range pipeline.Processors {
			name := processor.name(fmt.Sprintf("%s_%d", prefix, i))
			processorNames = append(processorNames, name)
			processors[name] = processor.Config
		}
		rdm := receiverPipeline.ResourceDetectionModes[pipeline.Type]
		if name, ok := resourceDetectionProcessorNames[rdm]; ok {
			processorNames = append(processorNames, name)
			processors[name] = resourceDetectionProcessors[rdm].Config
		}
		exporterType := receiverPipeline.ExporterTypes[pipeline.Type]
		if _, ok := exporterNames[exporterType]; !ok {
			exporter := c.Exporters[exporterType]
			name := exporter.name(exporterType.Name())
			exporterNames[exporterType] = name
			exporters[name] = exporter.Config
		}

		pipelines[pipeline.Type+"/"+prefix] = map[string]interface{}{
			"receivers":  []string{receiverName},
			"processors": processorNames,
			"exporters":  []string{exporterNames[exporterType]},
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

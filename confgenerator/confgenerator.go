// Copyright 2020 Google LLC
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

// Package confgenerator represents the Ops Agent configuration and provides functions to generate subagents configuration from unified agent.
package confgenerator

import (
	"context"
	"fmt"
	"log"
	"maps"
	"path"
	"strings"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/resourcedetector"
	"github.com/GoogleCloudPlatform/ops-agent/internal/platform"
)

func otlpExporterForTraces(userAgent string) otel.Component {
	return otel.Component{
		Type: "otlp_grpc",
		Config: map[string]interface{}{
			"endpoint":      "telemetry.googleapis.com:443",
			"balancer_name": "pick_first",
			"auth": map[string]interface{}{
				"authenticator": "googleclientauth",
			},
			"user_agent": userAgent,
		},
	}
}

func otlpExporterForMetrics(userAgent string) otel.Component {
	return otel.Component{
		Type: "otlp_grpc",
		Config: map[string]interface{}{
			"endpoint": "telemetry.googleapis.com:443",
			// b/485538253: Use pick_first balancer until we can understand why round_robin is failing.
			"balancer_name": "pick_first",
			"auth": map[string]interface{}{
				"authenticator": "googleclientauth",
			},
			"user_agent": userAgent,
		},
	}
}

func otlpExporterForLogs(userAgent string) otel.Component {
	return otel.Component{
		Type: "otlp_grpc",
		Config: map[string]interface{}{
			"endpoint": "telemetry.googleapis.com:443",
			// b/485538253: Use pick_first balancer until we can understand why round_robin is failing.
			"balancer_name": "pick_first",
			"auth": map[string]interface{}{
				"authenticator": "googleclientauth",
			},
			"user_agent": userAgent,
			"sending_queue": map[string]interface{}{
				"enabled":       true,
				"queue_size":    20000000,
				"num_consumers": 10,
				"sizer":         "bytes",
				// Blocks the "sending_queue" on overflow to reduce log loss.
				"block_on_overflow": true,
				// Set batch in "sending_queue" is recommended instead of using the batch processor.
				"batch": map[string]interface{}{
					"flush_timeout": "200ms",
					"min_size":      1000000,
					"max_size":      5000000,
					"sizer":         "bytes",
				},
				"storage": fileStorageExtensionType,
			},
		},
	}
}

func (uc *UnifiedConfig) getOTelLogLevel() string {
	logLevel := "info"
	if uc.Metrics != nil && uc.Metrics.Service != nil && uc.Metrics.Service.LogLevel != "" {
		logLevel = uc.Metrics.Service.LogLevel
	}
	return logLevel
}

const (
	fileStorageExtensionType      = "file_storage"
	googleClientAuthExtensionType = "googleclientauth"
)

// fileStorageExtension returns a configured file_storage extension to be used by all receivers and exporters.
func fileStorageExtension(stateDir string) otel.Component {
	return otel.Component{
		Type: fileStorageExtensionType,
		Config: map[string]interface{}{
			"directory":        path.Join(stateDir, "file_storage"),
			"create_directory": true,
		},
	}
}

func (uc *UnifiedConfig) GenerateOtelConfig(ctx context.Context, outDir, stateDir string) (string, error) {
	p := platform.FromContext(ctx)
	userAgent, _ := p.UserAgent("Google-Cloud-Ops-Agent-Metrics")
	metricVersionLabel, _ := p.VersionLabel("google-cloud-ops-agent-metrics")

	receiverPipelines, pipelines, err := uc.generateOtelPipelines(ctx)
	if err != nil {
		return "", err
	}

	agentSelfMetrics := AgentSelfMetrics{
		MetricsVersionLabel: metricVersionLabel,
		OtelPort:            int(uc.GetOtelMetricsPort()),
		OtelRuntimeDir:      outDir,
	}
	agentSelfMetrics.AddSelfMetricsPipelines(receiverPipelines, pipelines, ctx)
	resource, err := p.GetResource()
	if err != nil {
		return "", err
	}

	otelConfig, err := otel.ModularConfig{
		LogLevel:          uc.getOTelLogLevel(),
		ReceiverPipelines: receiverPipelines,
		Pipelines:         pipelines,
		MetricsPort:       uc.GetOtelMetricsPort(),
		Exporters: map[string]otel.ExporterComponents{
			"metrics": {
				Exporter:       otlpExporterForMetrics(userAgent),
				UsedExtensions: []string{googleClientAuthExtensionType},
				Processors: []otel.Component{
					otel.GCPProjectID(resource.ProjectName()),
					otel.MetricStartTime(),
					otel.BatchProcessor(200, 200, "200ms"),
				},
			},
			"logs": {
				Exporter:       otlpExporterForLogs(userAgent),
				UsedExtensions: []string{fileStorageExtensionType, googleClientAuthExtensionType},
				Processors: []otel.Component{
					otel.GCPProjectID(resource.ProjectName()),
					otel.DisableOtlpRoundTrip(),
					otel.PreserveInstrumentationScope(),
					otel.CopyServiceResourceLabels(),
				},
			},
			"traces": {
				Exporter:       otlpExporterForTraces(userAgent),
				UsedExtensions: []string{googleClientAuthExtensionType},
				Processors: []otel.Component{
					otel.GCPProjectID(resource.ProjectName()),
					otel.BatchProcessor(200, 200, "200ms"),
				},
			},
		},
		Extensions: map[string]otel.Component{
			googleClientAuthExtensionType: {Type: googleClientAuthExtensionType, Config: map[string]string{}},
			fileStorageExtensionType:      fileStorageExtension(stateDir),
		},
	}.Generate(ctx)
	if err != nil {
		return "", err
	}
	return otelConfig, nil
}

func (p PipelineInstance) OTelComponents(ctx context.Context) (map[string]otel.ReceiverPipeline, map[string]otel.Pipeline, error) {
	outR := make(map[string]otel.ReceiverPipeline)
	outP := make(map[string]otel.Pipeline)
	receiver, ok := p.Receiver.(OTelReceiver)
	if !ok {
		return nil, nil, fmt.Errorf("%q is not an otel receiver", p.RID)
	}
	// TODO: Add a way for receivers or processors to decide whether they're compatible with a particular config.
	receiverPipelines, err := receiver.Pipelines(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("receiver %q has invalid configuration: %w", p.RID, err)
	}
	gceMetadataAttributesProcessors, err := addGceMetadataAttributesProcessor(ctx).Processors(ctx)
	if err != nil {
		panic("Failed to generate static ModifyFields")
	}
	for i, receiverPipeline := range receiverPipelines {
		receiverPipelineName := strings.ReplaceAll(p.RID, "_", "__")
		if i > 0 {
			receiverPipelineName = fmt.Sprintf("%s_%d", receiverPipelineName, i)
		}

		prefix := fmt.Sprintf("%s_%s", strings.ReplaceAll(p.PID, "_", "__"), receiverPipelineName)
		if p.PipelineType != "metrics" {
			// Don't prepend for metrics pipelines to preserve old golden configs.
			prefix = fmt.Sprintf("%s_%s", p.PipelineType, prefix)
		}

		if _, ok := receiverPipeline.Processors["logs"]; ok {
			receiverPipeline.Processors["logs"] = append(
				receiverPipeline.Processors["logs"],
				otelSetLogNameComponents(ctx, p.RID)...,
			)
			if p.Receiver.Type() == "fluent_forward" {
				receiverPipeline.Processors["logs"] = append(
					receiverPipeline.Processors["logs"],
					otelFluentForwardSetLogNameComponents()...,
				)
			}
			receiverPipeline.Processors["logs"] = append(
				receiverPipeline.Processors["logs"],
				gceMetadataAttributesProcessors...,
			)
		}

		outR[receiverPipelineName] = receiverPipeline

		pipeline := otel.Pipeline{
			Type:                 p.PipelineType,
			ReceiverPipelineName: receiverPipelineName,
		}

		for _, processorItem := range p.Processors {
			processor, ok := processorItem.Component.(OTelProcessor)
			if !ok {
				return nil, nil, fmt.Errorf("processor %q not supported in pipeline %q", processorItem.ID, p.PID)
			}
			if processors, err := processor.Processors(ctx); err != nil {
				return nil, nil, fmt.Errorf("processor %q has invalid configuration: %w", processorItem.ID, err)
			} else {
				pipeline.Processors = append(pipeline.Processors, processors...)
			}
		}
		outP[prefix] = pipeline
	}
	return outR, outP, nil
}

// generateOtelPipelines generates a map of OTel pipeline names to OTel pipelines.
func (uc *UnifiedConfig) generateOtelPipelines(ctx context.Context) (map[string]otel.ReceiverPipeline, map[string]otel.Pipeline, error) {
	outR := make(map[string]otel.ReceiverPipeline)
	outP := make(map[string]otel.Pipeline)
	pipelines, err := uc.Pipelines(ctx)
	if err != nil {
		return nil, nil, err
	}
	for _, pipeline := range pipelines {
		if pipeline.Backend != BackendOTel {
			continue
		}
		pipeR, pipeP, err := pipeline.OTelComponents(ctx)
		if err != nil {
			return nil, nil, err
		}
		maps.Copy(outR, pipeR)
		maps.Copy(outP, pipeP)
	}
	return outR, outP, nil
}

func contains(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}

	return false
}

func sliceContains(s []string, v string) bool {
	for _, e := range s {
		if e == v {
			return true
		}
	}
	return false
}

const (
	attributeLabelPrefix string = "compute.googleapis.com/attributes/"
)

// addGceMetadataAttributesProcessor annotates logs with labels corresponding
// to specific instance attributes from the GCE metadata server.
func addGceMetadataAttributesProcessor(ctx context.Context) LoggingProcessorModifyFields {
	attributes := []string{
		"dataproc-cluster-name",
		"dataproc-cluster-uuid",
		"dataproc-region",
	}

	modifications := map[string]*ModifyField{}
	p := LoggingProcessorModifyFields{
		Fields: modifications,
	}
	resource, err := platform.FromContext(ctx).GetResource()
	if err != nil {
		log.Printf("can't get resource metadata: %v", err)
		return p
	}
	gceMetadata, ok := resource.(resourcedetector.GCEResource)
	if !ok {
		// Not on GCE; no attributes to detect.
		log.Printf("ignoring the gce_metadata_attributes processor outside of GCE: %T", resource)
		return p
	}
	for _, k := range attributes {
		if v, ok := gceMetadata.Metadata[k]; ok {
			modifications[fmt.Sprintf(`labels."%s%s"`, attributeLabelPrefix, k)] = &ModifyField{
				StaticValue: &v,
			}
		}
	}
	return p
}

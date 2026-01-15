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
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"log"
	"maps"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/resourcedetector"
	"github.com/GoogleCloudPlatform/ops-agent/internal/platform"
)

func googleCloudExporter(userAgent string, instrumentationLabels bool, serviceResourceLabels bool) otel.Component {
	return otel.Component{
		Type: "googlecloud",
		Config: map[string]interface{}{
			"user_agent": userAgent,
			"metric": map[string]interface{}{
				// Receivers are responsible for sending fully-qualified metric names.
				// NB: If a receiver fails to send a full URL, OT will add the prefix `workload.googleapis.com/{metric_name}`.
				// TODO(b/197129428): Write a test to make sure this doesn't happen.
				"prefix": "",
				// OT calls CreateMetricDescriptor by default. Skip because we want
				// descriptors to be created implicitly with new time series.
				"skip_create_descriptor": true,
				// Omit instrumentation labels, which break agent metrics.
				"instrumentation_library_labels": instrumentationLabels,
				// Omit service labels, which break agent metrics.
				"service_resource_labels": serviceResourceLabels,
				"resource_filters":        []map[string]interface{}{},
			},
		},
	}
}

func ConvertPrometheusExporterToOtlpExporter(receiver otel.ReceiverPipeline, ctx context.Context) otel.ReceiverPipeline {
	return ConvertToOtlpExporter(receiver, ctx, true)
}

func ConvertGCMOtelExporterToOtlpExporter(receiver otel.ReceiverPipeline, ctx context.Context) otel.ReceiverPipeline {
	return ConvertToOtlpExporter(receiver, ctx, false)
}

func ConvertToOtlpExporter(receiver otel.ReceiverPipeline, ctx context.Context, isPrometheus bool) otel.ReceiverPipeline {
	expOtlpExporter := experimentsFromContext(ctx)["otlp_exporter"]
	resource, _ := platform.FromContext(ctx).GetResource()
	if !expOtlpExporter {
		return receiver
	}
	_, err := receiver.ExporterTypes["metrics"]
	if !err {
		return receiver
	}
	receiver.ExporterTypes["metrics"] = otel.OTLP

	receiver.Processors["metrics"] = append(receiver.Processors["metrics"], otel.GCPProjectID(resource.ProjectName()))

	// The OTLP exporter doesn't batch by default like the googlecloud.* exporters. We need this to avoid the API point limits.
	receiver.Processors["metrics"] = append(receiver.Processors["metrics"], otel.Batch())
	if isPrometheus {
		receiver.Processors["metrics"] = append(receiver.Processors["metrics"], otel.MetricUnknownCounter())
		receiver.Processors["metrics"] = append(receiver.Processors["metrics"], otel.MetricStartTime())
	}
	return receiver
}

func otlpExporter(userAgent string) otel.Component {
	return otel.Component{
		Type: "otlphttp",
		Config: map[string]interface{}{
			"endpoint": "https://telemetry.googleapis.com",
			"auth": map[string]interface{}{
				"authenticator": "googleclientauth",
			},
			"headers": map[string]string{
				"User-Agent": userAgent,
			},
		},
	}
}

func googleManagedPrometheusExporter(userAgent string) otel.Component {
	return otel.Component{
		Type: "googlemanagedprometheus",
		Config: map[string]interface{}{
			"user_agent": userAgent,
			// The exporter has the config option addMetricSuffixes with default value true. It will add Prometheus
			// style suffixes to metric names, e.g., `_total` for a counter; set to false to collect metrics as is
			"metric": map[string]interface{}{
				"add_metric_suffixes": false,
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

func (uc *UnifiedConfig) GenerateOtelConfig(ctx context.Context, outDir string) (string, error) {
	p := platform.FromContext(ctx)
	userAgent, _ := p.UserAgent("Google-Cloud-Ops-Agent-Metrics")
	metricVersionLabel, _ := p.VersionLabel("google-cloud-ops-agent-metrics")
	loggingVersionLabel, _ := p.VersionLabel("google-cloud-ops-agent-logging")

	receiverPipelines, pipelines, err := uc.generateOtelPipelines(ctx)
	if err != nil {
		return "", err
	}

	agentSelfMetrics := AgentSelfMetrics{
		MetricsVersionLabel: metricVersionLabel,
		LoggingVersionLabel: loggingVersionLabel,
		FluentBitPort:       fluentbit.MetricsPort,
		OtelPort:            otel.MetricsPort,
		OtelRuntimeDir:      outDir,
		OtelLogging:         uc.Logging.Service.OTelLogging,
	}
	agentSelfMetrics.AddSelfMetricsPipelines(receiverPipelines, pipelines)

	expOtlpExporter := experimentsFromContext(ctx)["otlp_exporter"]
	extensions := map[string]interface{}{}
	if expOtlpExporter {
		extensions["googleclientauth"] = map[string]interface{}{}
	}

	otelConfig, err := otel.ModularConfig{
		LogLevel:          uc.getOTelLogLevel(),
		ReceiverPipelines: receiverPipelines,
		Pipelines:         pipelines,
		Extensions:        extensions,
		Exporters: map[otel.ExporterType]otel.Component{
			otel.System: googleCloudExporter(userAgent, false, false),
			otel.OTel:   googleCloudExporter(userAgent, true, true),
			otel.GMP:    googleManagedPrometheusExporter(userAgent),
			otel.OTLP:   otlpExporter(userAgent),
		},
	}.Generate(ctx)
	if err != nil {
		return "", err
	}
	return otelConfig, nil
}

func (p PipelineInstance) simplifiedLoggingComponents(ctx context.Context) (InternalLoggingReceiver, []InternalLoggingProcessor, error) {
	receiver, ok := p.Receiver.(InternalLoggingReceiver)
	if !ok {
		return nil, nil, fmt.Errorf("%q is not a logging receiver", p.RID)
	}
	// Expand receiver and processors
	// TODO: What if they can be recursively expanded?
	var processors []InternalLoggingProcessor
	if r, ok := p.Receiver.(LoggingReceiverMacro); ok {
		receiver, processors = r.Expand(ctx)
	}
	for _, processorItem := range p.Processors {
		processor, ok := processorItem.Component.(InternalLoggingProcessor)
		if !ok {
			return nil, nil, fmt.Errorf("logging processor %q is incompatible with a receiver of type %q", processorItem.ID, p.Receiver.Type())
		}
		if p, ok := processor.(LoggingProcessorMacro); ok {
			processors = append(processors, p.Expand(ctx)...)
			continue
		}
		processors = append(processors, processor)
	}
	// Now that receiver and processors are all expanded, try merging them.
	for len(processors) > 0 {
		// Check if current receiver can merge processors.
		// This needs to happen every iteration because the receiver might be different after a previous merge.
		mr, ok := receiver.(InternalLoggingProcessorMerger)
		if !ok {
			return receiver, processors, nil
		}

		// Attempt processor merge.
		receiver, processors[0] = mr.MergeInternalLoggingProcessor(processors[0])
		if processors[0] != nil {
			break
		}
		processors = processors[1:]
	}
	// Now receiver has been merged as much as possible.
	return receiver, processors, nil
}

func (p PipelineInstance) FluentBitComponents(ctx context.Context) (fbSource, error) {
	tag := fmt.Sprintf("%s.%s", p.PID, p.RID)

	// For fluent_forward we create the tag in the following format:
	// <hash_string>.<pipeline_id>.<receiver_id>.<existing_tag>
	//
	// hash_string: Deterministic unique identifier for the pipeline_id + receiver_id.
	//   This is needed to prevent collisions between receivers in the same
	//   pipeline when using the glob syntax for matching (using wildcards).
	// pipeline_id: User defined pipeline_id but with the "." replaced with "_"
	//   since the "." character is reserved to be used as a delimiter in the
	//   Lua script.
	// receiver_id: User defined receiver_id but with the "." replaced with "_"
	//   since the "." character is reserved to be used as a delimiter in the
	//   Lua script.
	//  existing_tag: Tag associated with the record prior to ingesting.
	//
	// For an example testing collisions in receiver_ids, see:
	//
	// testdata/valid/linux/logging-receiver_forward_multiple_receivers_conflicting_id
	if p.Receiver.Type() == "fluent_forward" {
		hashString := getMD5Hash(tag)

		// Note that we only update the tag for the tag. The LogName will still
		// use the user defined receiver_id without this replacement.
		pipelineIdCleaned := strings.ReplaceAll(p.PID, ".", "_")
		receiverIdCleaned := strings.ReplaceAll(p.RID, ".", "_")
		tag = fmt.Sprintf("%s.%s.%s", hashString, pipelineIdCleaned, receiverIdCleaned)
	}
	receiver, processors, err := p.simplifiedLoggingComponents(ctx)
	if err != nil {
		return fbSource{}, err
	}
	var components []fluentbit.Component
	receiverComponents := receiver.Components(ctx, tag)
	components = append(components, receiverComponents...)

	// To match on fluent_forward records, we need to account for the addition
	// of the existing tag (unknown during config generation) as the suffix
	// of the tag.
	globSuffix := ""
	regexSuffix := ""
	if p.Receiver.Type() == "fluent_forward" {
		regexSuffix = `\..*`
		globSuffix = `.*`
	}
	tagRegex := regexp.QuoteMeta(tag) + regexSuffix
	tag = tag + globSuffix

	for i, processor := range processors {
		processorComponents := processor.Components(ctx, tag, strconv.Itoa(i))
		components = append(components, processorComponents...)
	}
	components = append(components, setLogNameComponents(ctx, tag, p.RID, p.Receiver.Type())...)

	// Logs ingested using the fluent_forward receiver must add the existing_tag
	// on the record to the LogName. This is done with a Lua filter.
	if p.Receiver.Type() == "fluent_forward" {
		components = append(components, fluentbit.LuaFilterComponents(tag, addLogNameLuaFunction, addLogNameLuaScriptContents)...)
	}
	return fbSource{
		TagRegex:   tagRegex,
		Components: components,
	}, nil
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

		if processors, ok := receiverPipeline.Processors["logs"]; ok {
			receiverPipeline.Processors["logs"] = append(
				processors,
				otelSetLogNameComponents(ctx, p.RID)...,
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

// GenerateFluentBitConfigs generates configuration file(s) for Fluent Bit.
// It returns a map of filenames to file contents.
func (uc *UnifiedConfig) GenerateFluentBitConfigs(ctx context.Context, logsDir string, stateDir string) (map[string]string, error) {
	userAgent, _ := platform.FromContext(ctx).UserAgent("Google-Cloud-Ops-Agent-Logging")
	components, err := uc.generateFluentbitComponents(ctx, userAgent)
	if err != nil {
		return nil, err
	}

	c := fluentbit.ModularConfig{
		Variables: map[string]string{
			"buffers_dir": path.Join(stateDir, "buffers"),
			"logs_dir":    logsDir,
		},
		Components: components,
	}
	return c.Generate()
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

// addGceMetadataAttributesComponents annotates logs with labels corresponding
// to instance attributes from the GCE metadata server.
func addGceMetadataAttributesComponents(ctx context.Context, attributes []string, tag, uid string) []fluentbit.Component {
	processorName := fmt.Sprintf("%s.%s.gce_metadata", tag, uid)
	resource, err := platform.FromContext(ctx).GetResource()
	if err != nil {
		log.Printf("can't get resource metadata: %v", err)
		return nil
	}
	gceMetadata, ok := resource.(resourcedetector.GCEResource)
	if !ok {
		// Not on GCE; no attributes to detect.
		log.Printf("ignoring the gce_metadata_attributes processor outside of GCE: %T", resource)
		return nil
	}
	modifications := map[string]*ModifyField{}
	var attributeKeys []string
	for k, _ := range gceMetadata.Metadata {
		attributeKeys = append(attributeKeys, k)
	}
	sort.Strings(attributeKeys)
	for _, k := range attributeKeys {
		if !sliceContains(attributes, k) {
			continue
		}
		v := gceMetadata.Metadata[k]
		modifications[fmt.Sprintf(`labels."%s%s"`, attributeLabelPrefix, k)] = &ModifyField{
			StaticValue: &v,
		}
	}
	if len(modifications) == 0 {
		return nil
	}
	return LoggingProcessorModifyFields{
		Fields: modifications,
	}.Components(ctx, tag, processorName)
}

type fbSource struct {
	TagRegex   string
	Components []fluentbit.Component
}

// generateFluentbitComponents generates a slice of fluentbit config sections to represent l.
func (uc *UnifiedConfig) generateFluentbitComponents(ctx context.Context, userAgent string) ([]fluentbit.Component, error) {
	l := uc.Logging
	var out []fluentbit.Component
	if l.Service.LogLevel == "" {
		l.Service.LogLevel = "info"
	}
	service := fluentbit.Service{LogLevel: l.Service.LogLevel}
	out = append(out, service.Component())
	out = append(out, fluentbit.MetricsInputComponent())

	if l != nil && l.Service != nil && !l.Service.OTelLogging {
		// Type for sorting.
		var sources []fbSource
		var tags []string
		pipelines, err := uc.Pipelines(ctx)
		if err != nil {
			return nil, err
		}
		for _, pipeline := range pipelines {
			if pipeline.Backend != BackendFluentBit {
				continue
			}
			source, err := pipeline.FluentBitComponents(ctx)
			if err != nil {
				return nil, err
			}
			sources = append(sources, source)
			tags = append(tags, source.TagRegex)
		}
		sort.Slice(sources, func(i, j int) bool { return sources[i].TagRegex < sources[j].TagRegex })
		sort.Strings(tags)

		for _, s := range sources {
			out = append(out, s.Components...)
		}
		if len(tags) > 0 {
			out = append(out, stackdriverOutputComponent(ctx, strings.Join(tags, "|"), userAgent, "2G", l.Service.Compress))
		}
		out = append(out, addGceMetadataAttributesComponents(ctx, []string{
			"dataproc-cluster-name",
			"dataproc-cluster-uuid",
			"dataproc-region",
		}, "*", "default-dataproc")...)
	}
	out = append(out, uc.generateSelfLogsComponents(ctx, userAgent)...)
	out = append(out, fluentbit.MetricsOutputComponent())

	return out, nil
}

func getMD5Hash(text string) string {
	hasher := md5.New()
	hasher.Write([]byte(text))
	return hex.EncodeToString(hasher.Sum(nil))
}

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

func googleCloudExporter(userAgent string, instrumentationLabels bool) otel.Component {
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
				// TODO: Enable with instrumentationLabels when values are sane.
				"service_resource_labels": false,
				"resource_filters":        []map[string]interface{}{},
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

func (uc *UnifiedConfig) GenerateOtelConfig(ctx context.Context) (string, error) {
	p := platform.FromContext(ctx)
	userAgent, _ := p.UserAgent("Google-Cloud-Ops-Agent-Metrics")
	metricVersionLabel, _ := p.VersionLabel("google-cloud-ops-agent-metrics")
	loggingVersionLabel, _ := p.VersionLabel("google-cloud-ops-agent-logging")

	receiverPipelines, pipelines, err := uc.generateOtelPipelines(ctx)
	if err != nil {
		return "", err
	}

	receiverPipelines["otel"] = AgentSelfMetrics{
		Version: metricVersionLabel,
		Port:    otel.MetricsPort,
	}.MetricsSubmodulePipeline()
	pipelines["otel"] = otel.Pipeline{
		Type:                 "metrics",
		ReceiverPipelineName: "otel",
	}

	receiverPipelines["fluentbit"] = AgentSelfMetrics{
		Version: loggingVersionLabel,
		Port:    fluentbit.MetricsPort,
	}.LoggingSubmodulePipeline()
	pipelines["fluentbit"] = otel.Pipeline{
		Type:                 "metrics",
		ReceiverPipelineName: "fluentbit",
	}

	if uc.Metrics.Service.LogLevel == "" {
		uc.Metrics.Service.LogLevel = "info"
	}
	otelConfig, err := otel.ModularConfig{
		LogLevel:          uc.Metrics.Service.LogLevel,
		ReceiverPipelines: receiverPipelines,
		Pipelines:         pipelines,
		Exporters: map[otel.ExporterType]otel.Component{
			otel.System: googleCloudExporter(userAgent, false),
			otel.OTel:   googleCloudExporter(userAgent, true),
			otel.GMP:    googleManagedPrometheusExporter(userAgent),
		},
	}.Generate(ctx)
	if err != nil {
		return "", err
	}
	return otelConfig, nil
}

func (p pipelineInstance) fluentBitComponents(ctx context.Context) (fbSource, error) {
	receiver, ok := p.receiver.(LoggingReceiver)
	if !ok {
		return fbSource{}, fmt.Errorf("%q is not a logging receiver", p.rID)
	}
	tag := fmt.Sprintf("%s.%s", p.pID, p.rID)

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
	if receiver.Type() == "fluent_forward" {
		hashString := getMD5Hash(tag)

		// Note that we only update the tag for the tag. The LogName will still
		// use the user defined receiver_id without this replacement.
		pipelineIdCleaned := strings.ReplaceAll(p.pID, ".", "_")
		receiverIdCleaned := strings.ReplaceAll(p.rID, ".", "_")
		tag = fmt.Sprintf("%s.%s.%s", hashString, pipelineIdCleaned, receiverIdCleaned)
	}
	var components []fluentbit.Component
	receiverComponents := receiver.Components(ctx, tag)
	components = append(components, receiverComponents...)

	// To match on fluent_forward records, we need to account for the addition
	// of the existing tag (unknown during config generation) as the suffix
	// of the tag.
	globSuffix := ""
	regexSuffix := ""
	if receiver.Type() == "fluent_forward" {
		regexSuffix = `\..*`
		globSuffix = `.*`
	}
	tagRegex := regexp.QuoteMeta(tag) + regexSuffix
	tag = tag + globSuffix

	for i, processorItem := range p.processors {
		processor, ok := processorItem.Component.(LoggingProcessor)
		if !ok {
			return fbSource{}, fmt.Errorf("logging processor %q is incompatible with a receiver of type %q", processorItem.id, receiver.Type())
		}
		processorComponents := processor.Components(ctx, tag, strconv.Itoa(i))
		if err := processUserDefinedMultilineParser(i, processorItem.id, receiver, processor, receiverComponents, processorComponents); err != nil {
			return fbSource{}, err
		}
		components = append(components, processorComponents...)
	}
	components = append(components, setLogNameComponents(ctx, tag, p.rID, receiver.Type())...)

	// Logs ingested using the fluent_forward receiver must add the existing_tag
	// on the record to the LogName. This is done with a Lua filter.
	if receiver.Type() == "fluent_forward" {
		components = append(components, fluentbit.LuaFilterComponents(tag, addLogNameLuaFunction, addLogNameLuaScriptContents)...)
	}
	return fbSource{
		tagRegex:   tagRegex,
		components: components,
	}, nil
}

func (p pipelineInstance) otelComponents(ctx context.Context) (map[string]otel.ReceiverPipeline, map[string]otel.Pipeline, error) {
	outR := make(map[string]otel.ReceiverPipeline)
	outP := make(map[string]otel.Pipeline)
	receiver, ok := p.receiver.(OTelReceiver)
	if !ok {
		return nil, nil, fmt.Errorf("%q is not an otel receiver", p.rID)
	}
	// TODO: Add a way for receivers or processors to decide whether they're compatible with a particular config.
	receiverPipelines, err := receiver.Pipelines(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("receiver %q has invalid configuration: %w", p.rID, err)
	}
	for i, receiverPipeline := range receiverPipelines {
		receiverPipelineName := strings.ReplaceAll(p.rID, "_", "__")
		if i > 0 {
			receiverPipelineName = fmt.Sprintf("%s_%d", receiverPipelineName, i)
		}

		prefix := fmt.Sprintf("%s_%s", strings.ReplaceAll(p.pID, "_", "__"), receiverPipelineName)
		if p.pipelineType != "metrics" {
			// Don't prepend for metrics pipelines to preserve old golden configs.
			prefix = fmt.Sprintf("%s_%s", p.pipelineType, prefix)
		}

		if processors, ok := receiverPipeline.Processors["logs"]; ok {
			receiverPipeline.Processors["logs"] = append(
				processors,
				otelSetLogNameComponents(ctx, p.rID)...,
			)
		}

		outR[receiverPipelineName] = receiverPipeline

		pipeline := otel.Pipeline{
			Type:                 p.pipelineType,
			ReceiverPipelineName: receiverPipelineName,
		}
		// Check the Ops Agent receiver type.
		if receiverPipeline.ExporterTypes[p.pipelineType] == otel.GMP {
			// Prometheus receivers are incompatible with processors, so we need to assert that no processors are configured.
			if len(p.processors) > 0 {
				return nil, nil, fmt.Errorf("prometheus receivers are incompatible with Ops Agent processors")
			}
		}
		for _, processorItem := range p.processors {
			processor, ok := processorItem.Component.(OTelProcessor)
			if !ok {
				return nil, nil, fmt.Errorf("processor %q not supported in pipeline %q", processorItem.id, p.pID)
			}
			if processors, err := processor.Processors(ctx); err != nil {
				return nil, nil, fmt.Errorf("processor %q has invalid configuration: %w", processorItem.id, err)
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
		if pipeline.backend != backendOTel {
			continue
		}
		pipeR, pipeP, err := pipeline.otelComponents(ctx)
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

func processUserDefinedMultilineParser(i int, pID string, receiver LoggingReceiver, processor LoggingProcessor, receiverComponents []fluentbit.Component, processorComponents []fluentbit.Component) error {
	var multilineParserNames []string
	if processor.Type() != "parse_multiline" {
		return nil
	}
	for _, p := range processorComponents {
		if p.Kind == "MULTILINE_PARSER" {
			multilineParserNames = append(multilineParserNames, p.Config["name"])
		}
	}
	allowedMultilineReceiverTypes := []string{"files"}
	for _, r := range receiverComponents {
		if len(multilineParserNames) != 0 &&
			!contains(allowedMultilineReceiverTypes, receiver.Type()) {
			return fmt.Errorf(`processor %q with type "parse_multiline" can only be applied on receivers with type "files"`, pID)
		}
		if len(multilineParserNames) != 0 {
			r.Config["multiline.parser"] = strings.Join(multilineParserNames, ",")
		}

	}
	if i != 0 {
		return fmt.Errorf(`at most one logging processor with type "parse_multiline" is allowed in the pipeline. A logging processor with type "parse_multiline" must be right after a logging receiver with type "files"`)
	}
	return nil
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
	tagRegex   string
	components []fluentbit.Component
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
			if pipeline.backend != backendFluentBit {
				continue
			}
			source, err := pipeline.fluentBitComponents(ctx)
			if err != nil {
				return nil, err
			}
			sources = append(sources, source)
			tags = append(tags, source.tagRegex)
		}
		sort.Slice(sources, func(i, j int) bool { return sources[i].tagRegex < sources[j].tagRegex })
		sort.Strings(tags)

		for _, s := range sources {
			out = append(out, s.components...)
		}
		if len(tags) > 0 {
			out = append(out, stackdriverOutputComponent(ctx, strings.Join(tags, "|"), userAgent, "2G", l.Service.Compress))
		}
		out = append(out, uc.generateSelfLogsComponents(ctx, userAgent)...)
		out = append(out, addGceMetadataAttributesComponents(ctx, []string{
			"dataproc-cluster-name",
			"dataproc-cluster-uuid",
			"dataproc-region",
		}, "*", "default-dataproc")...)
	}
	out = append(out, fluentbit.MetricsOutputComponent())

	return out, nil
}

func getMD5Hash(text string) string {
	hasher := md5.New()
	hasher.Write([]byte(text))
	return hex.EncodeToString(hasher.Sum(nil))
}

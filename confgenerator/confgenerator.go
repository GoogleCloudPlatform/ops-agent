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
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
	"github.com/GoogleCloudPlatform/ops-agent/internal/platform"
)

const fluentBitSelfLogTag = "ops-agent-fluent-bit"
const healthChecksTag = "ops-agent-health-checks"

func googleCloudExporter(userAgent string, instrumentationLabels bool) otel.Component {
	return otel.Component{
		Type: "googlecloud",
		Config: map[string]interface{}{
			// (b/233372619) Due to a constraint in the Monarch API for retrying successful data points,
			// leaving this enabled is causing adverse effects for some customers. Google OpenTelemetry team
			// recommends disabling this.
			"retry_on_failure": map[string]interface{}{
				"enabled": false,
			},
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
			// (b/233372619) Due to a constraint in the Monarch API for retrying successful data points,
			// leaving this enabled is causing adverse effects for some customers. Google OpenTelemetry team
			// recommends disabling this.
			"retry_on_failure": map[string]interface{}{
				"enabled": false,
			},
			"user_agent": userAgent,
		},
	}
}

func (uc *UnifiedConfig) GenerateOtelConfig(ctx context.Context) (string, error) {
	p := platform.FromContext(ctx)
	userAgent, _ := p.UserAgent("Google-Cloud-Ops-Agent-Metrics")
	metricVersionLabel, _ := p.VersionLabel("google-cloud-ops-agent-metrics")
	loggingVersionLabel, _ := p.VersionLabel("google-cloud-ops-agent-logging")

	receiverPipelines := make(map[string]otel.ReceiverPipeline)
	pipelines := make(map[string]otel.Pipeline)
	var err error

	if uc.Metrics != nil {
		var err error
		receiverPipelines, pipelines, err = uc.generateOtelPipelines()
		if err != nil {
			return "", err
		}
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
	}.Generate()
	if err != nil {
		return "", err
	}
	return otelConfig, nil
}

// generateOtelPipelines generates a map of OTel pipeline names to OTel pipelines.
func (uc *UnifiedConfig) generateOtelPipelines() (map[string]otel.ReceiverPipeline, map[string]otel.Pipeline, error) {
	m := uc.Metrics
	outR := make(map[string]otel.ReceiverPipeline)
	outP := make(map[string]otel.Pipeline)
	addReceiver := func(pipelineType, pID, rID string, receiver OTelReceiver, processorIDs []string) error {
		for i, receiverPipeline := range receiver.Pipelines() {
			receiverPipelineName := strings.ReplaceAll(rID, "_", "__")
			if i > 0 {
				receiverPipelineName = fmt.Sprintf("%s_%d", receiverPipelineName, i)
			}

			prefix := fmt.Sprintf("%s_%s", strings.ReplaceAll(pID, "_", "__"), receiverPipelineName)
			if pipelineType != "metrics" {
				// Don't prepend for metrics pipelines to preserve old golden configs.
				prefix = fmt.Sprintf("%s_%s", pipelineType, prefix)
			}

			outR[receiverPipelineName] = receiverPipeline

			pipeline := otel.Pipeline{
				Type:                 pipelineType,
				ReceiverPipelineName: receiverPipelineName,
			}

			// Check the Ops Agent receiver type.
			if receiverPipeline.ExporterTypes[pipelineType] == otel.GMP {
				// Prometheus receivers are incompatible with processors, so we need to assert that no processors are configured.
				if len(processorIDs) > 0 {
					return fmt.Errorf("prometheus receivers are incompatible with Ops Agent processors")
				}
			}
			for _, pID := range processorIDs {
				// TODO: Change when we support trace processors.
				processor, ok := m.Processors[pID]
				if !ok {
					return fmt.Errorf("processor %q not found", pID)
				}
				pipeline.Processors = append(pipeline.Processors, processor.Processors()...)
			}
			outP[prefix] = pipeline
		}
		return nil
	}
	if m != nil && m.Service != nil {
		receivers, err := uc.MetricsReceivers()
		if err != nil {
			return nil, nil, err
		}
		for pID, p := range m.Service.Pipelines {
			for _, rID := range p.ReceiverIDs {
				receiver, ok := receivers[rID]
				if !ok {
					return nil, nil, fmt.Errorf("metrics receiver %q not found", rID)
				}
				if err := addReceiver("metrics", pID, rID, receiver, p.ProcessorIDs); err != nil {
					return nil, nil, err
				}
			}
		}
	}
	t := uc.Traces
	if t != nil && t.Service != nil {
		receivers, err := uc.TracesReceivers()
		if err != nil {
			return nil, nil, err
		}
		for pID, p := range t.Service.Pipelines {
			for _, rID := range p.ReceiverIDs {
				receiver, ok := receivers[rID]
				if !ok {
					return nil, nil, fmt.Errorf("traces receiver %q not found", rID)
				}
				if err := addReceiver("traces", pID, rID, receiver, p.ProcessorIDs); err != nil {
					return nil, nil, err
				}
			}
		}
	}
	return outR, outP, nil
}

// GenerateFluentBitConfigs generates configuration file(s) for Fluent Bit.
// It returns a map of filenames to file contents.
func (uc *UnifiedConfig) GenerateFluentBitConfigs(ctx context.Context, logsDir string, stateDir string) (map[string]string, error) {
	userAgent, _ := platform.FromContext(ctx).UserAgent("Google-Cloud-Ops-Agent-Logging")
	components, err := uc.Logging.generateFluentbitComponents(ctx, userAgent)
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

func healthChecksLog(p platform.Platform) string {
	if p.Type == platform.Windows {
		return "${logs_dir}/health-checks.log"
	}
	return "${logs_dir}/../health-checks.log"
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

// generateFluentbitComponents generates a slice of fluentbit config sections to represent l.
func (l *Logging) generateFluentbitComponents(ctx context.Context, userAgent string) ([]fluentbit.Component, error) {
	var out []fluentbit.Component
	if l.Service.LogLevel == "" {
		l.Service.LogLevel = "info"
	}
	service := fluentbit.Service{LogLevel: l.Service.LogLevel}
	out = append(out, service.Component())
	out = append(out, fluentbit.MetricsInputComponent())

	if l != nil && l.Service != nil {
		// Type for sorting.
		type fbSource struct {
			tag        string
			components []fluentbit.Component
		}
		var sources []fbSource
		var tags []string
		for pID, p := range l.Service.Pipelines {
			for _, rID := range p.ReceiverIDs {
				receiver, ok := l.Receivers[rID]
				if !ok {
					return nil, fmt.Errorf("receiver %q not found", rID)
				}
				tag := fmt.Sprintf("%s.%s", pID, rID)

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
					pipelineIdCleaned := strings.ReplaceAll(pID, ".", "_")
					receiverIdCleaned := strings.ReplaceAll(rID, ".", "_")
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
				tags = append(tags, regexp.QuoteMeta(tag)+regexSuffix)
				tag = tag + globSuffix

				for i, pID := range p.ProcessorIDs {
					processor, ok := l.Processors[pID]
					if !ok {
						processor, ok = LegacyBuiltinProcessors[pID]
					}
					if !ok {
						return nil, fmt.Errorf("processor %q not found", pID)
					}
					processorComponents := processor.Components(ctx, tag, strconv.Itoa(i))
					if err := processUserDefinedMultilineParser(i, pID, receiver, processor, receiverComponents, processorComponents); err != nil {
						return nil, err
					}
					components = append(components, processorComponents...)
				}
				components = append(components, setLogNameComponents(ctx, tag, rID, receiver.Type(), platform.FromContext(ctx).Hostname())...)

				// Logs ingested using the fluent_forward receiver must add the existing_tag
				// on the record to the LogName. This is done with a Lua filter.
				if receiver.Type() == "fluent_forward" {
					components = append(components, fluentbit.LuaFilterComponents(tag, addLogNameLuaFunction, addLogNameLuaScriptContents)...)
				}
				sources = append(sources, fbSource{tag, components})
			}
		}
		sort.Slice(sources, func(i, j int) bool { return sources[i].tag < sources[j].tag })
		sort.Strings(tags)

		for _, s := range sources {
			out = append(out, s.components...)
		}
		if len(tags) > 0 {
			out = append(out, stackdriverOutputComponent(strings.Join(tags, "|"), userAgent, "2G"))
		}
	}
	out = append(out, LoggingReceiverFilesMixin{
		IncludePaths: []string{"${logs_dir}/logging-module.log"},
		//Following: b/226668416 temporarily set storage.type to "memory"
		//to prevent chunk corruption errors
		BufferInMemory: true,
	}.Components(ctx, fluentBitSelfLogTag)...)

	out = append(out, LoggingReceiverFilesMixin{
		IncludePaths:   []string{healthChecksLog(platform.FromContext(ctx))},
		BufferInMemory: true,
	}.Components(ctx, healthChecksTag)...)

	out = append(out, generateSeveritySelfLogsParser(ctx)...)
	out = append(out, generateHealthChecksLogsParser(ctx)...)

	out = append(out, stackdriverOutputComponent(strings.Join([]string{fluentBitSelfLogTag, healthChecksTag}, "|"), userAgent, ""))
	out = append(out, fluentbit.MetricsOutputComponent())

	return out, nil
}

func generateHealthChecksLogsParser(ctx context.Context) []fluentbit.Component {
	out := make([]fluentbit.Component, 0)
	out = append(out, LoggingProcessorParseJson{
		// TODO(b/282754149): Remove TimeKey and TimeFormat when feature gets implemented.
		ParserShared: ParserShared{
			TimeKey:    "time",
			TimeFormat: "%Y-%m-%dT%H:%M:%S%z",
		},
	}.Components(ctx, healthChecksTag, "health-checks-json")...)
	out = append(out, []fluentbit.Component{
		// This is used to exclude any previous content of the health-checks file that
		// does not contain the ops-agent-version field.
		{
			Kind: "FILTER",
			Config: map[string]string{
				"Name":  "grep",
				"Match": healthChecksTag,
				"Regex": "ops-agent-version ^.*",
			},
		},
	}...)
	return out
}

func generateSeveritySelfLogsParser(ctx context.Context) []fluentbit.Component {
	out := make([]fluentbit.Component, 0)

	parser := LoggingProcessorParseRegex{
		Regex:       `(?<message>\[[ ]*(?<time>\d+\/\d+\/\d+ \d+:\d+:\d+)] \[[ ]*(?<severity>[a-z]+)\].*)`,
		PreserveKey: true,
		ParserShared: ParserShared{
			TimeKey:    "time",
			TimeFormat: "%Y/%m/%d %H:%M:%S",
			Types: map[string]string{
				"severity": "string",
			},
		},
	}.Components(ctx, fluentBitSelfLogTag, "self-logs-severity")

	out = append(out, parser...)

	out = append(out, fluentbit.TranslationComponents(fluentBitSelfLogTag, "severity", "logging.googleapis.com/severity", true,
		[]struct{ SrcVal, DestVal string }{
			{"debug", "DEBUG"},
			{"error", "ERROR"},
			{"info", "INFO"},
			{"warn", "WARNING"},
		})...,
	)
	return out
}

func getMD5Hash(text string) string {
	hasher := md5.New()
	hasher.Write([]byte(text))
	return hex.EncodeToString(hasher.Sum(nil))
}

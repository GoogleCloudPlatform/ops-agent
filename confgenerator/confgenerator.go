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
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/template"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
	"github.com/GoogleCloudPlatform/ops-agent/internal/version"
	"github.com/shirou/gopsutil/host"
)

func (uc *UnifiedConfig) GenerateOtelConfig(hostInfo *host.InfoStat) (string, error) {
	userAgent, _ := getUserAgent("Google-Cloud-Ops-Agent-Metrics", hostInfo)
	metricVersionLabel, _ := getVersionLabel("google-cloud-ops-agent-metrics")
	loggingVersionLabel, _ := getVersionLabel("google-cloud-ops-agent-logging")

	pipelines := make(map[string]otel.Pipeline)
	if uc.Metrics != nil {
		var err error
		pipelines, err = uc.Metrics.generateOtelPipelines()
		if err != nil {
			return "", err
		}
	}

	pipelines["otel"] = AgentSelfMetrics{
		Version: metricVersionLabel,
	}.MetricsSubmodulePipeline()

	pipelines["fluentbit"] = AgentSelfMetrics{
		Version: loggingVersionLabel,
	}.LoggingSubmodulePipeline()

	if uc.Metrics.Service.LogLevel == "" {
		uc.Metrics.Service.LogLevel = "info"
	}
	otelConfig, err := otel.ModularConfig{
		LogLevel:  uc.Metrics.Service.LogLevel,
		Pipelines: pipelines,
		GlobalProcessors: []otel.Component{{
			Type: "resourcedetection",
			Config: map[string]interface{}{
				"detectors": []string{"gce"},
			},
		}},
		Exporter: otel.Component{
			Type: "googlecloud",
			Config: map[string]interface{}{
				"user_agent": userAgent,
				"metric": map[string]interface{}{
					// Receivers are responsible for sending fully-qualified metric names.
					// NB: If a receiver fails to send a full URL, OT will add the prefix `custom.googleapis.com/opencensus/`.
					// TODO(b/197129428): Write a test to make sure this doesn't happen.
					"prefix": "",
				},
			},
		},
	}.Generate()
	if err != nil {
		return "", err
	}
	return otelConfig, nil
}

func (m *Metrics) generateOtelPipelines() (map[string]otel.Pipeline, error) {
	out := make(map[string]otel.Pipeline)
	for pID, p := range m.Service.Pipelines {
		for _, rID := range p.ReceiverIDs {
			receiver, ok := m.Receivers[rID]
			if !ok {
				return nil, fmt.Errorf("receiver %q not found", rID)
			}
			for i, receiverPipeline := range receiver.Pipelines() {
				prefix := fmt.Sprintf("%s_%s", strings.ReplaceAll(pID, "_", "__"), strings.ReplaceAll(rID, "_", "__"))
				if i > 0 {
					prefix = fmt.Sprintf("%s_%d", prefix, i)
				}
				for _, pID := range p.ProcessorIDs {
					processor, ok := m.Processors[pID]
					if !ok {
						return nil, fmt.Errorf("processor %q not found", pID)
					}
					receiverPipeline.Processors = append(receiverPipeline.Processors, processor.Processors()...)
				}
				out[prefix] = receiverPipeline
			}
		}
	}
	return out, nil
}

// GenerateFluentBitConfigs generates a main and parser configuration file for Fluent Bit.
func (uc *UnifiedConfig) GenerateFluentBitConfigs(logsDir string, stateDir string, hostInfo *host.InfoStat) (map[string]string, error) {
	userAgent, _ := getUserAgent("Google-Cloud-Ops-Agent-Logging", hostInfo)
	components, err := uc.Logging.generateFluentbitComponents(userAgent, hostInfo)
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

// generateFluentbitComponents generates a slice of fluentbit config sections to represent l.
func (l *Logging) generateFluentbitComponents(userAgent string, hostInfo *host.InfoStat) ([]fluentbit.Component, error) {
	var out []fluentbit.Component
	if l.Service.LogLevel == "" {
		l.Service.LogLevel = "info"
	}
	service := fluentbit.Service{LogLevel: l.Service.LogLevel}
	out = append(out, service.Component())
	out = append(out, service.MetricsComponent())

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
				components := receiver.Components(tag)

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
					components = append(components, processor.Components(tag, strconv.Itoa(i))...)
				}
				components = append(components, setLogNameComponents(tag, rID)...)

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
			out = append(out, stackdriverOutputComponent(strings.Join(tags, "|"), userAgent))
		}
	}
	out = append(out, LoggingReceiverFilesMixin{
		IncludePaths: []string{"${logs_dir}/logging-module.log"},
	}.Components("ops-agent-fluent-bit")...)

	out = append(out, stackdriverOutputComponent("ops-agent-fluent-bit", userAgent))
	out = append(out, prometheusExporterOutputComponent())

	return out, nil
}

var versionLabelTemplate = template.Must(template.New("versionlabel").Parse(`{{.Prefix}}/{{.AgentVersion}}-{{.BuildDistro}}`))
var userAgentTemplate = template.Must(template.New("useragent").Parse(`{{.Prefix}}/{{.AgentVersion}} (BuildDistro={{.BuildDistro}};Platform={{.Platform}};ShortName={{.ShortName}};ShortVersion={{.ShortVersion}})`))

func expandTemplate(t *template.Template, prefix string, extraParams map[string]string) (string, error) {
	params := map[string]string{
		"Prefix":       prefix,
		"AgentVersion": version.Version,
		"BuildDistro":  version.BuildDistro,
	}
	for k, v := range extraParams {
		params[k] = v
	}
	var b strings.Builder
	if err := t.Execute(&b, params); err != nil {
		fmt.Println(err.Error())
		return "", err
	}
	return b.String(), nil
}

func getVersionLabel(prefix string) (string, error) {
	return expandTemplate(versionLabelTemplate, prefix, nil)
}

func getUserAgent(prefix string, hostInfo *host.InfoStat) (string, error) {
	extraParams := map[string]string{
		"Platform":     hostInfo.OS,
		"ShortName":    hostInfo.Platform,
		"ShortVersion": hostInfo.PlatformVersion,
	}
	return expandTemplate(userAgentTemplate, prefix, extraParams)
}

func getMD5Hash(text string) string {
	hasher := md5.New()
	hasher.Write([]byte(text))
	return hex.EncodeToString(hasher.Sum(nil))
}

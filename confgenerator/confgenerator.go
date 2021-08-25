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
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
	"text/template"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
	"github.com/GoogleCloudPlatform/ops-agent/internal/version"
	"github.com/shirou/gopsutil/host"
)

func (uc *UnifiedConfig) GenerateOtelConfig(hostInfo *host.InfoStat) (string, error) {
	userAgent, _ := getUserAgent("Google-Cloud-Ops-Agent-Metrics", hostInfo)
	versionLabel, _ := getVersionLabel("google-cloud-ops-agent-metrics")
	pipelines := make(map[string]otel.Pipeline)
	if uc.Metrics != nil {
		var err error
		pipelines, err = uc.Metrics.generateOtelPipelines()
		if err != nil {
			return "", err
		}
	}

	pipelines["agent"] = MetricsReceiverAgent{
		Version: versionLabel,
	}.Pipeline()

	otelConfig, err := otel.ModularConfig{
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

func (uc *UnifiedConfig) GenerateFluentBitConfigs(logsDir string, stateDir string, hostInfo *host.InfoStat) (string, string, error) {
	userAgent, _ := getUserAgent("Google-Cloud-Ops-Agent-Logging", hostInfo)
	components, err := uc.Logging.generateFluentbitComponents(userAgent, hostInfo)
	if err != nil {
		return "", "", err
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

type fbSource struct {
	tag        string
	components []fluentbit.Component
}

func (l *Logging) generateFluentbitComponents(userAgent string, hostInfo *host.InfoStat) ([]fluentbit.Component, error) {
	var out []fluentbit.Component
	out = append(out, fluentbit.Service{}.Component())

	if l != nil && l.Service != nil {
		var sources []fbSource
		var logNames []string
		for pID, p := range l.Service.Pipelines {
			for _, rID := range p.ReceiverIDs {
				receiver, ok := l.Receivers[rID]
				if !ok {
					return nil, fmt.Errorf("receiver %q not found", rID)
				}
				tag := fmt.Sprintf("%s.%s", pID, rID)
				components := receiver.Components(tag)
				for i, pID := range p.ProcessorIDs {
					processor, ok := l.Processors[pID]
					if !ok {
						processor, ok = LegacyBuiltinProcessors[pID]
					}
					if !ok {
						return nil, fmt.Errorf("processor %q not found", pID)
					}
					components = append(components, processor.Components(tag, i)...)
				}
				components = append(components, setLogNameComponents(tag, rID)...)
				logNames = append(logNames, regexp.QuoteMeta(rID))
				sources = append(sources, fbSource{tag, components})
			}
		}
		sort.Slice(sources, func(i, j int) bool { return sources[i].tag < sources[j].tag })
		sort.Strings(logNames)

		for _, s := range sources {
			out = append(out, s.components...)
		}
		if len(logNames) > 0 {
			out = append(out, fluentbit.Stackdriver{
				Match:     strings.Join(logNames, "|"),
				Workers:   8,
				UserAgent: userAgent,
			}.Component())
		}
	}
	// TODO: Use receivers instead of generating Tail objects directly.
	out = append(out, fluentbit.Tail{
		Tag:          "ops-agent-fluent-bit",
		IncludePaths: []string{"${logs_dir}/logging-module.log"},
	}.Component())
	out = append(out, fluentbit.Stackdriver{
		Match:     "ops-agent-fluent-bit",
		Workers:   8,
		UserAgent: userAgent,
	}.Component())

	return out, nil
}

func setLogNameComponents(tag, logName string) []fluentbit.Component {
	// TODO: Can we just set log_name_key in the output plugin and avoid this mess?
	return []fluentbit.Component{
		{
			Kind: "FILTER",
			Config: map[string]string{
				"Match": tag,
				"Add":   fmt.Sprintf("logName %s", logName),
				"Name":  "modify",
			},
		},
		{
			Kind: "FILTER",
			Config: map[string]string{
				"Emitter_Mem_Buf_Limit": "10M",
				"Emitter_Storage.type":  "filesystem",
				"Match":                 tag,
				"Name":                  "rewrite_tag",
				"Rule":                  "$logName .* $logName false",
			},
		},
		{
			Kind: "FILTER",
			Config: map[string]string{
				"Match":  logName,
				"Name":   "modify",
				"Remove": "logName",
			},
		},
	}
}

func (p LoggingProcessorParseShared) Components(tag string, i int) (fluentbit.Component, fluentbit.Component) {
	parserName := fmt.Sprintf("%s.%d", tag, i)
	filter := fluentbit.Component{
		Kind: "FILTER",
		Config: map[string]string{
			"Match":  tag,
			"Name":   "parser",
			"Parser": parserName,
		},
	}
	if p.Field != "" {
		filter.Config["Key_Name"] = p.Field
	}
	parser := fluentbit.Component{
		Kind: "PARSER",
		Config: map[string]string{
			"Name": parserName,
		},
	}
	if p.TimeFormat != "" {
		parser.Config["Time_Format"] = p.TimeFormat
	}
	if p.TimeKey != "" {
		parser.Config["Time_Key"] = p.TimeKey
	}
	return filter, parser
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

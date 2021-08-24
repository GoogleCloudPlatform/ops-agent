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

type sortKey struct {
	n   int
	tag string
}

func sortKeyLess(a, b sortKey) bool {
	return a.n < b.n || (a.n == b.n && a.tag < b.tag)
}

func inputSortKey(i fluentbit.Input) sortKey {
	switch r := i.(type) {
	case *fluentbit.Tail:
		return sortKey{n: 1, tag: r.Tag}
	case *fluentbit.Syslog:
		return sortKey{n: 2, tag: r.Tag}
	case *fluentbit.WindowsEventlog:
		return sortKey{n: 3, tag: r.Tag}
	}
	panic(fmt.Sprintf("unknown type: %T", i))
}

func filterSortKey(f fluentbit.Filter) sortKey {
	switch p := f.(type) {
	case fluentbit.FilterParserGroup:
		return sortKey{n: 1, tag: p[0].Match}
	case *fluentbit.FilterModifyAddLogName:
		return sortKey{n: 2, tag: p.Match}
	case *fluentbit.FilterRewriteTag:
		return sortKey{n: 3, tag: p.Match}
	case *fluentbit.FilterModifyRemoveLogName:
		return sortKey{n: 4, tag: p.Match}
	}
	panic(fmt.Sprintf("unknown type: %T", f))
}

func outputSortKey(o fluentbit.Output) sortKey {
	switch e := o.(type) {
	case *fluentbit.Stackdriver:
		return sortKey{n: 1, tag: e.Match}
	}
	panic(fmt.Sprintf("unknown type: %T", o))
}

// GenerateFluentBitConfigs generates FluentBit configuration from unified agents configuration
// in yaml. GenerateFluentBitConfigs returns empty configurations without an error if `logging`
// does not exist as a top-level field in the input yaml format.
func (uc *UnifiedConfig) GenerateFluentBitConfigs(logsDir string, stateDir string, hostInfo *host.InfoStat) (string, string, error) {
	logging := uc.Logging
	inputs := defaultTails(logsDir, hostInfo)
	userAgent, _ := getUserAgent("Google-Cloud-Ops-Agent-Logging", hostInfo)
	outputs := defaultStackdriverOutputs()
	filters := []fluentbit.Filter{}
	jsonParsers := []*fluentbit.ParserJSON{}
	regexParsers := []*fluentbit.ParserRegex{}

	if logging != nil && logging.Service != nil {
		// Override any user-specified exporters
		// TODO: Refactor remaining code to not consult these fields
		logging.Exporters = map[string]LoggingExporter{
			"google": &LoggingExporterGoogleCloudLogging{
				ConfigComponent: ConfigComponent{Type: "google_cloud_logging"},
			},
		}
		for _, p := range logging.Service.Pipelines {
			p.ExporterIDs = []string{"google"}
		}

		var err error
		extractedInputs, err := generateFluentBitInputs(logging.Receivers, logging.Service.Pipelines)
		if err != nil {
			return "", "", err
		}
		inputs = append(inputs, extractedInputs...)
		extractedFilters, err := generateFluentBitFilters(logging.Processors, logging.Service.Pipelines)
		if err != nil {
			return "", "", err
		}
		filters = append(filters, extractedFilters...)
		exporterFilters, extractedOutputs, err := extractExporterPlugins(logging.Exporters, logging.Service.Pipelines)
		if err != nil {
			return "", "", err
		}
		filters = append(filters, exporterFilters...)
		outputs = append(outputs, extractedOutputs...)
		jsonParsers, regexParsers, err = extractFluentBitParsers(logging.Processors)
		if err != nil {
			return "", "", err
		}
	}

	// make sure all collections are sorted so that generated configs are consistently generated
	sort.Slice(inputs, func(i, j int) bool { return sortKeyLess(inputSortKey(inputs[i]), inputSortKey(inputs[j])) })
	sort.Slice(filters, func(i, j int) bool { return sortKeyLess(filterSortKey(filters[i]), filterSortKey(filters[j])) })
	sort.Slice(outputs, func(i, j int) bool { return sortKeyLess(outputSortKey(outputs[i]), outputSortKey(outputs[j])) })

	for _, o := range outputs {
		s := o.(*fluentbit.Stackdriver)
		s.UserAgent = userAgent
	}

	parsers := []fluentbit.Parser{}
	for _, p := range jsonParsers {
		parsers = append(parsers, p)
	}
	for _, p := range regexParsers {
		parsers = append(parsers, p)
	}

	mainConfig, parserConfig, err := fluentbit.Config{
		StateDir: stateDir,
		LogsDir:  logsDir,
		Inputs:   inputs,
		Outputs:  outputs,
		Filters:  filters,
		Parsers:  parsers,

		UserAgent: userAgent,
	}.Generate()
	if err != nil {
		return "", "", err
	}
	return mainConfig, parserConfig, nil
}

// defaultTails returns the default Tail sections for the agents' own logs.
func defaultTails(logsDir string, hostInfo *host.InfoStat) (tails []fluentbit.Input) {
	tails = []fluentbit.Input{}
	tailFluentbit := fluentbit.Tail{
		Tag:          "ops-agent-fluent-bit",
		IncludePaths: []string{"${logs_dir}/logging-module.log"},
	}
	tailCollectd := fluentbit.Tail{
		Tag:          "ops-agent-collectd",
		IncludePaths: []string{"${logs_dir}/metrics-module.log"},
	}
	tails = append(tails, &tailFluentbit)
	if hostInfo.OS != "windows" {
		tails = append(tails, &tailCollectd)
	}

	return tails
}

// defaultStackdriverOutputs returns the default Stackdriver sections for the agents' own logs.
func defaultStackdriverOutputs() (stackdrivers []fluentbit.Output) {
	return []fluentbit.Output{
		&fluentbit.Stackdriver{
			Match:   "ops-agent-fluent-bit|ops-agent-collectd",
			Workers: 8,
		},
	}
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

func generateFluentBitInputs(receivers map[string]LoggingReceiver, pipelines map[string]*LoggingPipeline) ([]fluentbit.Input, error) {
	inputs := []fluentbit.Input{}
	for _, pID := range sortedKeys(pipelines) {
		p := pipelines[pID]
		for _, rID := range p.ReceiverIDs {
			if r, ok := receivers[rID]; ok {
				switch r := r.(type) {
				case *LoggingReceiverFiles:
					fbTail := fluentbit.Tail{
						Tag:          fmt.Sprintf("%s.%s", pID, rID),
						IncludePaths: r.IncludePaths,
					}
					if len(r.ExcludePaths) != 0 {
						fbTail.ExcludePaths = r.ExcludePaths
					}
					inputs = append(inputs, &fbTail)
				case *LoggingReceiverSyslog:
					inputs = append(inputs, &fluentbit.Syslog{
						Tag:    fmt.Sprintf("%s.%s", pID, rID),
						Listen: r.ListenHost,
						Mode:   r.TransportProtocol,
						Port:   r.ListenPort,
					})
				case *LoggingReceiverWinevtlog:
					inputs = append(inputs, &fluentbit.WindowsEventlog{
						Tag:          fmt.Sprintf("%s.%s", pID, rID),
						Channels:     r.Channels,
						Interval_Sec: "1",
					})
				}
			}
		}
	}
	return inputs, nil
}

func generateFluentBitFilters(processors map[string]LoggingProcessor, pipelines map[string]*LoggingPipeline) ([]fluentbit.Filter, error) {
	// Note: Keep each pipeline's filters in a separate group, because
	// the order within that group is important, even though the order
	// of the groups themselves does not matter.
	groups := []fluentbit.Filter{}
	for _, pID := range sortedKeys(pipelines) {
		fbFilterParsers := fluentbit.FilterParserGroup{}
		pipeline := pipelines[pID]
		for _, processorID := range pipeline.ProcessorIDs {
			p, ok := processors[processorID]
			fbFilterParser := fluentbit.FilterParser{
				Match:   fmt.Sprintf("%s.*", pID),
				Parser:  processorID,
				KeyName: "message",
			}
			if ok && p.GetField() != "" {
				fbFilterParser.KeyName = p.GetField()
			}
			fbFilterParsers = append(fbFilterParsers, &fbFilterParser)
		}
		if len(fbFilterParsers) > 0 {
			groups = append(groups, fbFilterParsers)
		}
	}
	return groups, nil
}

func extractExporterPlugins(exporters map[string]LoggingExporter, pipelines map[string]*LoggingPipeline) (
	[]fluentbit.Filter, []fluentbit.Output, error) {
	filters := []fluentbit.Filter{}
	outputs := []fluentbit.Output{}
	stackdriverExporters := make(map[string][]string)
	for _, pID := range sortedKeys(pipelines) {
		pipeline := pipelines[pID]
		for _, exporterID := range pipeline.ExporterIDs {
			// for each receiver, generate a output plugin with the specified receiver id
			for _, rID := range pipeline.ReceiverIDs {
				filters = append(filters, &fluentbit.FilterModifyAddLogName{
					Match:   fmt.Sprintf("%s.%s", pID, rID),
					LogName: rID,
				})
				// generate single rewriteTag for this pipeline
				filters = append(filters, &fluentbit.FilterRewriteTag{
					Match: fmt.Sprintf("%s.%s", pID, rID),
				})
				filters = append(filters, &fluentbit.FilterModifyRemoveLogName{
					Match: rID,
				})
				stackdriverExporters[exporterID] = append(stackdriverExporters[exporterID], rID)
			}
		}
	}
	for _, tags := range stackdriverExporters {
		outputs = append(outputs, &fluentbit.Stackdriver{
			Match:   strings.Join(tags, "|"),
			Workers: 8,
		})
	}
	return filters, outputs, nil
}

func extractFluentBitParsers(processors map[string]LoggingProcessor) ([]*fluentbit.ParserJSON, []*fluentbit.ParserRegex, error) {
	fbJSONParsers := []*fluentbit.ParserJSON{}
	fbRegexParsers := []*fluentbit.ParserRegex{}
	for _, name := range sortedKeys(processors) {
		p := processors[name]
		switch p := p.(type) {
		case *LoggingProcessorParseJson:
			fbJSONParser := fluentbit.ParserJSON{
				Name:       name,
				TimeKey:    p.TimeKey,
				TimeFormat: p.TimeFormat,
			}
			fbJSONParsers = append(fbJSONParsers, &fbJSONParser)
		case *LoggingProcessorParseRegex:
			fbRegexParser := fluentbit.ParserRegex{
				Name:       name,
				Regex:      p.Regex,
				TimeKey:    p.TimeKey,
				TimeFormat: p.TimeFormat,
			}
			fbRegexParsers = append(fbRegexParsers, &fbRegexParser)
		}
	}
	return fbJSONParsers, fbRegexParsers, nil
}

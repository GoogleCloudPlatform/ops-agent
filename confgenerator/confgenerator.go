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
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
	"github.com/GoogleCloudPlatform/ops-agent/internal/version"
	"github.com/shirou/gopsutil/host"
)

// filepathJoin uses the real filepath.Join in actual executable
// but can be overriden in tests to impersonate an alternate OS.
var filepathJoin = defaultFilepathJoin

func defaultFilepathJoin(_ string, elem ...string) string {
	return filepath.Join(elem...)
}

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
	// TODO: Add resourcedetection processor to every pipeline

	otelConfig, err := otel.ModularConfig{
		Pipelines: pipelines,
		Exporter: otel.Component{
			Type: "googlecloud",
			Config: map[string]interface{}{
				"user_agent": userAgent,
				"metric": map[string]interface{}{
					"prefix": "agent.googleapis.com/",
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

func generateOtelReceivers(receivers map[string]MetricsReceiver, pipelines map[string]*MetricsPipeline) ([]otel.Receiver, map[string]otel.Receiver, error) {
	var receiverList []otel.Receiver
	receiverMap := make(map[string]otel.Receiver)
	for _, pID := range sortedKeys(pipelines) {
		p := pipelines[pID]
		for _, rID := range p.ReceiverIDs {
			if _, ok := receiverMap[rID]; ok {
				continue
			}
			if r, ok := receivers[rID]; ok {
				switch r := r.(type) {
				case *MetricsReceiverHostmetrics:
					hostMetrics := otel.HostMetrics{
						HostMetricsID:      "hostmetrics/" + rID,
						CollectionInterval: r.CollectionInterval,
					}
					receiverList = append(receiverList, &hostMetrics)
					receiverMap[rID] = &hostMetrics
				case *MetricsReceiverMssql:
					mssql := otel.MSSQL{
						MSSQLID:            "windowsperfcounters/mssql_" + rID,
						CollectionInterval: r.CollectionInterval,
					}
					receiverList = append(receiverList, &mssql)
					receiverMap[rID] = &mssql
				case *MetricsReceiverIis:
					iis := otel.IIS{
						IISID:              "windowsperfcounters/iis_" + rID,
						CollectionInterval: r.CollectionInterval,
					}
					receiverList = append(receiverList, &iis)
					receiverMap[rID] = &iis
				}
			}
		}
	}
	return receiverList, receiverMap, nil
}

func generateOtelExporters(exporters map[string]MetricsExporter, pipelines map[string]*MetricsPipeline) ([]otel.Exporter, map[string]otel.Exporter, error) {
	exporterList := []otel.Exporter{}
	exporterMap := make(map[string]otel.Exporter)
	for _, pID := range sortedKeys(pipelines) {
		p := pipelines[pID]
		for _, eID := range p.ExporterIDs {
			if _, ok := exporterMap[eID]; ok {
				continue
			}
			if exporter, ok := exporters[eID]; ok {
				switch exporter.Type() {
				case "google_cloud_monitoring":
					stackdriver := otel.Stackdriver{
						StackdriverID: "googlecloud/" + eID,
						Prefix:        "agent.googleapis.com/",
					}
					exporterList = append(exporterList, &stackdriver)
					exporterMap[eID] = &stackdriver
				}
			}
		}
	}
	return exporterList, exporterMap, nil
}

func generateOtelProcessors(processors map[string]MetricsProcessor, pipelines map[string]*MetricsPipeline) ([]otel.Processor, map[string]otel.Processor, error) {
	processorList := []otel.Processor{}
	processorMap := make(map[string]otel.Processor)
	for _, pID := range sortedKeys(pipelines) {
		p := pipelines[pID]
		for _, processorID := range p.ProcessorIDs {
			if _, ok := processorMap[processorID]; ok {
				continue
			}
			if p, ok := processors[processorID]; ok {
				switch p.Type() {
				case "exclude_metrics":
					p := p.(*MetricsProcessorExcludeMetrics)
					var metricNames []string
					for _, glob := range p.MetricsPattern {
						// TODO: Remove TrimPrefix when we support metrics with other prefixes.
						glob = strings.TrimPrefix(glob, "agent.googleapis.com/")
						// TODO: Move this glob to regexp into a template function inside otel/conf.go.
						var literals []string
						for _, g := range strings.Split(glob, "*") {
							literals = append(literals, regexp.QuoteMeta(g))
						}
						metricNames = append(metricNames, fmt.Sprintf(`^%s$`, strings.Join(literals, `.*`)))
					}
					excludeMetrics := otel.ExcludeMetrics{
						ExcludeMetricsID: "filter/exclude_" + processorID,
						MetricNames:      metricNames,
					}
					processorList = append(processorList, &excludeMetrics)
					processorMap[processorID] = &excludeMetrics
				}
			}
		}
	}
	return processorList, processorMap, nil
}

func generateOtelServices(receiverMap map[string]otel.Receiver, exporterMap map[string]otel.Exporter, processorMap map[string]otel.Processor, pipelines map[string]*MetricsPipeline) ([]*otel.Service, error) {
	serviceList := []*otel.Service{}
	for _, pID := range sortedKeys(pipelines) {
		p := pipelines[pID]
		for _, rID := range p.ReceiverIDs {
			r, ok := receiverMap[rID]
			if !ok {
				panic(fmt.Sprintf("Internal error: receiver %q not found", rID))
			}

			var processorIDs []string
			processorIDs = append(processorIDs, r.DefaultProcessors()...)
			for _, processorID := range p.ProcessorIDs {
				processorIDs = append(processorIDs, processorMap[processorID].GetID())
			}

			var pExportIDs []string
			for _, eID := range p.ExporterIDs {
				pExportIDs = append(pExportIDs, exporterMap[eID].GetID())
			}
			service := otel.Service{
				ID:         r.DefaultPipelineID(),
				Receivers:  []string{r.GetID()},
				Processors: processorIDs,
				Exporters:  pExportIDs,
			}
			serviceList = append(serviceList, &service)
		}
	}
	return serviceList, nil
}

// GenerateFluentBitConfigs generates FluentBit configuration from unified agents configuration
// in yaml. GenerateFluentBitConfigs returns empty configurations without an error if `logging`
// does not exist as a top-level field in the input yaml format.
func (uc *UnifiedConfig) GenerateFluentBitConfigs(logsDir string, stateDir string, hostInfo *host.InfoStat) (string, string, error) {
	logging := uc.Logging
	fbTails := defaultTails(logsDir, stateDir, hostInfo)
	userAgent, _ := getUserAgent("Google-Cloud-Ops-Agent-Logging", hostInfo)
	fbStackdrivers := defaultStackdriverOutputs(hostInfo)
	fbSyslogs := []*fluentbit.Syslog{}
	fbWinEventlogs := []*fluentbit.WindowsEventlog{}
	fbFilterParserGroups := []fluentbit.FilterParserGroup{}
	fbFilterAddLogNames := []*fluentbit.FilterModifyAddLogName{}
	fbFilterRewriteTags := []*fluentbit.FilterRewriteTag{}
	fbFilterRemoveLogNames := []*fluentbit.FilterModifyRemoveLogName{}
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

		extractedTails := []*fluentbit.Tail{}
		var err error
		extractedTails, fbSyslogs, fbWinEventlogs, err = generateFluentBitInputs(logging.Receivers, logging.Service.Pipelines, stateDir, hostInfo)
		if err != nil {
			return "", "", err
		}
		fbTails = append(fbTails, extractedTails...)
		fbFilterParserGroups, err = generateFluentBitFilters(logging.Processors, logging.Service.Pipelines)
		if err != nil {
			return "", "", err
		}
		extractedStackdrivers := []*fluentbit.Stackdriver{}
		fbFilterAddLogNames, fbFilterRewriteTags, fbFilterRemoveLogNames, extractedStackdrivers, err = extractExporterPlugins(logging.Exporters, logging.Service.Pipelines, hostInfo)
		if err != nil {
			return "", "", err
		}
		fbStackdrivers = append(fbStackdrivers, extractedStackdrivers...)
		jsonParsers, regexParsers, err = extractFluentBitParsers(logging.Processors)
		if err != nil {
			return "", "", err
		}
	}
	mainConfig, err := fluentbit.GenerateFluentBitMainConfig(fbTails, fbSyslogs, fbWinEventlogs, fbFilterParserGroups, fbFilterAddLogNames, fbFilterRewriteTags, fbFilterRemoveLogNames, fbStackdrivers, userAgent)
	if err != nil {
		return "", "", err
	}
	parserConfig, err := fluentbit.GenerateFluentBitParserConfig(jsonParsers, regexParsers)
	if err != nil {
		return "", "", err
	}
	return mainConfig, parserConfig, nil
}

// defaultTails returns the default Tail sections for the agents' own logs.
func defaultTails(logsDir string, stateDir string, hostInfo *host.InfoStat) (tails []*fluentbit.Tail) {
	tails = []*fluentbit.Tail{}
	tailFluentbit := fluentbit.Tail{
		Tag:  "ops-agent-fluent-bit",
		DB:   filepathJoin(hostInfo.OS, stateDir, "buffers", "ops-agent-fluent-bit"),
		Path: filepathJoin(hostInfo.OS, logsDir, "logging-module.log"),
	}
	tailCollectd := fluentbit.Tail{
		Tag:  "ops-agent-collectd",
		DB:   filepathJoin(hostInfo.OS, stateDir, "buffers", "ops-agent-collectd"),
		Path: filepathJoin(hostInfo.OS, logsDir, "metrics-module.log"),
	}
	tails = append(tails, &tailFluentbit)
	if hostInfo.OS != "windows" {
		tails = append(tails, &tailCollectd)
	}

	return tails
}

// defaultStackdriverOutputs returns the default Stackdriver sections for the agents' own logs.
func defaultStackdriverOutputs(hostInfo *host.InfoStat) (stackdrivers []*fluentbit.Stackdriver) {
	return []*fluentbit.Stackdriver{
		{
			Match:   "ops-agent-fluent-bit|ops-agent-collectd",
			Workers: getWorkers(hostInfo),
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

func getWorkers(hostInfo *host.InfoStat) int {
	if hostInfo.OS == "linux" {
		return 8
	} else {
		return 0
	}
}

func generateFluentBitInputs(receivers map[string]LoggingReceiver, pipelines map[string]*LoggingPipeline, stateDir string, hostInfo *host.InfoStat) ([]*fluentbit.Tail, []*fluentbit.Syslog, []*fluentbit.WindowsEventlog, error) {
	fbTails := []*fluentbit.Tail{}
	fbSyslogs := []*fluentbit.Syslog{}
	fbWinEventlogs := []*fluentbit.WindowsEventlog{}
	for _, pID := range sortedKeys(pipelines) {
		p := pipelines[pID]
		for _, rID := range p.ReceiverIDs {
			if r, ok := receivers[rID]; ok {
				switch r.Type() {
				case "files":
					r := r.(*LoggingReceiverFiles)
					fbTail := fluentbit.Tail{
						Tag:  fmt.Sprintf("%s.%s", pID, rID),
						DB:   filepathJoin(hostInfo.OS, stateDir, "buffers", pID+"_"+rID),
						Path: strings.Join(r.IncludePaths, ","),
					}
					if len(r.ExcludePaths) != 0 {
						fbTail.ExcludePath = strings.Join(r.ExcludePaths, ",")
					}
					fbTails = append(fbTails, &fbTail)
				case "syslog":
					r := r.(*LoggingReceiverSyslog)
					fbSyslog := fluentbit.Syslog{
						Tag:    fmt.Sprintf("%s.%s", pID, rID),
						Listen: r.ListenHost,
						Mode:   r.TransportProtocol,
						Port:   r.ListenPort,
					}
					fbSyslogs = append(fbSyslogs, &fbSyslog)
				case "windows_event_log":
					r := r.(*LoggingReceiverWinevtlog)
					fbWinlog := fluentbit.WindowsEventlog{
						Tag:          fmt.Sprintf("%s.%s", pID, rID),
						Channels:     strings.Join(r.Channels, ","),
						Interval_Sec: "1",
						DB:           filepathJoin(hostInfo.OS, stateDir, "buffers", pID+"_"+rID),
					}
					fbWinEventlogs = append(fbWinEventlogs, &fbWinlog)
				}
			}
		}
	}
	return fbTails, fbSyslogs, fbWinEventlogs, nil
}

func generateFluentBitFilters(processors map[string]LoggingProcessor, pipelines map[string]*LoggingPipeline) ([]fluentbit.FilterParserGroup, error) {
	// Note: Keep each pipeline's filters in a separate group, because
	// the order within that group is important, even though the order
	// of the groups themselves does not matter.
	groups := []fluentbit.FilterParserGroup{}
	for _, pID := range sortedKeys(pipelines) {
		fbFilterParsers := []*fluentbit.FilterParser{}
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

func extractExporterPlugins(exporters map[string]LoggingExporter, pipelines map[string]*LoggingPipeline, hostInfo *host.InfoStat) (
	[]*fluentbit.FilterModifyAddLogName, []*fluentbit.FilterRewriteTag, []*fluentbit.FilterModifyRemoveLogName, []*fluentbit.Stackdriver, error) {
	fbFilterModifyAddLogNames := []*fluentbit.FilterModifyAddLogName{}
	fbFilterRewriteTags := []*fluentbit.FilterRewriteTag{}
	fbFilterModifyRemoveLogNames := []*fluentbit.FilterModifyRemoveLogName{}
	fbStackdrivers := []*fluentbit.Stackdriver{}
	stackdriverExporters := make(map[string][]string)
	for _, pID := range sortedKeys(pipelines) {
		pipeline := pipelines[pID]
		for _, exporterID := range pipeline.ExporterIDs {
			// for each receiver, generate a output plugin with the specified receiver id
			for _, rID := range pipeline.ReceiverIDs {
				fbFilterModifyAddLogNames = append(fbFilterModifyAddLogNames, &fluentbit.FilterModifyAddLogName{
					Match:   fmt.Sprintf("%s.%s", pID, rID),
					LogName: rID,
				})
				// generate single rewriteTag for this pipeline
				fbFilterRewriteTags = append(fbFilterRewriteTags, &fluentbit.FilterRewriteTag{
					Match: fmt.Sprintf("%s.%s", pID, rID),
				})
				fbFilterModifyRemoveLogNames = append(fbFilterModifyRemoveLogNames, &fluentbit.FilterModifyRemoveLogName{
					Match: rID,
				})
				stackdriverExporters[exporterID] = append(stackdriverExporters[exporterID], rID)
			}
		}
	}
	for _, tags := range stackdriverExporters {
		fbStackdrivers = append(fbStackdrivers, &fluentbit.Stackdriver{
			Match:   strings.Join(tags, "|"),
			Workers: getWorkers(hostInfo),
		})
	}
	return fbFilterModifyAddLogNames, fbFilterRewriteTags, fbFilterModifyRemoveLogNames, fbStackdrivers, nil
}

func extractFluentBitParsers(processors map[string]LoggingProcessor) ([]*fluentbit.ParserJSON, []*fluentbit.ParserRegex, error) {
	fbJSONParsers := []*fluentbit.ParserJSON{}
	fbRegexParsers := []*fluentbit.ParserRegex{}
	for _, name := range sortedKeys(processors) {
		p := processors[name]
		switch t := p.Type(); t {
		case "parse_json":
			p := p.(*LoggingProcessorParseJson)
			fbJSONParser := fluentbit.ParserJSON{
				Name:       name,
				TimeKey:    p.TimeKey,
				TimeFormat: p.TimeFormat,
			}
			fbJSONParsers = append(fbJSONParsers, &fbJSONParser)
		case "parse_regex":
			p := p.(*LoggingProcessorParseRegex)
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

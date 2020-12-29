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

// Package confgenerator provides functions to generate subagents configuration from unified agent.
package confgenerator

import (
	"fmt"
	"sort"
	"strings"

	"github.com/GoogleCloudPlatform/ops-agent/collectd"
	"github.com/GoogleCloudPlatform/ops-agent/fluentbit/conf"
	"github.com/GoogleCloudPlatform/ops-agent/otel"

	yaml "gopkg.in/yaml.v2"
)

type unifiedConfig struct {
	Logging *logging         `yaml:"logging"`
	Metrics collectd.Metrics `yaml:"metrics"`
}

type unifiedConfigWindows struct {
	Logging *logging         `yaml:"logging"`
	Metrics *otelMetrics     `yaml:"metrics"`
}

type logging struct {
	Receivers  map[string]*receiver  `yaml:"receivers"`
	Processors map[string]*processor `yaml:"processors"`
	Exporters  map[string]*exporter  `yaml:"exporters"`
	Service    *loggingService       `yaml:"service"`
}

type receiver struct {
	// Required. It is either file or syslog.
	Type string `yaml:"type"`

	// Valid for type "files".
	IncludePaths []string `yaml:"include_paths"`
	ExcludePaths []string `yaml:"exclude_paths"`

	// Valid for type "syslog".
	TransportProtocol string `yaml:"transport_protocol"`
	ListenHost        string `yaml:"listen_host"`
	ListenPort        uint16 `yaml:"listen_port"`

	//Valid for type "windows_event_log".
	Channels []string `yaml:"channels"`
}

type processor struct {
	// Required. It is either parse_json or parse_regex.
	Type string `yaml:"type"`

	// Valid for parse_regex only.
	Regex string `yaml:"regex"`

	// Valid for type parse_json and parse_regex.
	Field      string `yaml:"field"`       // optional, default to "message"
	TimeKey    string `yaml:"time_key"`    // optional, by default does not parse timestamp
	TimeFormat string `yaml:"time_format"` // optional, must be provided if time_key is present
}

type exporter struct {
	// Required. It can only be `google_cloud_logging` now. More type may be supported later.
	Type string `yaml:"type"`
}

type loggingService struct {
	Pipelines map[string]*loggingPipeline
}

type loggingPipeline struct {
	Receivers  []string `yaml:"receivers"`
	Processors []string `yaml:"processors"`
	Exporters  []string `yaml:"exporters"`
}

type otelReceiver struct {
        Type string `yaml:"type"`
	// Valid for type "hostmetrics".
        CollectionInterval string `yaml:"collection_interval"`
}

type otelExporter struct {
        Type string `yaml:"type"`
}

type otelMetrics struct {
    Receivers map[string]*otelReceiver `yaml:"receivers"`
    Exporters map[string]*otelExporter `yaml:"exporters"`
	Service *otelService `yaml:"service"`
}

type otelService struct {
	Pipelines map[string]*otelPipeline
}

type otelPipeline struct {
	Receivers  []string `yaml:"receivers"`
	Processors []string `yaml:"processors"`
	Exporters  []string `yaml:"exporters"`
}

func GenerateOtelConfig(input []byte, logsDir string) (config string, err error){
	unifiedConfig, err := unifiedConfigWindowsReader(input)
	if err != nil {
		return "", err
	}
	otelConfig, err := generateOtelConfig(unifiedConfig.Metrics, logsDir)
	if err != nil {
		return "", err
	}
	return otelConfig, nil
}

func GenerateCollectdConfig(input []byte, logsDir string) (config string, err error) {
	unifiedConfig, err := unifiedConfigReader(input)
	if err != nil {
		return "", err
	}
	collectdConfig, err := collectd.GenerateCollectdConfig(unifiedConfig.Metrics, logsDir)
	if err != nil {
		return "", err
	}
	return collectdConfig, nil
}

// GenerateFluentBitConfigs generates FluentBit configuration from unified agents configuration
// in yaml. GenerateFluentBitConfigs returns empty configurations without an error if `logs`
// does not exist as a top-level field in the input yaml format.
func GenerateFluentBitConfigs(input []byte, logsDir string, stateDir string) (mainConfig string, parserConfig string, err error) {
	unifiedConfig, err := unifiedConfigReader(input)
	if err != nil {
		return "", "", err
	}
	if unifiedConfig.Logging == nil {
		return "", "", nil
	}
	return generateFluentBitConfigs(unifiedConfig.Logging, logsDir, stateDir)
}

func unifiedConfigReader(input []byte) (unifiedConfig, error) {
	config := unifiedConfig{}
	err := yaml.UnmarshalStrict(input, &config)
	if err != nil {
		return unifiedConfig{}, err
	}
	return config, nil
}

func unifiedConfigWindowsReader(input []byte) (unifiedConfigWindows, error) {
	config := unifiedConfigWindows{}
	err := yaml.UnmarshalStrict(input, &config)
	if err != nil {
		return unifiedConfigWindows{}, err
	}
	return config, nil
}

func generateOtelConfig(metrics *otelMetrics, logsDir string) (string, error){
	hostMetricsList := []*otel.HostMetrics{}
	stackdriverList := []*otel.Stackdriver{}
	serviceList := []*otel.Service{}
	receiverNameMap := make(map[string]string)
	exporterNameMap := make(map[string]string)
	if metrics.Service != nil {
		hostmetricsReceiverFactories, err := extractOtelReceiverFactories(metrics.Receivers)
		if err != nil {
			return "", err
		}
		hostMetricsList, receiverNameMap, err = generateOtelReceivers(hostmetricsReceiverFactories, metrics.Service.Pipelines)
		if err != nil {
			return "", err
		}
		stackdriverList, exporterNameMap, err = generateOtelExporters(metrics.Exporters, metrics.Service.Pipelines)
		if err != nil {
			return "", err
		}
		serviceList, err = generateOtelServices(receiverNameMap, exporterNameMap, metrics.Service.Pipelines)
	}
	otelConfig, err := otel.GenerateOtelConfig(hostMetricsList, stackdriverList, serviceList)
	if err != nil {
		return "", err
	}
	fmt.Print(otelConfig)
	return otelConfig, nil
}

func generateOtelServices(receiverNameMap map[string]string, exporterNameMap map[string]string, pipelines map[string]*otelPipeline) ([]*otel.Service, error) {
	serviceList := []*otel.Service{}
	var pipelineIDs []string
	for p := range pipelines {
		pipelineIDs = append(pipelineIDs, p)
	}
	sort.Strings(pipelineIDs)
	for _, pID := range pipelineIDs {
		p := pipelines[pID]
		var pRecevieIDs []string
		var isHostMetrics bool
		for _, rID := range p.Receivers {
			pRecevieIDs = append(pRecevieIDs, receiverNameMap[rID])
			if rID == "hostmetrics" {
				isHostMetrics = true
			}
		}
                var pExportIDs []string
                for _, eID := range p.Exporters {
                        pExportIDs = append(pExportIDs, exporterNameMap[eID])
                }
		service := otel.Service{}
		if isHostMetrics {
			defaultProcessors := []string{"agentmetrics/system", "filter/system", "metricstransform/system", "resourcedetection"}
			service = otel.Service{
				ID: "system",
				Receivers: fmt.Sprintf("[%s]", strings.Join(pRecevieIDs, ",")),
				Processors: fmt.Sprintf("[%s]", strings.Join(defaultProcessors, ",")),
				Exporters: fmt.Sprintf("[%s]", strings.Join(pExportIDs, ",")),
			}
		} else {
			service = otel.Service{
				ID: pID,
				Receivers: fmt.Sprintf("[%s]", strings.Join(pRecevieIDs, ",")),
				Exporters: fmt.Sprintf("[%s]", strings.Join(pExportIDs, ",")),
				Processors: fmt.Sprintf("[%s]", strings.Join(p.Processors, ",")),
			}
		}

		serviceList = append(serviceList, &service)
	}
	return serviceList, nil
}

// defaultTails returns the default Tail sections for the agents' own logs.
func defaultTails(logsDir string, stateDir string) (tails []*conf.Tail) {
	return []*conf.Tail{
		{
			Tag:  "ops-agent-fluent-bit",
			DB:   fmt.Sprintf("%s/buffers/ops-agent-fluent-bit", stateDir),
			Path: fmt.Sprintf("%s/logging-module.log", logsDir),
		},
		{
			Tag:  "ops-agent-collectd",
			DB:   fmt.Sprintf("%s/buffers/ops-agent-collectd", stateDir),
			Path: fmt.Sprintf("%s/metrics-module.log", logsDir),
		},
	}
}

// defaultStackdriverOutputs returns the default Stackdriver sections for the agents' own logs.
func defaultStackdriverOutputs() (stackdrivers []*conf.Stackdriver) {
	return []*conf.Stackdriver{
		{
			Match: "ops-agent-fluent-bit",
		},
		{
			Match: "ops-agent-collectd",
		},
	}
}

func generateFluentBitConfigs(logging *logging, logsDir string, stateDir string) (string, string, error) {
	fbTails := defaultTails(logsDir, stateDir)
	fbStackdrivers := defaultStackdriverOutputs()
	fbSyslogs := []*conf.Syslog{}
	fbWinEventlogs := []*conf.WindowsEventlog{}
	fbFilterParsers := []*conf.FilterParser{}
	fbFilterAddLogNames := []*conf.FilterModifyAddLogName{}
	fbFilterRewriteTags := []*conf.FilterRewriteTag{}
	fbFilterRemoveLogNames := []*conf.FilterModifyRemoveLogName{}

	if logging.Service != nil {
		fileReceiverFactories, syslogReceiverFactories, wineventlogReceiverFactories, err := extractReceiverFactories(logging.Receivers)
		if err != nil {
			return "", "", err
		}
		extractedTails := []*conf.Tail{}
		extractedTails, fbSyslogs, fbWinEventlogs, err = generateFluentBitInputs(fileReceiverFactories, syslogReceiverFactories, wineventlogReceiverFactories, logging.Service.Pipelines, stateDir)
		if err != nil {
			return "", "", err
		}
		fbTails = append(fbTails, extractedTails...)
		fbFilterParsers, err = generateFluentBitFilters(logging.Processors, logging.Service.Pipelines)
		if err != nil {
			return "", "", err
		}
		extractedStackdrivers := []*conf.Stackdriver{}
		fbFilterAddLogNames, fbFilterRewriteTags, fbFilterRemoveLogNames, extractedStackdrivers, err = extractExporterPlugins(logging.Exporters, logging.Service.Pipelines)
		if err != nil {
			return "", "", err
		}
		fbStackdrivers = append(fbStackdrivers, extractedStackdrivers...)
	}
	mainConfig, err := conf.GenerateFluentBitMainConfig(fbTails, fbSyslogs, fbWinEventlogs, fbFilterParsers, fbFilterAddLogNames, fbFilterRewriteTags, fbFilterRemoveLogNames, fbStackdrivers)
	if err != nil {
		return "", "", err
	}
	jsonParsers, regexParsers, err := extractFluentBitParsers(logging.Processors)
	if err != nil {
		return "", "", err
	}
	parserConfig, err := conf.GenerateFluentBitParserConfig(jsonParsers, regexParsers)
	if err != nil {
		return "", "", err
	}
	return mainConfig, parserConfig, nil
}

type syslogReceiverFactory struct {
	TransportProtocol string
	ListenHost        string
	ListenPort        uint16
}

type fileReceiverFactory struct {
	IncludePaths []string
	ExcludePaths []string
}

type wineventlogReceiverFactory struct {
	Channels []string
}

type hostmetricsReceiverFactory struct{
	CollectionInterval string
}

func extractOtelReceiverFactories(receivers map[string]*otelReceiver) (map[string]*hostmetricsReceiverFactory, error){
	hostmetricsReceiverFactories := map[string]*hostmetricsReceiverFactory{}
	for n, r := range receivers {
		switch r.Type {
		case "hostmetrics":
			hostmetricsReceiverFactories[n] = &hostmetricsReceiverFactory{
				CollectionInterval: r.CollectionInterval,
			}
		default:
			return nil, fmt.Errorf(`receiver %q should have type as one of the "hostmetrics"`, n)
		}
	}
	return hostmetricsReceiverFactories, nil
}

func extractReceiverFactories(receivers map[string]*receiver) (map[string]*fileReceiverFactory, map[string]*syslogReceiverFactory, map[string]*wineventlogReceiverFactory, error) {
	fileReceiverFactories := map[string]*fileReceiverFactory{}
	syslogReceiverFactories := map[string]*syslogReceiverFactory{}
	wineventlogReceiverFactories := map[string]*wineventlogReceiverFactory{}
	for n, r := range receivers {
		if strings.HasPrefix(n, "lib:") {
			return nil, nil, nil, fmt.Errorf(`receiver id prefix 'lib:' is reserved for pre-defined receivers. Receiver ID %q is not allowed.`, n)
		}
		switch r.Type {
		case "files":
			if r.TransportProtocol != "" {
				return nil, nil, nil, fmt.Errorf(`files type receiver %q should not have field "transport_protocol"`, n)
			}
			if r.ListenHost != "" {
				return nil, nil, nil, fmt.Errorf(`files type receiver %q should not have field "listen_host"`, n)
			}
			if r.ListenPort != 0 {
				return nil, nil, nil, fmt.Errorf(`files type receiver %q should not have field "listen_port"`, n)
			}
			if r.Channels != nil {
				return nil, nil, nil, fmt.Errorf(`files type receiver %q should not have field "channels"`, n)
			}
			fileReceiverFactories[n] = &fileReceiverFactory{
				IncludePaths: r.IncludePaths,
				ExcludePaths: r.ExcludePaths,
			}
		case "syslog":
			if r.IncludePaths != nil {
				return nil, nil, nil, fmt.Errorf(`syslog type receiver %q should not have field "include_paths"`, n)
			}
			if r.ExcludePaths != nil {
				return nil, nil, nil, fmt.Errorf(`syslog type receiver %q should not have field "exclude_paths"`, n)
			}
			if r.TransportProtocol != "tcp" && r.TransportProtocol != "udp" {
				return nil, nil, nil, fmt.Errorf(`syslog type receiver %q should have the mode as one of the "tcp", "udp"`, n)
			}
			if r.Channels != nil {
				return nil, nil, nil, fmt.Errorf(`syslog type receiver %q should not have field "channels"`, n)
			}
			syslogReceiverFactories[n] = &syslogReceiverFactory{
				TransportProtocol: r.TransportProtocol,
				ListenHost:        r.ListenHost,
				ListenPort:        r.ListenPort,
			}
		case "windows_event_log":
			if r.TransportProtocol != "" {
				return nil, nil, nil, fmt.Errorf(`windows_event_log type receiver %q should not have field "transport_protocol"`, n)
			}
			if r.ListenHost != "" {
				return nil, nil, nil, fmt.Errorf(`windows_event_log type receiver %q should not have field "listen_host"`, n)
			}
			if r.ListenPort != 0 {
				return nil, nil, nil, fmt.Errorf(`windows_event_log type receiver %q should not have field "listen_port"`, n)
			}
			if r.IncludePaths != nil {
				return nil, nil, nil, fmt.Errorf(`windows_event_log type receiver %q should not have field "include_paths"`, n)
			}
			if r.ExcludePaths != nil {
				return nil, nil, nil, fmt.Errorf(`windows_event_log type receiver %q should not have field "exclude_paths"`, n)
			}
			wineventlogReceiverFactories[n] = &wineventlogReceiverFactory{
				Channels: r.Channels,
			}
		default:
			return nil, nil, nil, fmt.Errorf(`receiver %q should have type as one of the "files", "syslog"`, n)
		}
	}
	return fileReceiverFactories, syslogReceiverFactories, wineventlogReceiverFactories, nil
}

func generateOtelReceivers(hostmetricsReceiverFactories map[string]*hostmetricsReceiverFactory, pipelines map[string]*otelPipeline) ([]*otel.HostMetrics, map[string]string, error) {
	hostMetricsList := []*otel.HostMetrics{}
	receiveNameMap := make(map[string]string)
	var pipelineIDs []string
	for p := range pipelines {
		pipelineIDs = append(pipelineIDs, p)
	}
	sort.Strings(pipelineIDs)
	for _, pID := range pipelineIDs {
		p := pipelines[pID]
		for _, rID := range p.Receivers {
			if _, ok := hostmetricsReceiverFactories[rID]; ok {
				hostMetrics := otel.HostMetrics{
					HostMetricsID: rID,
					CollectionInterval: "1",
				}
				hostMetricsList = append(hostMetricsList, &hostMetrics)
				receiveNameMap[rID] = "hostmetrics/"+rID
				continue
			}
			return nil, nil, fmt.Errorf(`receiver %q of pipeline %q is not defined`, rID, pID)
		}
	}
	return hostMetricsList, receiveNameMap, nil
}

func generateOtelExporters(exporters map[string]*otelExporter, pipelines map[string]*otelPipeline) ([]*otel.Stackdriver, map[string]string, error) {
	stackdriverList := []*otel.Stackdriver{}
	exportNameMap := make(map[string]string)
	var pipelineIDs []string
	for p := range pipelines {
		pipelineIDs = append(pipelineIDs, p)
	}
	sort.Strings(pipelineIDs)
	for _, pID := range pipelineIDs {
		p := pipelines[pID]
		for _, eID := range p.Exporters {
			if _, ok := exporters[eID]; ok {
				stackdriver := otel.Stackdriver{
					StackdriverID: eID,
					UserAgent: "$USERAGENT",
					Prefix: "agent.googleapis.com/",
				}
				stackdriverList = append(stackdriverList, &stackdriver)
				exportNameMap[eID] = "stackdriver/" + eID
				continue
			}
			return nil, nil, fmt.Errorf(`exporter %q of pipeline %q is not defined`, eID, pID)
		}
	}
	return stackdriverList, exportNameMap, nil
}

func generateFluentBitInputs(fileReceiverFactories map[string]*fileReceiverFactory, syslogReceiverFactories map[string]*syslogReceiverFactory, wineventlogReceiverFactories map[string]*wineventlogReceiverFactory, pipelines map[string]*loggingPipeline, stateDir string) ([]*conf.Tail, []*conf.Syslog, []*conf.WindowsEventlog, error) {
	fbTails := []*conf.Tail{}
	fbSyslogs := []*conf.Syslog{}
	fbWinEventlogs := []*conf.WindowsEventlog{}
	var pipelineIDs []string
	for p := range pipelines {
		pipelineIDs = append(pipelineIDs, p)
	}
	sort.Strings(pipelineIDs)
	for _, pID := range pipelineIDs {
		p := pipelines[pID]
		for _, rID := range p.Receivers {
			if f, ok := fileReceiverFactories[rID]; ok {
				fbTail := conf.Tail{
					Tag:  fmt.Sprintf("%s.%s", pID, rID),
					DB:   fmt.Sprintf("%s/buffers/%s_%s", stateDir, pID, rID),
					Path: strings.Join(f.IncludePaths, ","),
				}
				if len(f.ExcludePaths) != 0 {
					fbTail.ExcludePath = strings.Join(f.ExcludePaths, ",")
				}
				fbTails = append(fbTails, &fbTail)
				continue
			}
			if f, ok := syslogReceiverFactories[rID]; ok {
				fbSyslog := conf.Syslog{
					Tag:    fmt.Sprintf("%s.%s", pID, rID),
					Listen: f.ListenHost,
					Mode:   f.TransportProtocol,
					Port:   f.ListenPort,
				}
				fbSyslogs = append(fbSyslogs, &fbSyslog)
				continue
			}
			if f, ok := wineventlogReceiverFactories[rID]; ok {
				fbWinlog := conf.WindowsEventlog{
					Tag:          fmt.Sprintf("%s.%s", pID, rID),
					Channels:     strings.Join(f.Channels, ","),
					Interval_Sec: "1",
					DB:           fmt.Sprintf("%s/buffers/%s_%s", stateDir, pID, rID),
				}
				fbWinEventlogs = append(fbWinEventlogs, &fbWinlog)
				continue
			}
			return nil, nil, nil, fmt.Errorf(`receiver %q of pipeline %q is not defined`, rID, pID)
		}
	}
	return fbTails, fbSyslogs, fbWinEventlogs, nil
}

func generateFluentBitFilters(processors map[string]*processor, pipelines map[string]*loggingPipeline) ([]*conf.FilterParser, error) {
	fbFilterParsers := []*conf.FilterParser{}
	var pipelineIDs []string
	for p := range pipelines {
		pipelineIDs = append(pipelineIDs, p)
	}
	sort.Strings(pipelineIDs)
	for _, pipelineID := range pipelineIDs {
		pipeline := pipelines[pipelineID]
		for _, processorID := range pipeline.Processors {
			p, ok := processors[processorID]
			if !isDefaultProcessor(processorID) && !ok {
				return nil, fmt.Errorf(`logging processor not defined: %q`, processorID)
			}
			fbFilterParser := conf.FilterParser{
				Match:   fmt.Sprintf("%s.*", pipelineID),
				Parser:  processorID,
				KeyName: "message",
			}
			if ok && p.Field != "" {
				fbFilterParser.KeyName = p.Field
			}
			fbFilterParsers = append(fbFilterParsers, &fbFilterParser)
		}
	}
	return fbFilterParsers, nil
}

func isDefaultProcessor(name string) bool {
	switch name {
	case "lib:apache", "lib:apache2", "lib:apache_error", "lib:mongodb", "lib:nginx",
		"lib:syslog-rfc3164", "lib:syslog-rfc5424":
		return true
	default:
		return false
	}
}

func extractExporterPlugins(exporters map[string]*exporter, pipelines map[string]*loggingPipeline) (
	[]*conf.FilterModifyAddLogName, []*conf.FilterRewriteTag, []*conf.FilterModifyRemoveLogName, []*conf.Stackdriver, error) {
	fbFilterModifyAddLogNames := []*conf.FilterModifyAddLogName{}
	fbFilterRewriteTags := []*conf.FilterRewriteTag{}
	fbFilterModifyRemoveLogNames := []*conf.FilterModifyRemoveLogName{}
	fbStackdrivers := []*conf.Stackdriver{}
	var pipelineIDs []string
	for p := range pipelines {
		if strings.HasPrefix(p, "lib:") {
			return nil, nil, nil, nil, fmt.Errorf(`pipeline id prefix 'lib:' is reserved for pre-defined pipelines. Pipeline ID %q is not allowed.`, p)
		}
		pipelineIDs = append(pipelineIDs, p)
	}
	sort.Strings(pipelineIDs)
	for _, pipelineID := range pipelineIDs {
		pipeline := pipelines[pipelineID]
		for _, exporterID := range pipeline.Exporters {
			if strings.HasPrefix(exporterID, "lib:") {
				return nil, nil, nil, nil, fmt.Errorf(`exporter id prefix 'lib:' is reserved for pre-defined exporters. Exporter ID %q is not allowed.`, exporterID)
			}
			// if exporterID is google or we can find this ID is a google_cloud_logging type from the Stackdriver Exporter map
			if e, ok := exporters[exporterID]; !(ok && e.Type == "google_cloud_logging") {
				return nil, nil, nil, nil,
					fmt.Errorf(`pipeline %q cannot have an exporter %q which is not "google_cloud_logging" type`, pipelineID, exporterID)
			}
			// for each receiver, generate a output plugin with the specified receiver id
			for _, rID := range pipeline.Receivers {
				fbFilterModifyAddLogNames = append(fbFilterModifyAddLogNames, &conf.FilterModifyAddLogName{
					Match:   fmt.Sprintf("%s.%s", pipelineID, rID),
					LogName: rID,
				})
				// generate single rewriteTag for this pipeline
				fbFilterRewriteTags = append(fbFilterRewriteTags, &conf.FilterRewriteTag{
					Match: fmt.Sprintf("%s.%s", pipelineID, rID),
				})
				fbFilterModifyRemoveLogNames = append(fbFilterModifyRemoveLogNames, &conf.FilterModifyRemoveLogName{
					Match: rID,
				})
				fbStackdrivers = append(fbStackdrivers, &conf.Stackdriver{
					Match: rID,
				})
			}
		}
	}
	return fbFilterModifyAddLogNames, fbFilterRewriteTags, fbFilterModifyRemoveLogNames, fbStackdrivers, nil
}

func extractFluentBitParsers(processors map[string]*processor) ([]*conf.ParserJSON, []*conf.ParserRegex, error) {
	var names []string
	for n := range processors {
		if strings.HasPrefix(n, "lib:") {
			return nil, nil, fmt.Errorf(`process id prefix 'lib:' is reserved for pre-defined processors. Processor ID %q is not allowed.`, n)
		}
		names = append(names, n)
	}
	sort.Strings(names)

	fbJSONParsers := []*conf.ParserJSON{}
	fbRegexParsers := []*conf.ParserRegex{}
	for _, name := range names {
		p := processors[name]
		switch t := p.Type; t {
		case "parse_json":
			fbJSONParser := conf.ParserJSON{
				Name:       name,
				TimeKey:    p.TimeKey,
				TimeFormat: p.TimeFormat,
			}
			fbJSONParsers = append(fbJSONParsers, &fbJSONParser)
		case "parse_regex":
			fbRegexParser := conf.ParserRegex{
				Name:       name,
				Regex:      p.Regex,
				TimeKey:    p.TimeKey,
				TimeFormat: p.TimeFormat,
			}
			fbRegexParsers = append(fbRegexParsers, &fbRegexParser)
		default:
			return nil, nil, fmt.Errorf(`processor %q should be one of the type \"parse_json\", \"parse_regex\"`, name)
		}
	}
	return fbJSONParsers, fbRegexParsers, nil
}

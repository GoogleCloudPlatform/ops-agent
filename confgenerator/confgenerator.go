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
	"net"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/config"
	"github.com/GoogleCloudPlatform/ops-agent/fluentbit/conf"
	"github.com/GoogleCloudPlatform/ops-agent/internal/version"
	"github.com/GoogleCloudPlatform/ops-agent/otel"
	"github.com/shirou/gopsutil/host"
)

// filepathJoin uses the real filepath.Join in actual executable
// but can be overriden in tests to impersonate an alternate OS.
var filepathJoin = defaultFilepathJoin

func defaultFilepathJoin(_ string, elem ...string) string {
	return filepath.Join(elem...)
}

func generateOtelConfig(metrics *config.Metrics, hostInfo *host.InfoStat) (string, error) {
	userAgent, _ := getUserAgent("Google-Cloud-Ops-Agent-Metrics", hostInfo)
	versionLabel, _ := getVersionLabel("google-cloud-ops-agent-metrics")
	hostMetricsList := []*otel.HostMetrics{}
	mssqlList := []*otel.MSSQL{}
	iisList := []*otel.IIS{}
	stackdriverList := []*otel.Stackdriver{}
	serviceList := []*otel.Service{}
	receiverNameMap := make(map[string]string)
	exporterNameMap := make(map[string]string)
	if metrics != nil {
		var err error
		hostMetricsList, mssqlList, iisList, receiverNameMap, err = generateOtelReceivers(metrics.Receivers, metrics.Service.Pipelines)
		if err != nil {
			return "", err
		}
		stackdriverList, exporterNameMap, err = generateOtelExporters(metrics.Exporters, metrics.Service.Pipelines)
		if err != nil {
			return "", err
		}
		serviceList, err = generateOtelServices(receiverNameMap, exporterNameMap, metrics.Service.Pipelines)
		if err != nil {
			return "", err
		}
	}
	otelConfig, err := otel.Config{
		HostMetrics: hostMetricsList,
		MSSQL:       mssqlList,
		IIS:         iisList,
		Stackdriver: stackdriverList,
		Service:     serviceList,

		UserAgent: userAgent,
		Version:   versionLabel,
		Windows:   hostInfo.OS == "windows",
	}.Generate()
	if err != nil {
		return "", err
	}
	return otelConfig, nil
}

func generateOtelServices(receiverNameMap map[string]string, exporterNameMap map[string]string, pipelines map[string]*config.MetricsPipeline) ([]*otel.Service, error) {
	serviceList := []*otel.Service{}
	for _, pID := range config.SortedKeys(pipelines) {
		p := pipelines[pID]
		for _, rID := range p.ReceiverIDs {
			var pipelineID string
			var defaultProcessors []string
			if strings.HasPrefix(receiverNameMap[rID], "hostmetrics/") {
				defaultProcessors = []string{"agentmetrics/system", "filter/system", "metricstransform/system", "resourcedetection"}
				pipelineID = "system"
			} else if strings.HasPrefix(receiverNameMap[rID], "windowsperfcounters/mssql") {
				defaultProcessors = []string{"metricstransform/mssql", "resourcedetection"}
				pipelineID = "mssql"
			} else if strings.HasPrefix(receiverNameMap[rID], "windowsperfcounters/iis") {
				defaultProcessors = []string{"metricstransform/iis", "resourcedetection"}
				pipelineID = "iis"
			}
			var pExportIDs []string
			for _, eID := range p.ExporterIDs {
				pExportIDs = append(pExportIDs, exporterNameMap[eID])
			}
			service := otel.Service{
				ID:         pipelineID,
				Receivers:  fmt.Sprintf("[%s]", receiverNameMap[rID]),
				Processors: fmt.Sprintf("[%s]", strings.Join(defaultProcessors, ",")),
				Exporters:  fmt.Sprintf("[%s]", strings.Join(pExportIDs, ",")),
			}
			serviceList = append(serviceList, &service)
		}
	}
	return serviceList, nil
}

// defaultTails returns the default Tail sections for the agents' own logs.
func defaultTails(logsDir string, stateDir string, hostInfo *host.InfoStat) (tails []*conf.Tail) {
	tails = []*conf.Tail{}
	tailFluentbit := conf.Tail{
		Tag:  "ops-agent-fluent-bit",
		DB:   filepathJoin(hostInfo.OS, stateDir, "buffers", "ops-agent-fluent-bit"),
		Path: filepathJoin(hostInfo.OS, logsDir, "logging-module.log"),
	}
	tailCollectd := conf.Tail{
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
func defaultStackdriverOutputs(hostInfo *host.InfoStat) (stackdrivers []*conf.Stackdriver) {
	return []*conf.Stackdriver{
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
		return 1
	}
}

// generateFluentBitConfigs generates FluentBit configuration from unified agents configuration
// in yaml. generateFluentBitConfigs returns empty configurations without an error if `logging`
// does not exist as a top-level field in the input yaml format.
func generateFluentBitConfigs(logging *config.Logging, logsDir string, stateDir string, hostInfo *host.InfoStat) (string, string, error) {
	fbTails := defaultTails(logsDir, stateDir, hostInfo)
	userAgent, _ := getUserAgent("Google-Cloud-Ops-Agent-Logging", hostInfo)
	fbStackdrivers := defaultStackdriverOutputs(hostInfo)
	fbSyslogs := []*conf.Syslog{}
	fbWinEventlogs := []*conf.WindowsEventlog{}
	fbFilterParsers := []*conf.FilterParser{}
	fbFilterAddLogNames := []*conf.FilterModifyAddLogName{}
	fbFilterRewriteTags := []*conf.FilterRewriteTag{}
	fbFilterRemoveLogNames := []*conf.FilterModifyRemoveLogName{}
	jsonParsers := []*conf.ParserJSON{}
	regexParsers := []*conf.ParserRegex{}

	if logging != nil && logging.Service != nil {
		extractedTails := []*conf.Tail{}
		var err error
		extractedTails, fbSyslogs, fbWinEventlogs, err = generateFluentBitInputs(logging.Receivers, logging.Service.Pipelines, stateDir, hostInfo)
		if err != nil {
			return "", "", err
		}
		fbTails = append(fbTails, extractedTails...)
		fbFilterParsers, err = generateFluentBitFilters(logging.Processors, logging.Service.Pipelines)
		if err != nil {
			return "", "", err
		}
		extractedStackdrivers := []*conf.Stackdriver{}
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
	mainConfig, err := conf.GenerateFluentBitMainConfig(fbTails, fbSyslogs, fbWinEventlogs, fbFilterParsers, fbFilterAddLogNames, fbFilterRewriteTags, fbFilterRemoveLogNames, fbStackdrivers, userAgent)
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

type hostmetricsReceiverFactory struct {
	CollectionInterval string
}

type mssqlReceiverFactory struct {
	CollectionInterval string
}

type iisReceiverFactory struct {
	CollectionInterval string
}

func extractOtelReceiverFactories(receivers map[string]*config.MetricsReceiver) (map[string]*hostmetricsReceiverFactory, map[string]*mssqlReceiverFactory, map[string]*iisReceiverFactory, error) {
	hostmetricsReceiverFactories := map[string]*hostmetricsReceiverFactory{}
	mssqlReceiverFactories := map[string]*mssqlReceiverFactory{}
	iisReceiverFactories := map[string]*iisReceiverFactory{}
	for n, r := range receivers {
		switch r.Type {
		case "hostmetrics":
			hostmetricsReceiverFactories[n] = &hostmetricsReceiverFactory{
				CollectionInterval: r.CollectionInterval,
			}
		case "mssql":
			mssqlReceiverFactories[n] = &mssqlReceiverFactory{
				CollectionInterval: r.CollectionInterval,
			}
		case "iis":
			iisReceiverFactories[n] = &iisReceiverFactory{
				CollectionInterval: r.CollectionInterval,
			}
		}
	}
	return hostmetricsReceiverFactories, mssqlReceiverFactories, iisReceiverFactories, nil
}

func extractReceiverFactories(receivers map[string]*config.LoggingReceiver) (map[string]*fileReceiverFactory, map[string]*syslogReceiverFactory, map[string]*wineventlogReceiverFactory, error) {
	fileReceiverFactories := map[string]*fileReceiverFactory{}
	syslogReceiverFactories := map[string]*syslogReceiverFactory{}
	wineventlogReceiverFactories := map[string]*wineventlogReceiverFactory{}
	for rID, r := range receivers {
		switch r.Type {
		case "files":
			fileReceiverFactories[rID] = &fileReceiverFactory{
				IncludePaths: r.IncludePaths,
				ExcludePaths: r.ExcludePaths,
			}
		case "syslog":
			if r.TransportProtocol != "tcp" && r.TransportProtocol != "udp" {
				return nil, nil, nil, fmt.Errorf(`unknown transport_protocol %q in the logging receiver %q. Supported transport_protocol for %q type logging receiver: [tcp, udp].`, r.TransportProtocol, rID, r.Type)
			}
			if net.ParseIP(r.ListenHost) == nil {
				return nil, nil, nil, fmt.Errorf(`unknown listen_host %q in the logging receiver %q. Value of listen_host for %q type logging receiver should be a valid IP.`, r.ListenHost, rID, r.Type)
			}
			syslogReceiverFactories[rID] = &syslogReceiverFactory{
				TransportProtocol: r.TransportProtocol,
				ListenHost:        r.ListenHost,
				ListenPort:        r.ListenPort,
			}
		case "windows_event_log":
			wineventlogReceiverFactories[rID] = &wineventlogReceiverFactory{
				Channels: r.Channels,
			}
		}
	}
	return fileReceiverFactories, syslogReceiverFactories, wineventlogReceiverFactories, nil
}

func generateOtelReceivers(receivers map[string]*config.MetricsReceiver, pipelines map[string]*config.MetricsPipeline) ([]*otel.HostMetrics, []*otel.MSSQL, []*otel.IIS, map[string]string, error) {
	hostMetricsList := []*otel.HostMetrics{}
	mssqlList := []*otel.MSSQL{}
	iisList := []*otel.IIS{}
	receiverNameMap := make(map[string]string)
	hostmetricsReceiverFactories, mssqlReceiverFactories, iisReceiverFactories, err := extractOtelReceiverFactories(receivers)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	for _, pID := range config.SortedKeys(pipelines) {
		p := pipelines[pID]
		for _, rID := range p.ReceiverIDs {
			if _, ok := receiverNameMap[rID]; ok {
				continue
			}
			if h, ok := hostmetricsReceiverFactories[rID]; ok {
				hostMetrics := otel.HostMetrics{
					HostMetricsID:      rID,
					CollectionInterval: h.CollectionInterval,
				}
				hostMetricsList = append(hostMetricsList, &hostMetrics)
				receiverNameMap[rID] = "hostmetrics/" + rID
			} else if m, ok := mssqlReceiverFactories[rID]; ok {
				mssql := otel.MSSQL{
					MSSQLID:            rID,
					CollectionInterval: m.CollectionInterval,
				}
				mssqlList = append(mssqlList, &mssql)
				receiverNameMap[rID] = "windowsperfcounters/mssql_" + rID
			} else if i, ok := iisReceiverFactories[rID]; ok {
				iis := otel.IIS{
					IISID:              rID,
					CollectionInterval: i.CollectionInterval,
				}
				iisList = append(iisList, &iis)
				receiverNameMap[rID] = "windowsperfcounters/iis_" + rID
			}
		}
	}
	if len(hostMetricsList) > 1 {
		return nil, nil, nil, nil, fmt.Errorf(`at most one metrics receiver with type "hostmetrics" is allowed.`)
	}
	if len(mssqlList) > 1 {
		return nil, nil, nil, nil, fmt.Errorf(`at most one metrics receiver with type "mssql" is allowed.`)
	}
	if len(iisList) > 1 {
		return nil, nil, nil, nil, fmt.Errorf(`at most one metrics receiver with type "iis" is allowed.`)
	}
	return hostMetricsList, mssqlList, iisList, receiverNameMap, nil
}

func generateOtelExporters(exporters map[string]*config.MetricsExporter, pipelines map[string]*config.MetricsPipeline) ([]*otel.Stackdriver, map[string]string, error) {
	stackdriverList := []*otel.Stackdriver{}
	exportNameMap := make(map[string]string)
	for _, pID := range config.SortedKeys(pipelines) {
		p := pipelines[pID]
		for _, eID := range p.ExporterIDs {
			exporter, ok := exporters[eID]
			if !ok {
				continue
			}
			switch exporter.Type {
			case "google_cloud_monitoring":
				if _, ok := exportNameMap[eID]; !ok {
					stackdriver := otel.Stackdriver{
						StackdriverID: eID,
						Prefix:        "agent.googleapis.com/",
					}
					stackdriverList = append(stackdriverList, &stackdriver)
					exportNameMap[eID] = "googlecloud/" + eID
				}
			}
		}
	}
	if len(stackdriverList) > 1 {
		return nil, nil, fmt.Errorf(`Only one exporter of the same type in [google_cloud_monitoring] is allowed.`)
	}
	return stackdriverList, exportNameMap, nil
}

func generateFluentBitInputs(receivers map[string]*config.LoggingReceiver, pipelines map[string]*config.LoggingPipeline, stateDir string, hostInfo *host.InfoStat) ([]*conf.Tail, []*conf.Syslog, []*conf.WindowsEventlog, error) {
	fbTails := []*conf.Tail{}
	fbSyslogs := []*conf.Syslog{}
	fbWinEventlogs := []*conf.WindowsEventlog{}
	fileReceiverFactories, syslogReceiverFactories, wineventlogReceiverFactories, err := extractReceiverFactories(receivers)
	if err != nil {
		return nil, nil, nil, err
	}
	for _, pID := range config.SortedKeys(pipelines) {
		p := pipelines[pID]
		for _, rID := range p.Receivers {
			if f, ok := fileReceiverFactories[rID]; ok {
				fbTail := conf.Tail{
					Tag:  fmt.Sprintf("%s.%s", pID, rID),
					DB:   filepathJoin(hostInfo.OS, stateDir, "buffers", pID+"_"+rID),
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
					DB:           filepathJoin(hostInfo.OS, stateDir, "buffers", pID+"_"+rID),
				}
				fbWinEventlogs = append(fbWinEventlogs, &fbWinlog)
				continue
			}
		}
	}
	return fbTails, fbSyslogs, fbWinEventlogs, nil
}

func generateFluentBitFilters(processors map[string]*config.LoggingProcessor, pipelines map[string]*config.LoggingPipeline) ([]*conf.FilterParser, error) {
	fbFilterParsers := []*conf.FilterParser{}
	for _, pID := range config.SortedKeys(pipelines) {
		pipeline := pipelines[pID]
		for _, processorID := range pipeline.Processors {
			p, ok := processors[processorID]
			fbFilterParser := conf.FilterParser{
				Match:   fmt.Sprintf("%s.*", pID),
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

func extractExporterPlugins(exporters map[string]*config.LoggingExporter, pipelines map[string]*config.LoggingPipeline, hostInfo *host.InfoStat) (
	[]*conf.FilterModifyAddLogName, []*conf.FilterRewriteTag, []*conf.FilterModifyRemoveLogName, []*conf.Stackdriver, error) {
	fbFilterModifyAddLogNames := []*conf.FilterModifyAddLogName{}
	fbFilterRewriteTags := []*conf.FilterRewriteTag{}
	fbFilterModifyRemoveLogNames := []*conf.FilterModifyRemoveLogName{}
	fbStackdrivers := []*conf.Stackdriver{}
	stackdriverExporters := make(map[string][]string)
	for _, pID := range config.SortedKeys(pipelines) {
		pipeline := pipelines[pID]
		for _, exporterID := range pipeline.Exporters {
			// for each receiver, generate a output plugin with the specified receiver id
			for _, rID := range pipeline.Receivers {
				fbFilterModifyAddLogNames = append(fbFilterModifyAddLogNames, &conf.FilterModifyAddLogName{
					Match:   fmt.Sprintf("%s.%s", pID, rID),
					LogName: rID,
				})
				// generate single rewriteTag for this pipeline
				fbFilterRewriteTags = append(fbFilterRewriteTags, &conf.FilterRewriteTag{
					Match: fmt.Sprintf("%s.%s", pID, rID),
				})
				fbFilterModifyRemoveLogNames = append(fbFilterModifyRemoveLogNames, &conf.FilterModifyRemoveLogName{
					Match: rID,
				})
				stackdriverExporters[exporterID] = append(stackdriverExporters[exporterID], rID)
			}
		}
	}
	for _, tags := range stackdriverExporters {
		fbStackdrivers = append(fbStackdrivers, &conf.Stackdriver{
			Match:   strings.Join(tags, "|"),
			Workers: getWorkers(hostInfo),
		})
	}
	return fbFilterModifyAddLogNames, fbFilterRewriteTags, fbFilterModifyRemoveLogNames, fbStackdrivers, nil
}

func extractFluentBitParsers(processors map[string]*config.LoggingProcessor) ([]*conf.ParserJSON, []*conf.ParserRegex, error) {
	fbJSONParsers := []*conf.ParserJSON{}
	fbRegexParsers := []*conf.ParserRegex{}
	for _, name := range config.SortedKeys(processors) {
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
		}
	}
	return fbJSONParsers, fbRegexParsers, nil
}

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
	"sort"
	"strings"
	"text/template"

	"github.com/GoogleCloudPlatform/ops-agent/collectd"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/config"
	"github.com/GoogleCloudPlatform/ops-agent/fluentbit/conf"
	"github.com/GoogleCloudPlatform/ops-agent/internal/version"
	"github.com/GoogleCloudPlatform/ops-agent/otel"
	"github.com/shirou/gopsutil/host"

	yaml "gopkg.in/yaml.v2"
)

var (
	// Supported component types.
	supportedComponentTypes = map[string][]string{
		"linux_logging_receiver":    []string{"files", "syslog"},
		"linux_logging_processor":   []string{"parse_json", "parse_regex"},
		"linux_logging_exporter":    []string{"google_cloud_logging"},
		"linux_metrics_receiver":    []string{"hostmetrics"},
		"linux_metrics_exporter":    []string{"google_cloud_monitoring"},
		"windows_logging_receiver":  []string{"files", "syslog", "windows_event_log"},
		"windows_logging_processor": []string{"parse_json", "parse_regex"},
		"windows_logging_exporter":  []string{"google_cloud_logging"},
		"windows_metrics_receiver":  []string{"hostmetrics", "iis", "mssql"},
		"windows_metrics_exporter":  []string{"google_cloud_monitoring"},
	}

	// Supported parameters.
	supportedParameters = map[string][]string{
		"files":             []string{"include_paths", "exclude_paths"},
		"syslog":            []string{"transport_protocol", "listen_host", "listen_port"},
		"windows_event_log": []string{"channels"},
		"parse_json":        []string{"field", "time_key", "time_format"},
		"parse_regex":       []string{"field", "time_key", "time_format", "regex"},
		"hostmetrics":       []string{"collection_interval"},
	}
)

// filepathJoin uses the real filepath.Join in actual executable
// but can be overriden in tests to impersonate an alternate OS.
var filepathJoin = defaultFilepathJoin

func defaultFilepathJoin(_ string, elem ...string) string {
	return filepath.Join(elem...)
}

type UnifiedConfig struct {
	Logging *config.Logging `yaml:"logging"`
	Metrics *config.Metrics `yaml:"metrics"`
}

func (uc *UnifiedConfig) HasLogging() bool {
	return uc.Logging != nil
}

func (uc *UnifiedConfig) HasMetrics() bool {
	return uc.Metrics != nil
}

func (uc *UnifiedConfig) GenerateOtelConfig(hostInfo *host.InfoStat) (config string, err error) {
	return generateOtelConfig(uc.Metrics, hostInfo)
}

func (uc *UnifiedConfig) GenerateCollectdConfig(logsDir string) (config string, err error) {
	return collectd.GenerateCollectdConfig(uc.Metrics, logsDir)
}

// GenerateFluentBitConfigs generates FluentBit configuration from unified agents configuration
// in yaml. GenerateFluentBitConfigs returns empty configurations without an error if `logs`
// does not exist as a top-level field in the input yaml format.
func (uc *UnifiedConfig) GenerateFluentBitConfigs(logsDir string, stateDir string, hostInfo *host.InfoStat) (mainConfig string, parserConfig string, err error) {
	return generateFluentBitConfigs(uc.Logging, logsDir, stateDir, hostInfo)
}

func ParseUnifiedConfig(input []byte) (UnifiedConfig, error) {
	config := UnifiedConfig{}
	err := yaml.UnmarshalStrict(input, &config)
	if err != nil {
		return UnifiedConfig{}, fmt.Errorf("the agent config file is not valid YAML. detailed error: %s", err)
	}
	return config, nil
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
	if err := config.ValidateComponentIds(pipelines, "metrics", "pipeline"); err != nil {
		return nil, err
	}
	var pipelineIDs []string
	for p := range pipelines {
		pipelineIDs = append(pipelineIDs, p)
	}
	sort.Strings(pipelineIDs)
	for _, pID := range pipelineIDs {
		p := pipelines[pID]
		for _, rID := range p.ReceiverIDs {
			// TODO: Fix the platform. It should be "windows" for Windows and "linux" for Linux.
			// TODO: replace receiverNameMap[rID] with the actual receiver type.
			cid := componentID{platform: "windows", subagent: "metrics", component: "receiver", componentType: receiverNameMap[rID], id: rID}
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
			} else {
				return nil, unsupportedComponentTypeError(cid)
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
		// TODO: Fix the platform. It should be "windows" for Windows and "linux" for Linux.
		cid := componentID{subagent: "metrics", component: "receiver", componentType: r.Type, id: n, platform: "windows"}
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
		default:
			return nil, nil, nil, unsupportedComponentTypeError(cid)
		}
	}
	return hostmetricsReceiverFactories, mssqlReceiverFactories, iisReceiverFactories, nil
}

type componentID struct {
	// subagent should be "logging", or "metrics".
	subagent string
	// component should be "receiver", "processor", or "exporter".
	component string
	// id is the id of the component.
	id string
	// componentType is the type of the receiver, processor, or exporter, e.g., "hostmetrics".
	componentType string
	// platform should be "linux" or "windows".
	platform string
}

// unsupportedComponentTypeError returns an error message when users specify a component type that is not supported.
// id is the id of the receiver, processor, or exporter.
func unsupportedComponentTypeError(id componentID) error {
	// e.g. metrics receiver "receiver_1" with type "unsupported_type" is not supported. Supported metrics receiver types: [hostmetrics, iis, mssql].
	return fmt.Errorf(`%s %s %q with type %q is not supported. Supported %s %s types: [%s].`,
		id.subagent, id.component, id.id, id.componentType, id.subagent, id.component, strings.Join(supportedComponentTypes[id.platform+"_"+id.subagent+"_"+id.component], ", "))
}

// missingRequiredParameterError returns an error message when users miss a required parameter.
// id is the id of the receiver, processor, or exporter.
// componentType is the type of the receiver, processor, or exporter, e.g., "hostmetrics".
// parameter is name of the parameter that is missing.
func missingRequiredParameterError(id componentID, parameter string) error {
	// e.g. parameter "include_paths" is required in logging receiver "receiver_1" because its type is "files".
	return fmt.Errorf(`parameter %q is required in %s %s %q because its type is %q.`, parameter, id.subagent, id.component, id.id, id.componentType)
}

// unsupportedParameterError returns an error message when users specifies an unsupported parameter.
// id is the id of the receiver, processor, or exporter.
// componentType is the type of the receiver, processor, or exporter, e.g., "hostmetrics".
// parameter is name of the parameter that is not supported.
func unsupportedParameterError(id componentID, parameter string) error {
	// e.g. parameter "transport_protocol" in logging receiver "receiver_1" is not supported. Supported parameters
	// for "files" type logging receiver: [include_paths, exclude_paths].
	return fmt.Errorf(`parameter %q in %s %s %q is not supported. Supported parameters for %q type %s %s: [%s].`,
		parameter, id.subagent, id.component, id.id, id.componentType, id.subagent, id.component, strings.Join(supportedParameters[id.componentType], ", "))
}

func extractReceiverFactories(receivers map[string]*config.LoggingReceiver) (map[string]*fileReceiverFactory, map[string]*syslogReceiverFactory, map[string]*wineventlogReceiverFactory, error) {
	fileReceiverFactories := map[string]*fileReceiverFactory{}
	syslogReceiverFactories := map[string]*syslogReceiverFactory{}
	wineventlogReceiverFactories := map[string]*wineventlogReceiverFactory{}
	if err := config.ValidateComponentIds(receivers, "logging", "receiver"); err != nil {
		return nil, nil, nil, err
	}
	for rID, r := range receivers {
		// TODO: Fix the platform. It should be "windows" for Windows and "linux" for Linux.
		cid := componentID{subagent: "logging", component: "receiver", componentType: r.Type, id: rID, platform: "windows"}
		switch r.Type {
		case "files":
			if r.TransportProtocol != "" {
				return nil, nil, nil, unsupportedParameterError(cid, "transport_protocol")
			}
			if r.ListenHost != "" {
				return nil, nil, nil, unsupportedParameterError(cid, "listen_host")
			}
			if r.ListenPort != 0 {
				return nil, nil, nil, unsupportedParameterError(cid, "listen_port")
			}
			if r.Channels != nil {
				return nil, nil, nil, unsupportedParameterError(cid, "channels")
			}
			if r.IncludePaths == nil {
				return nil, nil, nil, missingRequiredParameterError(cid, "include_paths")
			}
			fileReceiverFactories[rID] = &fileReceiverFactory{
				IncludePaths: r.IncludePaths,
				ExcludePaths: r.ExcludePaths,
			}
		case "syslog":
			if r.IncludePaths != nil {
				return nil, nil, nil, unsupportedParameterError(cid, "include_paths")
			}
			if r.ExcludePaths != nil {
				return nil, nil, nil, unsupportedParameterError(cid, "exclude_paths")
			}
			if r.TransportProtocol != "tcp" && r.TransportProtocol != "udp" {
				return nil, nil, nil, fmt.Errorf(`unknown transport_protocol %q in the logging receiver %q. Supported transport_protocol for %q type logging receiver: [tcp, udp].`, r.TransportProtocol, rID, r.Type)
			}
			if net.ParseIP(r.ListenHost) == nil {
				return nil, nil, nil, fmt.Errorf(`unknown listen_host %q in the logging receiver %q. Value of listen_host for %q type logging receiver should be a valid IP.`, r.ListenHost, rID, r.Type)
			}
			if r.ListenPort == 0 {
				return nil, nil, nil, missingRequiredParameterError(cid, "listen_port")
			}
			if r.Channels != nil {
				return nil, nil, nil, unsupportedParameterError(cid, "channels")
			}
			syslogReceiverFactories[rID] = &syslogReceiverFactory{
				TransportProtocol: r.TransportProtocol,
				ListenHost:        r.ListenHost,
				ListenPort:        r.ListenPort,
			}
		case "windows_event_log":
			if r.TransportProtocol != "" {
				return nil, nil, nil, unsupportedParameterError(cid, "transport_protocol")
			}
			if r.ListenHost != "" {
				return nil, nil, nil, unsupportedParameterError(cid, "listen_host")
			}
			if r.ListenPort != 0 {
				return nil, nil, nil, unsupportedParameterError(cid, "listen_port")
			}
			if r.IncludePaths != nil {
				return nil, nil, nil, unsupportedParameterError(cid, "include_paths")
			}
			if r.ExcludePaths != nil {
				return nil, nil, nil, unsupportedParameterError(cid, "exclude_paths")
			}
			if r.Channels == nil {
				return nil, nil, nil, missingRequiredParameterError(cid, "channels")
			}
			wineventlogReceiverFactories[rID] = &wineventlogReceiverFactory{
				Channels: r.Channels,
			}
		default:
			return nil, nil, nil, unsupportedComponentTypeError(cid)
		}
	}
	return fileReceiverFactories, syslogReceiverFactories, wineventlogReceiverFactories, nil
}

func generateOtelReceivers(receivers map[string]*config.MetricsReceiver, pipelines map[string]*config.MetricsPipeline) ([]*otel.HostMetrics, []*otel.MSSQL, []*otel.IIS, map[string]string, error) {
	hostMetricsList := []*otel.HostMetrics{}
	mssqlList := []*otel.MSSQL{}
	iisList := []*otel.IIS{}
	receiverNameMap := make(map[string]string)
	if err := config.ValidateComponentIds(pipelines, "metrics", "pipeline"); err != nil {
		return nil, nil, nil, nil, err
	}
	if err := config.ValidateComponentIds(receivers, "metrics", "receiver"); err != nil {
		return nil, nil, nil, nil, err
	}
	hostmetricsReceiverFactories, mssqlReceiverFactories, iisReceiverFactories, err := extractOtelReceiverFactories(receivers)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	var pipelineIDs []string
	for p := range pipelines {
		pipelineIDs = append(pipelineIDs, p)
	}
	sort.Strings(pipelineIDs)
	for _, pID := range pipelineIDs {
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
			} else {
				return nil, nil, nil, nil, fmt.Errorf(`metrics receiver %q from pipeline %q is not defined.`, rID, pID)
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
	if err := config.ValidateComponentIds(exporters, "metrics", "exporter"); err != nil {
		return nil, nil, err
	}
	var pipelineIDs []string
	for p := range pipelines {
		pipelineIDs = append(pipelineIDs, p)
	}
	sort.Strings(pipelineIDs)
	for _, pID := range pipelineIDs {
		p := pipelines[pID]
		for _, eID := range p.ExporterIDs {
			if _, ok := exporters[eID]; !ok {
				return nil, nil, fmt.Errorf(`metrics exporter %q from pipeline %q is not defined.`, eID, pID)
			}
			exporter := exporters[eID]
			// TODO: Fix the platform. It should be "windows" for Windows and "linux" for Linux.
			cid := componentID{subagent: "metrics", component: "exporter", id: eID, platform: "windows", componentType: exporter.Type}
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
			default:
				return nil, nil, unsupportedComponentTypeError(cid)
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
			return nil, nil, nil, fmt.Errorf(`logging receiver %q from pipeline %q is not defined.`, rID, pID)
		}
	}
	return fbTails, fbSyslogs, fbWinEventlogs, nil
}

func generateFluentBitFilters(processors map[string]*config.LoggingProcessor, pipelines map[string]*config.LoggingPipeline) ([]*conf.FilterParser, error) {
	fbFilterParsers := []*conf.FilterParser{}
	var pipelineIDs []string
	for p := range pipelines {
		pipelineIDs = append(pipelineIDs, p)
	}
	sort.Strings(pipelineIDs)
	for _, pID := range pipelineIDs {
		pipeline := pipelines[pID]
		for _, processorID := range pipeline.Processors {
			p, ok := processors[processorID]
			if !isDefaultProcessor(processorID) && !ok {
				return nil, fmt.Errorf(`logging processor %q from pipeline %q is not defined.`, processorID, pID)
			}
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

func isDefaultProcessor(name string) bool {
	switch name {
	case "lib:apache", "lib:apache2", "lib:apache_error", "lib:mongodb", "lib:nginx",
		"lib:syslog-rfc3164", "lib:syslog-rfc5424":
		return true
	default:
		return false
	}
}

func extractExporterPlugins(exporters map[string]*config.LoggingExporter, pipelines map[string]*config.LoggingPipeline, hostInfo *host.InfoStat) (
	[]*conf.FilterModifyAddLogName, []*conf.FilterRewriteTag, []*conf.FilterModifyRemoveLogName, []*conf.Stackdriver, error) {
	fbFilterModifyAddLogNames := []*conf.FilterModifyAddLogName{}
	fbFilterRewriteTags := []*conf.FilterRewriteTag{}
	fbFilterModifyRemoveLogNames := []*conf.FilterModifyRemoveLogName{}
	fbStackdrivers := []*conf.Stackdriver{}
	if err := config.ValidateComponentIds(pipelines, "logging", "pipeline"); err != nil {
		return nil, nil, nil, nil, err
	}
	if err := config.ValidateComponentIds(exporters, "logging", "exporter"); err != nil {
		return nil, nil, nil, nil, err
	}
	var pipelineIDs []string
	for p := range pipelines {
		pipelineIDs = append(pipelineIDs, p)
	}
	sort.Strings(pipelineIDs)
	stackdriverExporters := make(map[string][]string)
	for _, pID := range pipelineIDs {
		pipeline := pipelines[pID]
		for _, exporterID := range pipeline.Exporters {
			e, ok := exporters[exporterID]
			if !ok {
				return nil, nil, nil, nil, fmt.Errorf(`logging exporter %q from pipeline %q is not defined.`, exporterID, pID)
			} else if e.Type != "google_cloud_logging" {
				// TODO: Fix the platform. It should be "windows" for Windows and "linux" for Linux.
				cid := componentID{subagent: "logging", component: "exporter", id: exporterID, platform: "linux", componentType: e.Type}
				return nil, nil, nil, nil, unsupportedComponentTypeError(cid)
			}
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
	if err := config.ValidateComponentIds(processors, "logging", "processor"); err != nil {
		return nil, nil, err
	}
	var names []string
	for n := range processors {
		names = append(names, n)
	}
	sort.Strings(names)

	for _, name := range names {
		p := processors[name]
		cid := componentID{subagent: "logging", component: "processor", componentType: p.Type, id: name, platform: "linux"}
		switch t := p.Type; t {
		case "parse_json":
			fbJSONParser := conf.ParserJSON{
				Name:       name,
				TimeKey:    p.TimeKey,
				TimeFormat: p.TimeFormat,
			}
			fbJSONParsers = append(fbJSONParsers, &fbJSONParser)
		case "parse_regex":
			if p.Regex == "" {
				return nil, nil, missingRequiredParameterError(cid, "regex")
			}
			fbRegexParser := conf.ParserRegex{
				Name:       name,
				Regex:      p.Regex,
				TimeKey:    p.TimeKey,
				TimeFormat: p.TimeFormat,
			}
			fbRegexParsers = append(fbRegexParsers, &fbRegexParser)
		default:
			return nil, nil, unsupportedComponentTypeError(cid)
		}
	}
	return fbJSONParsers, fbRegexParsers, nil
}

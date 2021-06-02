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
	"path/filepath"
	"sort"
	"strings"

	"github.com/GoogleCloudPlatform/ops-agent/collectd"
	"github.com/GoogleCloudPlatform/ops-agent/fluentbit/conf"
	"github.com/GoogleCloudPlatform/ops-agent/internal/version"
	"github.com/GoogleCloudPlatform/ops-agent/otel"
	"github.com/shirou/gopsutil/host"

	"text/template"

	yaml "gopkg.in/yaml.v2"
)

var (
	// Supported component types.
	supportedComponentTypes = map[string][]string{
		"linux_logging_exporter":   []string{"google_cloud_logging"},
		"linux_logging_receiver":   []string{"files", "syslog"},
		"linux_metrics_exporter":   []string{"google_cloud_monitoring"},
		"linux_metrics_receiver":   []string{"hostmetrics"},
		"windows_logging_receiver": []string{"files", "syslog", "windows_event_log"},
		"windows_logging_exporter": []string{"google_cloud_logging"},
		"windows_metrics_receiver": []string{"hostmetrics", "iis", "mssql"},
		"windows_metrics_exporter": []string{"google_cloud_monitoring"},
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

type UnifiedConfig struct {
	Logging *logging          `yaml:"logging"`
	Metrics *collectd.Metrics `yaml:"metrics"`
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

func generateOtelConfig(metrics *collectd.Metrics, hostInfo *host.InfoStat) (string, error) {
	userAgent, _ := getUserAgent("Google-Cloud-Ops-Agent-Collector", hostInfo)
	hostMetricsList := []*otel.HostMetrics{}
	mssqlList := []*otel.MSSQL{}
	iisList := []*otel.IIS{}
	stackdriverList := []*otel.Stackdriver{}
	serviceList := []*otel.Service{}
	receiverNameMap := make(map[string]string)
	exporterNameMap := make(map[string]string)
	if metrics != nil {
		hostmetricsReceiverFactories, mssqlReceiverFactories, iisReceiverFactories, err := extractOtelReceiverFactories(metrics.Receivers)
		if err != nil {
			return "", err
		}
		hostMetricsList, mssqlList, iisList, receiverNameMap, err = generateOtelReceivers(hostmetricsReceiverFactories, mssqlReceiverFactories, iisReceiverFactories, metrics.Service.Pipelines)
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
		Windows:   hostInfo.OS == "windows",
	}.Generate()
	if err != nil {
		// TODO: unwrap template errors so users don't see extraneous template line numbers
		return "", err
	}
	return otelConfig, nil
}

func generateOtelServices(receiverNameMap map[string]string, exporterNameMap map[string]string, pipelines map[string]collectd.Pipeline) ([]*otel.Service, error) {
	serviceList := []*otel.Service{}
	var pipelineIDs []string
	for p := range pipelines {
		pipelineIDs = append(pipelineIDs, p)
	}
	sort.Strings(pipelineIDs)
	for _, pID := range pipelineIDs {
		if strings.HasPrefix(pID, "lib:") {
			return nil, reservedIdPrefixError("metrics", "pipeline", pID)
		}

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
			} else {
				// TODO: replace receiverNameMap[rID] with the actual receiver type.
				return nil, unsupportedComponentTypeError("windows", "metrics", "receiver", receiverNameMap[rID], rID)
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
		DB:   filepath.Join(stateDir, "buffers", "ops-agent-fluent-bit"),
		Path: filepath.Join(logsDir, "logging-module.log"),
	}
	tailCollectd := conf.Tail{
		Tag:  "ops-agent-collectd",
		DB:   filepath.Join(stateDir, "buffers", "ops-agent-collectd"),
		Path: filepath.Join(logsDir, "metrics-module.log"),
	}
	tails = append(tails, &tailFluentbit)
	if hostInfo.OS != "windows" {
		tails = append(tails, &tailCollectd)
	}

	return tails
}

// defaultStackdriverOutputs returns the default Stackdriver sections for the agents' own logs.
func defaultStackdriverOutputs() (stackdrivers []*conf.Stackdriver) {
	return []*conf.Stackdriver{
		{
			Match: "ops-agent-fluent-bit|ops-agent-collectd",
		},
	}
}

var userAgentTemplate = template.Must(template.New("useragent").Parse(`{{.Prefix}}/{{.AgentVersion}} (BuildDistro={{.BuildDistro}};Platform={{.Platform}};ShortName={{.ShortName}};ShortVersion={{.ShortVersion}},gzip(gfe))`))

func getUserAgent(prefix string, hostInfo *host.InfoStat) (string, error) {
	userAgent := map[string]string{
		"Prefix":       prefix,
		"AgentVersion": version.Version,
		"BuildDistro":  version.BuildDistro,
		"Platform":     hostInfo.OS,
		"ShortName":    hostInfo.Platform,
		"ShortVersion": hostInfo.PlatformVersion,
	}
	var userAgentBuilder strings.Builder
	if err := userAgentTemplate.Execute(&userAgentBuilder, userAgent); err != nil {
		fmt.Println(err.Error())
		return "", err
	}
	return userAgentBuilder.String(), nil
}

func generateFluentBitConfigs(logging *logging, logsDir string, stateDir string, hostInfo *host.InfoStat) (string, string, error) {
	fbTails := defaultTails(logsDir, stateDir, hostInfo)
	userAgent, _ := getUserAgent("Google-Cloud-Ops-Agent-Logging", hostInfo)
	fbStackdrivers := defaultStackdriverOutputs()
	fbSyslogs := []*conf.Syslog{}
	fbWinEventlogs := []*conf.WindowsEventlog{}
	fbFilterParsers := []*conf.FilterParser{}
	fbFilterAddLogNames := []*conf.FilterModifyAddLogName{}
	fbFilterRewriteTags := []*conf.FilterRewriteTag{}
	fbFilterRemoveLogNames := []*conf.FilterModifyRemoveLogName{}
	jsonParsers := []*conf.ParserJSON{}
	regexParsers := []*conf.ParserRegex{}

	if logging != nil && logging.Service != nil {
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

func extractOtelReceiverFactories(receivers map[string]collectd.Receiver) (map[string]*hostmetricsReceiverFactory, map[string]*mssqlReceiverFactory, map[string]*iisReceiverFactory, error) {
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
		default:
			return nil, nil, nil, unsupportedComponentTypeError("windows", "metrics", "receiver", r.Type, n)
		}
	}
	return hostmetricsReceiverFactories, mssqlReceiverFactories, iisReceiverFactories, nil
}

// unsupportedComponentTypeError returns an error message when users specify a receiver, processor, or exporter type that is not supported.
// platform should be "linux" or "windows".
// subagent should be "logging", or "metrics".
// component should be "receiver", "processor", or "exporter".
// t is the type of the receiver, processor, or exporter. e.g. "hostmetrics".
// id is the id of the receiver, processor, or exporter.
func unsupportedComponentTypeError(platform, subagent, component, componentType, id string) error {
	// e.g. metrics receiver "receiver_1" with type "unsupported_type" is not supported. Supported metrics receiver types: [hostmetrics, iis, mssql].
	return fmt.Errorf(`%s %s %q with type %q is not supported. Supported %s %s types: [%s].`,
		subagent, component, id, componentType, subagent, component, strings.Join(supportedComponentTypes[platform+"_"+subagent+"_"+component], ", "))
}

// reservedIdPrefixError returns an error message when users specify a id that starts with "lib:" which is reserved.
// subagent should be "logging", or "metrics". component should be "receiver", "processor", or "exporter".
// id is the ID of the receiver, processor, or exporter.
func reservedIdPrefixError(subagent, component, id string) error {
	// e.g. logging receiver id %q is not allowed because prefix 'lib:' is reserved for pre-defined receivers.
	return fmt.Errorf(`%s %s id %q is not allowed because prefix 'lib:' is reserved for pre-defined %ss.`,
		subagent, component, id, component)
}

// missingRequiredParameterError returns an error message when users miss a required parameter.
// subagent should be "logging", or "metrics". component should be "receiver", "processor", or "exporter".
// t is the type of the receiver, processor, or exporter. e.g. "hostmetrics". id is the id of the receiver, processor, or exporter.
// parameter is name of the parameter that is missing.
func missingRequiredParameterError(subagent, component, componentType, id, parameter string) error {
	// e.g. parameter "include_paths" is required in logging receiver "receiver_1" because its type is "files".
	return fmt.Errorf(`parameter %q is required in %s %s %q because its type is %q.`, parameter, subagent, component, id, componentType)
}

// unsupportedParameterError returns an error message when users specifies an unsupported parameter.
// subagent should be "logging", or "metrics". component should be "receiver", "processor", or "exporter".
// t is the type of the receiver, processor, or exporter. e.g. "hostmetrics". id is the id of the receiver, processor, or exporter.
// parameter is name of the parameter that is not supported.
func unsupportedParameterError(subagent, component, componentType, id, parameter string) error {
	// e.g. parameter "transport_protocol" in logging receiver "receiver_1" is not supported. Supported parameters
	// for "files" type logging receiver: [include_paths, exclude_paths].
	return fmt.Errorf(`parameter %q in %s %s %q is not supported. Supported parameters for %q type %s %s: [%s].`,
		parameter, subagent, component, id, componentType, subagent, component, strings.Join(supportedParameters[componentType], ", "))
}

func extractReceiverFactories(receivers map[string]*receiver) (map[string]*fileReceiverFactory, map[string]*syslogReceiverFactory, map[string]*wineventlogReceiverFactory, error) {
	fileReceiverFactories := map[string]*fileReceiverFactory{}
	syslogReceiverFactories := map[string]*syslogReceiverFactory{}
	wineventlogReceiverFactories := map[string]*wineventlogReceiverFactory{}
	for rID, r := range receivers {
		if strings.HasPrefix(rID, "lib:") {
			return nil, nil, nil, reservedIdPrefixError("logging", "receiver", rID)
		}
		switch r.Type {
		case "files":
			if r.TransportProtocol != "" {
				return nil, nil, nil, unsupportedParameterError("logging", "receiver", r.Type, rID, "transport_protocol")
			}
			if r.ListenHost != "" {
				return nil, nil, nil, unsupportedParameterError("logging", "receiver", r.Type, rID, "listen_host")
			}
			if r.ListenPort != 0 {
				return nil, nil, nil, unsupportedParameterError("logging", "receiver", r.Type, rID, "listen_port")
			}
			if r.Channels != nil {
				return nil, nil, nil, unsupportedParameterError("logging", "receiver", r.Type, rID, "channels")
			}
			if r.IncludePaths == nil {
				return nil, nil, nil, missingRequiredParameterError("logging", "receiver", r.Type, rID, "include_paths")
			}
			fileReceiverFactories[rID] = &fileReceiverFactory{
				IncludePaths: r.IncludePaths,
				ExcludePaths: r.ExcludePaths,
			}
		case "syslog":
			if r.IncludePaths != nil {
				return nil, nil, nil, unsupportedParameterError("logging", "receiver", r.Type, rID, "include_paths")
			}
			if r.ExcludePaths != nil {
				return nil, nil, nil, unsupportedParameterError("logging", "receiver", r.Type, rID, "exclude_paths")
			}
			if r.TransportProtocol != "tcp" && r.TransportProtocol != "udp" {
				return nil, nil, nil, fmt.Errorf(`unknown transport protocol %q in the logging receiver %q. Supported transport protocol for %q type logging receiver: [tcp, udp].`, r.TransportProtocol, rID, r.Type)
			}
			if r.Channels != nil {
				return nil, nil, nil, unsupportedParameterError("logging", "receiver", r.Type, rID, "channels")
			}
			syslogReceiverFactories[rID] = &syslogReceiverFactory{
				TransportProtocol: r.TransportProtocol,
				ListenHost:        r.ListenHost,
				ListenPort:        r.ListenPort,
			}
		case "windows_event_log":
			if r.TransportProtocol != "" {
				return nil, nil, nil, unsupportedParameterError("logging", "receiver", r.Type, rID, "transport_protocol")
			}
			if r.ListenHost != "" {
				return nil, nil, nil, unsupportedParameterError("logging", "receiver", r.Type, rID, "listen_host")
			}
			if r.ListenPort != 0 {
				return nil, nil, nil, unsupportedParameterError("logging", "receiver", r.Type, rID, "listen_port")
			}
			if r.IncludePaths != nil {
				return nil, nil, nil, unsupportedParameterError("logging", "receiver", r.Type, rID, "include_paths")
			}
			if r.ExcludePaths != nil {
				return nil, nil, nil, unsupportedParameterError("logging", "receiver", r.Type, rID, "exclude_paths")
			}
			if r.Channels == nil {
				return nil, nil, nil, missingRequiredParameterError("logging", "receiver", r.Type, rID, "channels")
			}
			wineventlogReceiverFactories[rID] = &wineventlogReceiverFactory{
				Channels: r.Channels,
			}
		default:
			// TODO: Fix the supported types. It should be windowsLoggingReceiverTypes for Windows and linuxLoggingReceiverType for Linux.
			return nil, nil, nil, unsupportedComponentTypeError("windows", "logging", "receiver", r.Type, rID)
		}
	}
	return fileReceiverFactories, syslogReceiverFactories, wineventlogReceiverFactories, nil
}

func generateOtelReceivers(hostmetricsReceiverFactories map[string]*hostmetricsReceiverFactory, mssqlReceiverFactories map[string]*mssqlReceiverFactory, iisReceiverFactories map[string]*iisReceiverFactory, pipelines map[string]collectd.Pipeline) ([]*otel.HostMetrics, []*otel.MSSQL, []*otel.IIS, map[string]string, error) {
	hostMetricsList := []*otel.HostMetrics{}
	mssqlList := []*otel.MSSQL{}
	iisList := []*otel.IIS{}
	receiverNameMap := make(map[string]string)
	var pipelineIDs []string
	for p := range pipelines {
		pipelineIDs = append(pipelineIDs, p)
	}
	sort.Strings(pipelineIDs)
	for _, pID := range pipelineIDs {
		p := pipelines[pID]
		for _, rID := range p.ReceiverIDs {
			if strings.HasPrefix(rID, "lib:") {
				return nil, nil, nil, nil, reservedIdPrefixError("metrics", "receiver", rID)
			}
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

func generateOtelExporters(exporters map[string]collectd.Exporter, pipelines map[string]collectd.Pipeline) ([]*otel.Stackdriver, map[string]string, error) {
	stackdriverList := []*otel.Stackdriver{}
	exportNameMap := make(map[string]string)
	var pipelineIDs []string
	for p := range pipelines {
		pipelineIDs = append(pipelineIDs, p)
	}
	sort.Strings(pipelineIDs)
	for _, pID := range pipelineIDs {
		p := pipelines[pID]
		for _, eID := range p.ExporterIDs {
			if strings.HasPrefix(eID, "lib:") {
				return nil, nil, reservedIdPrefixError("metrics", "exporter", eID)
			}
			if _, ok := exporters[eID]; !ok {
				return nil, nil, fmt.Errorf(`metrics exporter %q from pipeline %q is not defined.`, eID, pID)
			}
			exporter := exporters[eID]
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
				return nil, nil, unsupportedComponentTypeError("windows", "metrics", "exporter", exporter.Type, eID)
			}
		}
	}
	if len(stackdriverList) > 1 {
		return nil, nil, fmt.Errorf(`Only one exporter of the same type in [google_cloud_monitoring] is allowed.`)
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
					DB:   filepath.Join(stateDir, "buffers", pID+"_"+rID),
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
					DB:           filepath.Join(stateDir, "buffers", pID+"_"+rID),
				}
				fbWinEventlogs = append(fbWinEventlogs, &fbWinlog)
				continue
			}
			return nil, nil, nil, fmt.Errorf(`logging receiver %q from pipeline %q is not defined.`, rID, pID)
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
				return nil, fmt.Errorf(`logging processor %q from pipeline %q is not defined.`, processorID, pipelineID)
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
			return nil, nil, nil, nil, reservedIdPrefixError("logging", "pipeline", p)
		}
		pipelineIDs = append(pipelineIDs, p)
	}
	sort.Strings(pipelineIDs)
	stackdriverExporters := make(map[string][]string)
	for _, pipelineID := range pipelineIDs {
		pipeline := pipelines[pipelineID]
		for _, exporterID := range pipeline.Exporters {
			if strings.HasPrefix(exporterID, "lib:") {
				return nil, nil, nil, nil, reservedIdPrefixError("logging", "exporter", exporterID)
			}
			e, ok := exporters[exporterID]
			if !ok {
				return nil, nil, nil, nil, fmt.Errorf(`logging exporter %q from pipeline %q is not defined.`, exporterID, pipelineID)
			} else if e.Type != "google_cloud_logging" {
				return nil, nil, nil, nil, unsupportedComponentTypeError("linux", "logging", "exporter", e.Type, exporterID)
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
				stackdriverExporters[exporterID] = append(stackdriverExporters[exporterID], rID)
			}
		}
	}
	for _, tags := range stackdriverExporters {
		fbStackdrivers = append(fbStackdrivers, &conf.Stackdriver{
			Match: strings.Join(tags, "|"),
		})
	}
	return fbFilterModifyAddLogNames, fbFilterRewriteTags, fbFilterModifyRemoveLogNames, fbStackdrivers, nil
}

func extractFluentBitParsers(processors map[string]*processor) ([]*conf.ParserJSON, []*conf.ParserRegex, error) {
	var names []string
	for n := range processors {
		if strings.HasPrefix(n, "lib:") {
			return nil, nil, reservedIdPrefixError("logging", "processor", n)
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
			if p.Regex == "" {
				return nil, nil, missingRequiredParameterError("logging", "processor", p.Type, name, "regex")
			}
			fbRegexParser := conf.ParserRegex{
				Name:       name,
				Regex:      p.Regex,
				TimeKey:    p.TimeKey,
				TimeFormat: p.TimeFormat,
			}
			fbRegexParsers = append(fbRegexParsers, &fbRegexParser)
		default:
			return nil, nil, fmt.Errorf(`logging processor %q with type %q is not supported. Supported logging processor types: [parse_json, parse_regex].`, name, t)
		}
	}
	return fbJSONParsers, fbRegexParsers, nil
}

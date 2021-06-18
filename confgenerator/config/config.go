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

// Package config represents the Ops Agent configuration.
package config

import (
	"fmt"
	"math"
	"net"
	"reflect"
	"sort"
	"strings"
	"time"

	yaml "gopkg.in/yaml.v2"
)

// Ops Agent config.
type UnifiedConfig struct {
	Logging *Logging `yaml:"logging"`
	Metrics *Metrics `yaml:"metrics"`
}

func (uc *UnifiedConfig) HasLogging() bool {
	return uc.Logging != nil
}

func (uc *UnifiedConfig) HasMetrics() bool {
	return uc.Metrics != nil
}

func ParseUnifiedConfig(input []byte, platform string) (UnifiedConfig, error) {
	config := UnifiedConfig{}
	if err := yaml.UnmarshalStrict(input, &config); err != nil {
		return UnifiedConfig{}, fmt.Errorf("the agent config file is not valid YAML. detailed error: %s", err)
	}
	if err := config.Validate(platform); err != nil {
		return config, err
	}
	return config, nil
}

type configComponent struct {
	Type string `yaml:"type"`
}

// Ops Agent logging config.
type Logging struct {
	Receivers  map[string]*LoggingReceiver  `yaml:"receivers"`
	Processors map[string]*LoggingProcessor `yaml:"processors"`
	Exporters  map[string]*LoggingExporter  `yaml:"exporters"`
	Service    *LoggingService              `yaml:"service"`
}

type LoggingReceiverFiles struct {
	IncludePaths []string `yaml:"include_paths"`
	ExcludePaths []string `yaml:"exclude_paths,omitempty"` // optional
}

type LoggingReceiverSyslog struct {
	TransportProtocol string `yaml:"transport_protocol"` // one of "tcp" or "udp"
	ListenHost        string `yaml:"listen_host"`
	ListenPort        uint16 `yaml:"listen_port"`
}

type LoggingReceiverWinevtlog struct {
	Channels []string `yaml:"channels"`
}

type LoggingReceiver struct {
	configComponent `yaml:",inline"`

	LoggingReceiverSyslog    `yaml:",inline"` // Type "syslog"
	LoggingReceiverFiles     `yaml:",inline"` // Type "files"
	LoggingReceiverWinevtlog `yaml:",inline"` // Type "windows_event_log"
}

type LoggingProcessorParseJson struct {
	Field      string `yaml:"field,omitempty"`       // optional, default to "message"
	TimeKey    string `yaml:"time_key,omitempty"`    // optional, by default does not parse timestamp
	TimeFormat string `yaml:"time_format,omitempty"` // optional, must be provided if time_key is present
}

type LoggingProcessorParseRegex struct {
	Regex string `yaml:"regex"`

	LoggingProcessorParseJson `yaml:",inline"` // Type "parse_json"
}

type LoggingProcessor struct {
	configComponent `yaml:",inline"`

	LoggingProcessorParseRegex `yaml:",inline"` // Type "parse_json" or "parse_regex"
}

type LoggingExporter struct {
	configComponent `yaml:",inline"`
}

type LoggingService struct {
	Pipelines map[string]*LoggingPipeline
}

type LoggingPipeline struct {
	Receivers  []string `yaml:"receivers"`
	Processors []string `yaml:"processors"`
	Exporters  []string `yaml:"exporters"`
}

// Ops Agent metrics config.
type Metrics struct {
	Receivers map[string]*MetricsReceiver `yaml:"receivers"`
	Exporters map[string]*MetricsExporter `yaml:"exporters"`
	Service   *MetricsService             `yaml:"service"`
}

type MetricsReceiver struct {
	configComponent `yaml:",inline"`

	CollectionInterval string `yaml:"collection_interval"` // time.Duration format
}

type MetricsExporter struct {
	configComponent `yaml:",inline"`
}

type MetricsService struct {
	Pipelines map[string]*MetricsPipeline `yaml:"pipelines"`
}

type MetricsPipeline struct {
	ReceiverIDs []string `yaml:"receivers"`
	ExporterIDs []string `yaml:"exporters"`
}

func (uc *UnifiedConfig) Validate(platform string) error {
	if uc.Logging != nil {
		if err := uc.Logging.Validate(platform); err != nil {
			return err
		}
	}
	if uc.Metrics != nil {
		if err := uc.Metrics.Validate(platform); err != nil {
			return err
		}
	}
	return nil
}

func (l *Logging) Validate(platform string) error {
	subagent := "logging"
	if err := validateComponentIds(l.Receivers, subagent, "receiver"); err != nil {
		return err
	}
	if err := validateComponentIds(l.Processors, subagent, "processor"); err != nil {
		return err
	}
	if err := validateComponentIds(l.Exporters, subagent, "exporter"); err != nil {
		return err
	}
	for id, r := range l.Receivers {
		if err := r.ValidateType(subagent, "receiver", id, platform); err != nil {
			return err
		}
	}
	for id, p := range l.Processors {
		if err := p.ValidateType(subagent, "processor", id, platform); err != nil {
			return err
		}
	}
	for id, e := range l.Exporters {
		if err := e.ValidateType(subagent, "exporter", id, platform); err != nil {
			return err
		}
	}
	for id, r := range l.Receivers {
		if err := r.ValidateParameters(subagent, "receiver", id); err != nil {
			return err
		}
	}
	for id, p := range l.Processors {
		if err := p.ValidateParameters(subagent, "processor", id); err != nil {
			return err
		}
	}
	if l.Service == nil {
		return nil
	}
	if err := validateComponentIds(l.Service.Pipelines, subagent, "pipeline"); err != nil {
		return err
	}
	for _, id := range SortedKeys(l.Service.Pipelines) {
		p := l.Service.Pipelines[id]
		if err := validateComponentKeys(l.Receivers, p.Receivers, subagent, "receiver", id); err != nil {
			return err
		}
		validProcessors := map[string]*LoggingProcessor{}
		for k, v := range l.Processors {
			validProcessors[k] = v
		}
		for _, k := range defaultProcessors {
			validProcessors[k] = nil
		}
		if err := validateComponentKeys(validProcessors, p.Processors, subagent, "processor", id); err != nil {
			return err
		}
		if err := validateComponentKeys(l.Exporters, p.Exporters, subagent, "exporter", id); err != nil {
			return err
		}
	}
	return nil
}

func (m *Metrics) Validate(platform string) error {
	subagent := "metrics"
	if err := validateComponentIds(m.Receivers, subagent, "receiver"); err != nil {
		return err
	}
	if err := validateComponentIds(m.Exporters, subagent, "exporter"); err != nil {
		return err
	}
	for id, r := range m.Receivers {
		if err := r.ValidateType(subagent, "receiver", id, platform); err != nil {
			return err
		}
	}
	for id, e := range m.Exporters {
		if err := e.ValidateType(subagent, "exporter", id, platform); err != nil {
			return err
		}
	}
	for id, r := range m.Receivers {
		if err := r.ValidateParameters(subagent, "receiver", id); err != nil {
			return err
		}
	}
	for id, r := range m.Receivers {
		if _, err := ValidateCollectionInterval(id, r.CollectionInterval); err != nil {
			return err
		}
	}
	if m.Service == nil {
		return nil
	}
	if err := validateComponentIds(m.Service.Pipelines, subagent, "pipeline"); err != nil {
		return err
	}
	for _, id := range SortedKeys(m.Service.Pipelines) {
		p := m.Service.Pipelines[id]
		if err := validateComponentKeys(m.Receivers, p.ReceiverIDs, subagent, "receiver", id); err != nil {
			return err
		}
		if err := validateComponentKeys(m.Exporters, p.ExporterIDs, subagent, "exporter", id); err != nil {
			return err
		}
	}
	return nil
}

func sliceContains(slice []string, value string) bool {
	for _, e := range slice {
		if e == value {
			return true
		}
	}
	return false
}

func (c *configComponent) ValidateType(subagent string, component string, id string, platform string) error {
	supportedTypes := supportedComponentTypes[platform+"_"+subagent+"_"+component]
	if !sliceContains(supportedTypes, c.Type) {
		return unsupportedComponentTypeError(subagent, component, id, c.Type, supportedTypes)
	}
	return nil
}

func (r *LoggingReceiver) ValidateParameters(subagent string, component string, id string) error {
	return validateParameters(*r, subagent, component, id, r.Type)
}

func (p *LoggingProcessor) ValidateParameters(subagent string, component string, id string) error {
	return validateParameters(*p, subagent, component, id, p.Type)
}

func (r *MetricsReceiver) ValidateParameters(subagent string, component string, id string) error {
	return validateParameters(*r, subagent, component, id, r.Type)
}

type yamlField struct {
	Name     string
	Required bool
	Value    interface{}
	IsZero   bool
}

func nonZeroFields(sm reflect.Value) []yamlField {
	var parameters []yamlField
	tm := sm.Type()
	if tm.NumField() != sm.NumField() {
		panic(fmt.Sprintf("expected the number of fields in %v and %v to match", sm, tm))
	}
	for i := 0; i < tm.NumField(); i++ {
		f := tm.Field(i)
		t, _ := f.Tag.Lookup("yaml")
		split := strings.Split(t, ",")
		n := split[0]
		annotations := split[1:]
		if n == "-" {
			continue
		} else if n == "" {
			n = strings.ToLower(f.Name)
		}
		v := sm.Field(i)
		if sliceContains(annotations, "inline") {
			// Expand inline structs.
			parameters = append(parameters, nonZeroFields(v)...)
		} else if f.Name[:1] != strings.ToLower(f.Name[:1]) { // skip private non-struct fields
			parameters = append(parameters, yamlField{
				Name:     n,
				Required: !sliceContains(annotations, "omitempty"),
				Value:    v.Interface(),
				IsZero:   v.IsZero(),
			})
		}
	}
	return parameters
}

func validateParameters(s interface{}, subagent string, component string, id string, componentType string) error {
	supportedParameters := supportedParameters[componentType]
	// Include type when checking.
	allParameters := []string{"type"}
	allParameters = append(allParameters, supportedParameters...)
	additionalValidation, hasAdditionalValidation := additionalParameterValidation[componentType]
	sm := reflect.ValueOf(s)
	parameters := nonZeroFields(sm)
	for _, p := range parameters {
		if !sliceContains(allParameters, p.Name) {
			if !p.IsZero {
				return unsupportedParameterError(subagent, component, id, componentType, p.Name, supportedParameters)
			}
			continue
		}
		if p.IsZero && p.Required {
			return missingRequiredParameterError(subagent, component, id, componentType, p.Name)
		}
		if hasAdditionalValidation {
			if f, ok := additionalValidation[p.Name]; ok {
				if err := f(p.Value, p.Name, subagent, component, id, componentType); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func ValidateCollectionInterval(receiverID string, collectionInterval string) (float64, error) {
	t, err := time.ParseDuration(collectionInterval)
	if err != nil {
		return math.NaN(), fmt.Errorf("parameter \"collection_interval\" in metrics receiver %q has invalid value %q that is not an interval (e.g. \"60s\"). Detailed error: %s", receiverID, collectionInterval, err)
	}
	interval := t.Seconds()
	if interval < 10 {
		return math.NaN(), fmt.Errorf("parameter \"collection_interval\" in metrics receiver %q has invalid value \"%vs\" that is below the minimum threshold of \"10s\".", receiverID, interval)
	}
	return interval, nil
}

var (
	defaultProcessors = []string{
		"lib:apache", "lib:apache2", "lib:apache_error", "lib:mongodb",
		"lib:nginx", "lib:syslog-rfc3164", "lib:syslog-rfc5424"}

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

	supportedParameters = map[string][]string{
		"files":             []string{"include_paths", "exclude_paths"},
		"syslog":            []string{"transport_protocol", "listen_host", "listen_port"},
		"windows_event_log": []string{"channels"},
		"parse_json":        []string{"field", "time_key", "time_format"},
		"parse_regex":       []string{"field", "time_key", "time_format", "regex"},
		"hostmetrics":       []string{"collection_interval"},
		"iis":               []string{"collection_interval"},
		"mssql":             []string{"collection_interval"},
	}

	additionalParameterValidation = map[string]map[string]func(interface{}, string, string, string, string, string) error{
		"syslog": map[string]func(interface{}, string, string, string, string, string) error{
			"transport_protocol": func(v interface{}, p string, subagent string, component string, id string, componentType string) error {
				validValues := []string{"tcp", "udp"}
				if !sliceContains(validValues, v.(string)) {
					return fmt.Errorf(`unknown %s %q in the %s %s %q. Supported %s for %q type %s %s: [%s].`, p, v, subagent, component, id, p, componentType, subagent, component, strings.Join(validValues, ", "))
				}
				return nil
			},
			"listen_host": func(v interface{}, p string, subagent string, component string, id string, componentType string) error {
				if net.ParseIP(v.(string)) == nil {
					return fmt.Errorf(`unknown %s %q in the %s %s %q. Value of %s for %q type %s %s should be a valid IP.`, p, v, subagent, component, id, p, componentType, subagent, component)
				}
				return nil
			},
		},
	}
)

// mapKeys returns keys from a map[string]Any as a map[string]interface{}.
func mapKeys(m interface{}) map[string]interface{} {
	keys := map[string]interface{}{}
	for iter := reflect.ValueOf(m).MapRange(); iter.Next(); {
		k := iter.Key()
		if k.Kind() != reflect.String {
			panic(fmt.Sprintf("key %v not a string", k))
		}
		keys[k.String()] = nil
	}
	return keys
}

// SortedKeys returns keys from a map[string]Any as a sorted string slice.
func SortedKeys(m interface{}) []string {
	var r []string
	for k := range mapKeys(m) {
		r = append(r, k)
	}
	sort.Strings(r)
	return r
}

// findInvalid returns all strings from a slice that are not in allowed.
func findInvalid(actual []string, allowed map[string]interface{}) []string {
	var invalid []string
	for _, v := range actual {
		if _, ok := allowed[v]; !ok {
			invalid = append(invalid, v)
		}
	}
	return invalid
}

func validateComponentIds(components interface{}, subagent string, component string) error {
	for _, id := range SortedKeys(components) {
		if strings.HasPrefix(id, "lib:") {
			return reservedIdPrefixError(subagent, component, id)
		}
	}
	return nil
}

func validateComponentKeys(components interface{}, refs []string, subagent string, component string, pipeline string) error {
	invalid := findInvalid(refs, mapKeys(components))
	if len(invalid) > 0 {
		return fmt.Errorf("%s %s %q from pipeline %q is not defined.", subagent, component, invalid[0], pipeline)
	}
	return nil
}

// reservedIdPrefixError returns an error message when users specify a id that starts with "lib:" which is reserved.
// id is the id of the pipeline, receiver, processor, or exporter.
func reservedIdPrefixError(subagent string, component string, id string) error {
	// e.g. logging receiver id "lib:abc" is not allowed because prefix 'lib:' is reserved for pre-defined receivers.
	return fmt.Errorf(`%s %s id %q is not allowed because prefix 'lib:' is reserved for pre-defined %ss.`,
		subagent, component, id, component)
}

// unsupportedComponentTypeError returns an error message when users specify a component type that is not supported.
// id is the id of the receiver, processor, or exporter.
func unsupportedComponentTypeError(subagent string, component string, id string, componentType string, supportedTypes []string) error {
	// e.g. metrics receiver "receiver_1" with type "unsupported_type" is not supported. Supported metrics receiver types: [hostmetrics, iis, mssql].
	return fmt.Errorf(`%s %s %q with type %q is not supported. Supported %s %s types: [%s].`,
		subagent, component, id, componentType, subagent, component, strings.Join(supportedTypes, ", "))
}

// missingRequiredParameterError returns an error message when users miss a required parameter.
// id is the id of the receiver, processor, or exporter.
// componentType is the type of the receiver, processor, or exporter, e.g., "hostmetrics".
// parameter is name of the parameter that is missing.
func missingRequiredParameterError(subagent string, component string, id string, componentType string, parameter string) error {
	// e.g. parameter "include_paths" is required in logging receiver "receiver_1" because its type is "files".
	return fmt.Errorf(`parameter %q is required in %s %s %q because its type is %q.`, parameter, subagent, component, id, componentType)
}

// unsupportedParameterError returns an error message when users specifies an unsupported parameter.
// id is the id of the receiver, processor, or exporter.
// componentType is the type of the receiver, processor, or exporter, e.g., "hostmetrics".
// parameter is name of the parameter that is not supported.
func unsupportedParameterError(subagent string, component string, id string, componentType string, parameter string, supportedParameters []string) error {
	// e.g. parameter "transport_protocol" in logging receiver "receiver_1" is not supported. Supported parameters
	// for "files" type logging receiver: [include_paths, exclude_paths].
	return fmt.Errorf(`parameter %q in %s %s %q is not supported. Supported parameters for %q type %s %s: [%s].`,
		parameter, subagent, component, id, componentType, subagent, component, strings.Join(supportedParameters, ", "))
}

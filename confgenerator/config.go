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

package confgenerator

import (
	"fmt"
	"log"
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

func (uc *UnifiedConfig) DeepCopy() (UnifiedConfig, error) {
	toYaml, err := yaml.Marshal(uc)
	if err != nil {
		return UnifiedConfig{}, fmt.Errorf("failed to convert UnifiedConfig to yaml: %w.", err)
	}
	fromYaml, err := UnmarshalYamlToUnifiedConfig(toYaml)
	if err != nil {
		return UnifiedConfig{}, fmt.Errorf("failed to convert yaml to UnifiedConfig: %w.", err)
	}

	return fromYaml, nil
}

func UnmarshalYamlToUnifiedConfig(input []byte) (UnifiedConfig, error) {
	config := UnifiedConfig{}
	if err := yaml.UnmarshalStrict(input, &config); err != nil {
		return UnifiedConfig{}, fmt.Errorf("the agent config file is not valid YAML. detailed error: %s", err)
	}
	return config, nil
}

func ParseUnifiedConfigAndValidate(input []byte, platform string) (UnifiedConfig, error) {
	config, err := UnmarshalYamlToUnifiedConfig(input)
	if err != nil {
		return UnifiedConfig{}, err
	}
	if err = config.Validate(platform); err != nil {
		return config, err
	}
	return config, nil
}

type component interface {
	Type() string
	ValidateType(subagent string, component string, id string, platform string) error
	ValidateParameters(subagent string, component string, id string) error
	//Validate() error
}

type configComponent struct {
	ComponentType string `yaml:"type"`
}

func (c *configComponent) Type() string {
	return c.ComponentType
}

type componentTypeRegistry struct {
	Subagent string
	Kind     string
	TypeMap  map[string]func() component
}

func unmarshalComponentYaml(registry *componentTypeRegistry, inner *interface{}, unmarshal func(interface{}) error) error {
	c := configComponent{}
	unmarshal(&c) // Get the type; ignore the error
	f := registry.TypeMap[c.Type()]
	if f == nil {
		return fmt.Errorf("Unknown type %q", c.Type())
	}
	*inner = f()
	return unmarshal(*inner)
}

// Ops Agent logging config.
type loggingReceiverMap map[string]LoggingReceiver
type loggingProcessorMap map[string]LoggingProcessor
type Logging struct {
	Receivers  loggingReceiverMap          `yaml:"receivers,omitempty"`
	Processors loggingProcessorMap         `yaml:"processors,omitempty"`
	Exporters  map[string]*LoggingExporter `yaml:"exporters,omitempty"`
	Service    *LoggingService             `yaml:"service"`
}

type LoggingReceiver interface {
	component
}

type LoggingReceiverFiles struct {
	configComponent `yaml:",inline"`

	IncludePaths []string `yaml:"include_paths,omitempty"`
	ExcludePaths []string `yaml:"exclude_paths,omitempty" config:"optional"`
}

func (r LoggingReceiverFiles) Type() string {
	return "files"
}

func init() {
	registerLoggingReceiverType(func() component { return &LoggingReceiverFiles{} })
}

type LoggingReceiverSyslog struct {
	configComponent `yaml:",inline"`

	TransportProtocol string `yaml:"transport_protocol,omitempty" config:"optional"` // one of "tcp" or "udp"
	ListenHost        string `yaml:"listen_host,omitempty"`
	ListenPort        uint16 `yaml:"listen_port,omitempty"`
}

func (r LoggingReceiverSyslog) Type() string {
	return "syslog"
}

func init() {
	registerLoggingReceiverType(func() component { return &LoggingReceiverSyslog{} })
}

type LoggingReceiverWinevtlog struct {
	configComponent `yaml:",inline"`

	Channels []string `yaml:"channels,omitempty,flow"`
}

func (r LoggingReceiverWinevtlog) Type() string {
	return "windows_event_log"
}

func init() {
	registerLoggingReceiverType(func() component { return &LoggingReceiverWinevtlog{} })
}

var loggingReceiverTypes = &componentTypeRegistry{
	Subagent: "logging", Kind: "receiver",
	TypeMap: map[string]func() component{},
}

func registerLoggingReceiverType(constructor func() component) error {
	name := constructor().(LoggingReceiver).Type()
	if _, ok := loggingReceiverTypes.TypeMap[name]; ok {
		return fmt.Errorf("Duplicate receiver type: %q", name)
	}
	loggingReceiverTypes.TypeMap[name] = constructor
	return nil
}

// Wrapper type to store the unmarshaled YAML value.
type loggingReceiverWrapper struct {
	inner interface{}
}

func (l *loggingReceiverWrapper) UnmarshalYAML(unmarshal func(interface{}) error) error {
	return unmarshalComponentYaml(loggingReceiverTypes, &l.inner, unmarshal)
}

func (m *loggingReceiverMap) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// Unmarshal into a temporary map to capture types.
	tm := map[string]loggingReceiverWrapper{}
	if err := unmarshal(&tm); err != nil {
		return err
	}
	// Unwrap the structs.
	*m = loggingReceiverMap{}
	for k, r := range tm {
		(*m)[k] = r.inner.(LoggingReceiver)
	}
	return nil
}

type LoggingProcessor interface {
	component
	GetField() string
}

type loggingProcessorParseShared struct {
	Field      string `yaml:"field,omitempty" config:"optional"`       // default to "message"
	TimeKey    string `yaml:"time_key,omitempty" config:"optional"`    // by default does not parse timestamp
	TimeFormat string `yaml:"time_format,omitempty" config:"optional"` // must be provided if time_key is present
}

func (p loggingProcessorParseShared) GetField() string {
	return p.Field
}

type LoggingProcessorParseJson struct {
	configComponent             `yaml:",inline"`
	loggingProcessorParseShared `yaml:",inline"`
}

func (r LoggingProcessorParseJson) Type() string {
	return "parse_json"
}

func init() {
	registerLoggingProcessorType(func() component { return &LoggingProcessorParseJson{} })
}

type LoggingProcessorParseRegex struct {
	configComponent             `yaml:",inline"`
	loggingProcessorParseShared `yaml:",inline"`

	Regex string `yaml:"regex,omitempty"`
}

func (r LoggingProcessorParseRegex) Type() string {
	return "parse_regex"
}

func init() {
	registerLoggingProcessorType(func() component { return &LoggingProcessorParseRegex{} })
}

var loggingProcessorTypes = &componentTypeRegistry{
	Subagent: "logging", Kind: "processor",
	TypeMap: map[string]func() component{},
}

func registerLoggingProcessorType(constructor func() component) error {
	name := constructor().(LoggingProcessor).Type()
	if _, ok := loggingProcessorTypes.TypeMap[name]; ok {
		return fmt.Errorf("Duplicate processor type: %q", name)
	}
	loggingProcessorTypes.TypeMap[name] = constructor
	return nil
}

// Wrapper type to store the unmarshaled YAML value.
type loggingProcessorWrapper struct {
	inner interface{}
}

func (l *loggingProcessorWrapper) UnmarshalYAML(unmarshal func(interface{}) error) error {
	return unmarshalComponentYaml(loggingProcessorTypes, &l.inner, unmarshal)
}

func (m *loggingProcessorMap) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// Unmarshal into a temporary map to capture types.
	tm := map[string]loggingProcessorWrapper{}
	if err := unmarshal(&tm); err != nil {
		return err
	}
	// Unwrap the structs.
	*m = loggingProcessorMap{}
	for k, r := range tm {
		(*m)[k] = r.inner.(LoggingProcessor)
	}
	return nil
}

type LoggingExporter struct {
	configComponent `yaml:",inline"`
}

type LoggingService struct {
	Pipelines map[string]*LoggingPipeline
}

type LoggingPipeline struct {
	ReceiverIDs  []string `yaml:"receivers,omitempty,flow"`
	ProcessorIDs []string `yaml:"processors,omitempty,flow"`
	ExporterIDs  []string `yaml:"exporters,omitempty,flow"`
}

// Ops Agent metrics config.
type Metrics struct {
	Receivers  map[string]*MetricsReceiver  `yaml:"receivers"`
	Processors map[string]*MetricsProcessor `yaml:"processors"`
	Exporters  map[string]*MetricsExporter  `yaml:"exporters,omitempty"`
	Service    *MetricsService              `yaml:"service"`
}

type MetricsReceiver struct {
	configComponent `yaml:",inline"`

	CollectionInterval string `yaml:"collection_interval"` // time.Duration format
}

type MetricsProcessorExcludeMetrics struct {
	MetricsPattern []string `yaml:"metrics_pattern,flow"`
}

type MetricsProcessor struct {
	configComponent `yaml:",inline"`

	MetricsProcessorExcludeMetrics `yaml:",inline"` // Type "exclude_metrics"
}

type MetricsExporter struct {
	configComponent `yaml:",inline"`
}

type MetricsService struct {
	Pipelines map[string]*MetricsPipeline `yaml:"pipelines"`
}

type MetricsPipeline struct {
	ReceiverIDs  []string `yaml:"receivers,flow"`
	ProcessorIDs []string `yaml:"processors,flow"`
	ExporterIDs  []string `yaml:"exporters,omitempty,flow"`
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
	if len(l.Exporters) > 0 {
		log.Print(`The "logging.exporters" field is no longer needed and will be ignored. This does not change any functionality. Please remove it from your configuration.`)
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
	for _, id := range sortedKeys(l.Service.Pipelines) {
		p := l.Service.Pipelines[id]
		if err := validateComponentKeys(l.Receivers, p.ReceiverIDs, subagent, "receiver", id); err != nil {
			return err
		}
		validProcessors := map[string]LoggingProcessor{}
		for k, v := range l.Processors {
			validProcessors[k] = v
		}
		for _, k := range defaultProcessors {
			validProcessors[k] = nil
		}
		if err := validateComponentKeys(validProcessors, p.ProcessorIDs, subagent, "processor", id); err != nil {
			return err
		}
		if err := validateComponentTypeCounts(l.Receivers, p.ReceiverIDs, subagent, "receiver"); err != nil {
			return err
		}
		if err := validateComponentTypeCounts(l.Processors, p.ProcessorIDs, subagent, "processor"); err != nil {
			return err
		}
		if len(p.ExporterIDs) > 0 {
			log.Printf(`The "logging.service.pipelines.%s.exporters" field is deprecated and will be ignored. Please remove it from your configuration.`, id)
		}
	}
	return nil
}

func (m *Metrics) Validate(platform string) error {
	subagent := "metrics"
	if err := validateComponentIds(m.Receivers, subagent, "receiver"); err != nil {
		return err
	}
	if err := validateComponentIds(m.Processors, subagent, "processor"); err != nil {
		return err
	}
	if len(m.Exporters) > 0 {
		log.Print(`The "metrics.exporters" field is deprecated and will be ignored. Please remove it from your configuration.`)
	}
	for id, r := range m.Receivers {
		if err := r.ValidateType(subagent, "receiver", id, platform); err != nil {
			return err
		}
	}
	for id, p := range m.Processors {
		if err := p.ValidateType(subagent, "processor", id, platform); err != nil {
			return err
		}
	}
	for id, p := range m.Processors {
		if err := p.ValidateParameters(subagent, "processor", id); err != nil {
			return err
		}
	}
	for id, r := range m.Receivers {
		if err := r.ValidateParameters(subagent, "receiver", id); err != nil {
			return err
		}
	}
	if m.Service == nil {
		return nil
	}
	if err := validateComponentIds(m.Service.Pipelines, subagent, "pipeline"); err != nil {
		return err
	}
	for _, id := range sortedKeys(m.Service.Pipelines) {
		p := m.Service.Pipelines[id]
		if err := validateComponentKeys(m.Receivers, p.ReceiverIDs, subagent, "receiver", id); err != nil {
			return err
		}
		if err := validateComponentKeys(m.Processors, p.ProcessorIDs, subagent, "processor", id); err != nil {
			return err
		}
		if err := validateComponentTypeCounts(m.Receivers, p.ReceiverIDs, subagent, "receiver"); err != nil {
			return err
		}
		if err := validateComponentTypeCounts(m.Processors, p.ProcessorIDs, subagent, "processor"); err != nil {
			return err
		}
		if len(p.ExporterIDs) > 0 {
			log.Printf(`The "metrics.service.pipelines.%s.exporters" field is deprecated and will be ignored. Please remove it from your configuration.`, id)
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

func (c *configComponent) ValidateType(subagent string, kind string, id string, platform string) error {
	supportedTypes := supportedComponentTypes[platform+"_"+subagent+"_"+kind]
	if !sliceContains(supportedTypes, c.Type()) {
		// e.g. metrics receiver "receiver_1" with type "unsupported_type" is not supported.
		// Supported metrics receiver types: [hostmetrics, iis, mssql].
		return fmt.Errorf(`%s %s %q with type %q is not supported. Supported %s %s types: [%s].`,
			subagent, kind, id, c.Type(), subagent, kind, strings.Join(supportedTypes, ", "))
	}
	return nil
}

func (r *LoggingReceiverFiles) ValidateParameters(subagent string, kind string, id string) error {
	return validateParameters(*r, subagent, kind, id, r.Type())
}

func (r *LoggingReceiverSyslog) ValidateParameters(subagent string, kind string, id string) error {
	if err := validateParameters(*r, subagent, kind, id, r.Type()); err != nil {
		return err
	}
	validProtocolValues := []string{"tcp", "udp"}
	if !sliceContains(validProtocolValues, r.TransportProtocol) {
		err := fmt.Errorf(`must be one of [%s].`, strings.Join(validProtocolValues, ", "))
		return fmt.Errorf(`%s has invalid value %q: %s`, parameterErrorPrefix(subagent, kind, id, r.Type(), "transport_protocol"), r.TransportProtocol, err)
	}
	if net.ParseIP(r.ListenHost) == nil {
		err := fmt.Errorf(`must be a valid IP.`)
		return fmt.Errorf(`%s has invalid value %q: %s`, parameterErrorPrefix(subagent, kind, id, r.Type(), "listen_host"), r.ListenHost, err)
	}
	return nil
}

func (r *LoggingReceiverWinevtlog) ValidateParameters(subagent string, kind string, id string) error {
	return validateParameters(*r, subagent, kind, id, r.Type())
}

func (p *LoggingProcessorParseJson) ValidateParameters(subagent string, kind string, id string) error {
	return validateParameters(*p, subagent, kind, id, p.Type())
}

func (p *LoggingProcessorParseRegex) ValidateParameters(subagent string, kind string, id string) error {
	return validateParameters(*p, subagent, kind, id, p.Type())
}

func (r *MetricsReceiver) ValidateParameters(subagent string, kind string, id string) error {
	return validateParameters(*r, subagent, kind, id, r.Type())
}

func (p *MetricsProcessor) ValidateParameters(subagent string, kind string, id string) error {
	return validateParameters(*p, subagent, kind, id, p.Type())
}

type yamlField struct {
	Name     string
	Required bool
	Value    interface{}
	IsZero   bool
}

// collectYamlFields takes a struct object, and extracts all fields annotated by yaml tags.
// Fields from embedded structs with the "inline" annotation are flattened.
// For each field, it collects the yaml name, the value, and whether the value is unset/zero.
// Fields with the "omitempty" annotation are marked as optional.
func collectYamlFields(s interface{}) []yamlField {
	var recurse func(reflect.Value) []yamlField
	recurse = func(sm reflect.Value) []yamlField {
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
				parameters = append(parameters, recurse(v)...)
			} else if f.PkgPath == "" { // skip private non-struct fields
				t, e := f.Tag.Lookup("config")
				if !e {
					t = ""
				}
				parameters = append(parameters, yamlField{
					Name:     n,
					Required: t != "optional",
					Value:    v.Interface(),
					IsZero:   v.IsZero(),
				})
			}
		}
		return parameters
	}
	return recurse(reflect.ValueOf(s))
}

func validateParameters(s interface{}, subagent string, kind string, id string, componentType string) error {
	supportedParameters := supportedParameters[componentType]
	// Include type when checking.
	allParameters := []string{"type"}
	allParameters = append(allParameters, supportedParameters...)
	additionalValidation, hasAdditionalValidation := additionalParameterValidation[componentType]
	parameters := collectYamlFields(s)
	for _, p := range parameters {
		if supportedParameters != nil && !sliceContains(allParameters, p.Name) {
			if !p.IsZero {
				// e.g. parameter "transport_protocol" in "files" type logging receiver "receiver_1" is not supported.
				// Supported parameters: [include_paths, exclude_paths].
				return fmt.Errorf(`%s is not supported. Supported parameters: [%s].`,
					parameterErrorPrefix(subagent, kind, id, componentType, p.Name), strings.Join(supportedParameters, ", "))
			}
			continue
		}
		if p.IsZero && p.Required {
			// e.g. parameter "include_paths" in "files" type logging receiver "receiver_1" is required.
			return fmt.Errorf(`%s is required.`, parameterErrorPrefix(subagent, kind, id, componentType, p.Name))
		}
		if hasAdditionalValidation {
			if f, ok := additionalValidation[p.Name]; ok {
				if err := f(p.Value); err != nil {
					// e.g. parameter "collection_interval" in "hostmetrics" type metrics receiver "receiver_1"
					// has invalid value "1s": below the minimum threshold of "10s".
					return fmt.Errorf(`%s has invalid value %q: %s`, parameterErrorPrefix(subagent, kind, id, componentType, p.Name), p.Value, err)
				}
			}
		}
	}
	return nil
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
		"linux_metrics_processor":   []string{"exclude_metrics"},
		"linux_metrics_exporter":    []string{"google_cloud_monitoring"},
		"windows_logging_receiver":  []string{"files", "syslog", "windows_event_log"},
		"windows_logging_processor": []string{"parse_json", "parse_regex"},
		"windows_logging_exporter":  []string{"google_cloud_logging"},
		"windows_metrics_receiver":  []string{"hostmetrics", "iis", "mssql"},
		"windows_metrics_processor": []string{"exclude_metrics"},
		"windows_metrics_exporter":  []string{"google_cloud_monitoring"},
	}

	componentTypeLimits = map[string]int{
		"google_cloud_monitoring": 1,
		"hostmetrics":             1,
		"iis":                     1,
		"mssql":                   1,
	}

	supportedParameters = map[string][]string{
		"hostmetrics":     []string{"collection_interval"},
		"iis":             []string{"collection_interval"},
		"mssql":           []string{"collection_interval"},
		"exclude_metrics": []string{"metrics_pattern"},
	}

	collectionIntervalValidation = map[string]func(interface{}) error{
		"collection_interval": func(v interface{}) error {
			t, err := time.ParseDuration(v.(string))
			if err != nil {
				return fmt.Errorf(`not an interval (e.g. "60s"). Detailed error: %s`, err)
			}
			interval := t.Seconds()
			if interval < 10 {
				return fmt.Errorf(`below the minimum threshold of "10s".`)
			}
			return nil
		},
	}

	additionalParameterValidation = map[string]map[string]func(interface{}) error{
		"hostmetrics": collectionIntervalValidation,
		"iis":         collectionIntervalValidation,
		"mssql":       collectionIntervalValidation,
		"exclude_metrics": map[string]func(interface{}) error{
			"metrics_pattern": func(v interface{}) error {
				var errors []string
				for _, prefix := range v.([]string) {
					if !strings.HasSuffix(prefix, "/*") {
						errors = append(errors, fmt.Sprintf(`%q must end with "/*"`, prefix))
					}
					// TODO: Relax the prefix check when we support metrics with other prefixes.
					if !strings.HasPrefix(prefix, "agent.googleapis.com/") {
						errors = append(errors, fmt.Sprintf(`%q must start with "agent.googleapis.com/"`, prefix))
					}
				}
				if len(errors) > 0 {
					return fmt.Errorf(strings.Join(errors, " | "))
				}
				return nil
			},
		},
	}
)

// mapKeys returns keys from a map[string]Any as a map[string]bool.
func mapKeys(m interface{}) map[string]bool {
	keys := map[string]bool{}
	switch m := m.(type) {
	case map[string]LoggingReceiver:
		for k := range m {
			keys[k] = true
		}
	case loggingReceiverMap:
		for k := range m {
			keys[k] = true
		}
	case map[string]LoggingProcessor:
		for k := range m {
			keys[k] = true
		}
	case loggingProcessorMap:
		for k := range m {
			keys[k] = true
		}
	case map[string]*LoggingExporter:
		for k := range m {
			keys[k] = true
		}
	case map[string]*LoggingPipeline:
		for k := range m {
			keys[k] = true
		}
	case map[string]*MetricsReceiver:
		for k := range m {
			keys[k] = true
		}
	case map[string]*MetricsProcessor:
		for k := range m {
			keys[k] = true
		}
	case map[string]*MetricsExporter:
		for k := range m {
			keys[k] = true
		}
	case map[string]*MetricsPipeline:
		for k := range m {
			keys[k] = true
		}
	default:
		panic(fmt.Sprintf("Unknown type: %T", m))
	}
	return keys
}

// sortedKeys returns keys from a map[string]Any as a sorted string slice.
func sortedKeys(m interface{}) []string {
	var r []string
	for k := range mapKeys(m) {
		r = append(r, k)
	}
	sort.Strings(r)
	return r
}

// findInvalid returns all strings from a slice that are not in allowed.
func findInvalid(actual []string, allowed map[string]bool) []string {
	var invalid []string
	for _, v := range actual {
		if !allowed[v] {
			invalid = append(invalid, v)
		}
	}
	return invalid
}

func validateComponentIds(components interface{}, subagent string, kind string) error {
	for _, id := range sortedKeys(components) {
		if strings.HasPrefix(id, "lib:") {
			// e.g. logging receiver id "lib:abc" is not allowed because prefix 'lib:' is reserved for pre-defined receivers.
			return fmt.Errorf(`%s %s id %q is not allowed because prefix 'lib:' is reserved for pre-defined %ss.`,
				subagent, kind, id, kind)
		}
	}
	return nil
}

func validateComponentKeys(components interface{}, refs []string, subagent string, kind string, pipeline string) error {
	invalid := findInvalid(refs, mapKeys(components))
	if len(invalid) > 0 {
		return fmt.Errorf("%s %s %q from pipeline %q is not defined.", subagent, kind, invalid[0], pipeline)
	}
	return nil
}

func validateComponentTypeCounts(components interface{}, refs []string, subagent string, kind string) error {
	r := map[string]int{}
	cm := reflect.ValueOf(components)
	for _, id := range refs {
		v := cm.MapIndex(reflect.ValueOf(id))
		if !v.IsValid() {
			continue // Some reserved ids don't map to components.
		}
		t := v.Interface().(component).Type()
		if _, ok := r[t]; ok {
			r[t] += 1
		} else {
			r[t] = 1
		}
		if limit, ok := componentTypeLimits[t]; ok && r[t] > limit {
			if limit == 1 {
				return fmt.Errorf("at most one %s %s with type %q is allowed.", subagent, kind, t)
			}
			return fmt.Errorf("at most %d %s %ss with type %q are allowed.", limit, subagent, kind, t)
		}
	}
	return nil
}

// parameterErrorPrefix returns the common parameter error prefix.
// id is the id of the receiver, processor, or exporter.
// componentType is the type of the receiver, processor, or exporter, e.g., "hostmetrics".
// parameter is name of the parameter.
func parameterErrorPrefix(subagent string, kind string, id string, componentType string, parameter string) string {
	return fmt.Sprintf(`parameter %q in %q type %s %s %q`, parameter, componentType, subagent, kind, id)
}

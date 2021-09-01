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
	"context"
	"fmt"
	"log"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
	"github.com/go-playground/validator/v10"
	yaml "github.com/goccy/go-yaml"
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

func (uc *UnifiedConfig) DeepCopy(platform string) (UnifiedConfig, error) {
	toYaml, err := yaml.Marshal(uc)
	if err != nil {
		return UnifiedConfig{}, fmt.Errorf("failed to convert UnifiedConfig to yaml: %w.", err)
	}
	fromYaml, err := UnmarshalYamlToUnifiedConfig(toYaml, platform)
	if err != nil {
		return UnifiedConfig{}, fmt.Errorf("failed to convert yaml to UnifiedConfig: %w.", err)
	}

	return fromYaml, nil
}

type validatorContext struct {
	ctx context.Context
	v   *validator.Validate
}

type validationErrors []validationError

func (ve validationErrors) Error() string {
	var out []string
	for _, err := range ve {
		out = append(out, err.Error())
	}
	return strings.Join(out, ",")
}

type validationError struct {
	validator.FieldError
}

func (ve validationError) StructField() string {
	// TODO: Fix yaml library so that this is unnecessary.
	// Remove subscript on field name so go-yaml can associate this with a line number.
	parts := strings.Split(ve.FieldError.StructField(), "[")
	return parts[0]
}

// TODO: Implement validationError.Error() for better error messages.

func (v *validatorContext) Struct(s interface{}) error {
	err := v.v.StructCtx(v.ctx, s)
	errors, ok := err.(validator.ValidationErrors)
	if !ok {
		// Including nil
		return err
	}
	var out validationErrors
	for _, err := range errors {
		out = append(out, validationError{err})
	}
	return out
}

type platformKeyType struct{}

// platformKey is a singleton that is used as a Context key for retrieving the current platform from the context.Context.
var platformKey = platformKeyType{}

func newValidator() *validator.Validate {
	v := validator.New()
	v.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("yaml"), ",", 2)[0]
		if name == "-" {
			return ""
		}
		return name
	})
	// platform validates that the current platform is equal to the parameter
	v.RegisterValidationCtx("platform", func(ctx context.Context, fl validator.FieldLevel) bool {
		return ctx.Value(platformKey) == fl.Param()
	})
	// duration validates that the value is a valid durationa and >= the parameter
	v.RegisterValidation("duration", func(fl validator.FieldLevel) bool {
		t, err := time.ParseDuration(fl.Field().String())
		if err != nil {
			return false
		}
		tmin, err := time.ParseDuration(fl.Param())
		if err != nil {
			panic(err)
		}
		return t >= tmin
	})
	return v
}

func UnmarshalYamlToUnifiedConfig(input []byte, platform string) (UnifiedConfig, error) {
	ctx := context.WithValue(context.TODO(), platformKey, platform)
	config := UnifiedConfig{}
	v := &validatorContext{
		ctx: ctx,
		v:   newValidator(),
	}
	if err := yaml.UnmarshalContext(ctx, input, &config, yaml.Strict(), yaml.Validator(v)); err != nil {
		return UnifiedConfig{}, fmt.Errorf("the agent config file is not valid. detailed error: %s", err)
	}
	return config, nil
}

func ParseUnifiedConfigAndValidate(input []byte, platform string) (UnifiedConfig, error) {
	config, err := UnmarshalYamlToUnifiedConfig(input, platform)
	if err != nil {
		return UnifiedConfig{}, err
	}
	if err = config.Validate(platform); err != nil {
		return config, err
	}
	return config, nil
}

type component interface {
	// Type returns the component type string as used in the configuration file (e.g. "hostmetrics")
	Type() string
}

// ConfigComponent holds the shared configuration fields that all components have.
// It is also used by itself when unmarshaling a component's configuration.
type ConfigComponent struct {
	Type string `yaml:"type" validate:"required"`
}

// componentFactory is the value type for the componentTypeRegistry map.
type componentFactory struct {
	// constructor creates a concrete instance for this component. For example, the "files" constructor would return a *LoggingReceiverFiles, which has an IncludePaths field.
	constructor func() component
	// platforms is a list of platforms on which the component is valid, or any platform if the slice is empty.
	platforms []string
}

func (ct componentFactory) supportsPlatform(ctx context.Context) bool {
	platform := ctx.Value(platformKey).(string)
	for _, v := range ct.platforms {
		if v == platform {
			return true
		}
	}
	return len(ct.platforms) == 0
}

type componentTypeRegistry struct {
	// Subagent is "logging" or "metric" (only used for error messages)
	Subagent string
	// Kind is "receiver" or "processor" (only used for error messages)
	Kind string
	// TypeMap contains a map of component "type" string as used in the configuration file to information about that component.
	TypeMap map[string]*componentFactory
}

func (r *componentTypeRegistry) registerType(constructor func() component, platforms ...string) {
	name := constructor().Type()
	if _, ok := r.TypeMap[name]; ok {
		panic(fmt.Sprintf("attempt to register duplicate %s %s type: %q", r.Subagent, r.Kind, name))
	}
	if r.TypeMap == nil {
		r.TypeMap = make(map[string]*componentFactory)
	}
	r.TypeMap[name] = &componentFactory{constructor, platforms}
}

// unmarshalComponentYaml is the custom unmarshaller for reading a component's configuration from the config file.
// It first unmarshals into a struct containing only the "type" field, then looks up the config struct with the full set of fields for that type, and finally unmarshals into an instance of that struct.
func (r *componentTypeRegistry) unmarshalComponentYaml(ctx context.Context, inner *interface{}, unmarshal func(interface{}) error) error {
	c := ConfigComponent{}
	unmarshal(&c) // Get the type; ignore the error
	var o interface{}
	if ct := r.TypeMap[c.Type]; ct != nil && ct.supportsPlatform(ctx) {
		o = ct.constructor()
	}
	if o == nil {
		var supportedTypes []string
		for k, ct := range r.TypeMap {
			if ct.supportsPlatform(ctx) {
				supportedTypes = append(supportedTypes, k)
			}
		}
		sort.Strings(supportedTypes)
		return fmt.Errorf(`%s %s with type %q is not supported. Supported %s %s types: [%s].`,
			r.Subagent, r.Kind, c.Type,
			r.Subagent, r.Kind, strings.Join(supportedTypes, ", "))
	}
	*inner = o
	return unmarshal(*inner)
}

// Ops Agent logging config.
type loggingReceiverMap map[string]LoggingReceiver
type loggingProcessorMap map[string]LoggingProcessor
type Logging struct {
	Receivers  loggingReceiverMap  `yaml:"receivers,omitempty" validate:"dive,keys,startsnotwith=lib:"`
	Processors loggingProcessorMap `yaml:"processors,omitempty" validate:"dive,keys,startsnotwith=lib:"`
	// Exporters are deprecated and ignored, so do not have any validation.
	Exporters map[string]interface{} `yaml:"exporters,omitempty"`
	Service   *LoggingService        `yaml:"service"`
}

type LoggingReceiver interface {
	component
	Components(tag string) []fluentbit.Component
}

var loggingReceiverTypes = &componentTypeRegistry{
	Subagent: "logging", Kind: "receiver",
}

// Wrapper type to store the unmarshaled YAML value.
type loggingReceiverWrapper struct {
	inner interface{}
}

func (l *loggingReceiverWrapper) UnmarshalYAML(ctx context.Context, unmarshal func(interface{}) error) error {
	return loggingReceiverTypes.unmarshalComponentYaml(ctx, &l.inner, unmarshal)
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
	// Components returns fluentbit components that implement this procesor.
	// tag is the log tag that should be matched by those components, and uid is a string which should be used when needed to generate unique names.
	Components(tag string, uid string) []fluentbit.Component
}

var loggingProcessorTypes = &componentTypeRegistry{
	Subagent: "logging", Kind: "processor",
}

// Wrapper type to store the unmarshaled YAML value.
type loggingProcessorWrapper struct {
	inner interface{}
}

func (l *loggingProcessorWrapper) UnmarshalYAML(ctx context.Context, unmarshal func(interface{}) error) error {
	return loggingProcessorTypes.unmarshalComponentYaml(ctx, &l.inner, unmarshal)
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

type LoggingService struct {
	Pipelines map[string]*LoggingPipeline `validate:"dive,keys,startsnotwith=lib:"`
}

type LoggingPipeline struct {
	ReceiverIDs  []string `yaml:"receivers,omitempty,flow"`
	ProcessorIDs []string `yaml:"processors,omitempty,flow"`
	// ExporterIDs is deprecated and ignored.
	ExporterIDs []string `yaml:"exporters,omitempty,flow"`
}

// Ops Agent metrics config.
type metricsReceiverMap map[string]MetricsReceiver
type metricsProcessorMap map[string]MetricsProcessor
type Metrics struct {
	Receivers  metricsReceiverMap  `yaml:"receivers" validate:"dive,keys,startsnotwith=lib:"`
	Processors metricsProcessorMap `yaml:"processors" validate:"dive,keys,startsnotwith=lib:"`
	// Exporters are deprecated and ignored, so do not have any validation.
	Exporters map[string]interface{} `yaml:"exporters,omitempty"`
	Service   *MetricsService        `yaml:"service"`
}

type MetricsReceiver interface {
	component
	Pipelines() []otel.Pipeline
}

type MetricsReceiverShared struct {
	CollectionInterval string `yaml:"collection_interval" validate:"required,duration=10s"` // time.Duration format
}

func (m MetricsReceiverShared) CollectionIntervalString() string {
	// TODO: Remove when https://github.com/goccy/go-yaml/pull/246 is merged
	if m.CollectionInterval != "" {
		return m.CollectionInterval
	}
	return "60s"
}

var metricsReceiverTypes = &componentTypeRegistry{
	Subagent: "metrics", Kind: "receiver",
}

// Wrapper type to store the unmarshaled YAML value.
type metricsReceiverWrapper struct {
	inner interface{}
}

func (m *metricsReceiverWrapper) UnmarshalYAML(ctx context.Context, unmarshal func(interface{}) error) error {
	return metricsReceiverTypes.unmarshalComponentYaml(ctx, &m.inner, unmarshal)
}

func (m *metricsReceiverMap) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// Unmarshal into a temporary map to capture types.
	tm := map[string]metricsReceiverWrapper{}
	if err := unmarshal(&tm); err != nil {
		return err
	}
	// Unwrap the structs.
	*m = metricsReceiverMap{}
	for k, r := range tm {
		if r.inner == nil {
			return fmt.Errorf("unknown type for receiver %q", k) // TODO: better error
		}
		(*m)[k] = r.inner.(MetricsReceiver)
	}
	return nil
}

type MetricsProcessor interface {
	component
	Processors() []otel.Component
}

var metricsProcessorTypes = &componentTypeRegistry{
	Subagent: "metrics", Kind: "processor",
}

// Wrapper type to store the unmarshaled YAML value.
type metricsProcessorWrapper struct {
	inner interface{}
}

func (m *metricsProcessorWrapper) UnmarshalYAML(ctx context.Context, unmarshal func(interface{}) error) error {
	return metricsProcessorTypes.unmarshalComponentYaml(ctx, &m.inner, unmarshal)
}

func (m *metricsProcessorMap) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// Unmarshal into a temporary map to capture types.
	tm := map[string]metricsProcessorWrapper{}
	if err := unmarshal(&tm); err != nil {
		return err
	}
	// Unwrap the structs.
	*m = metricsProcessorMap{}
	for k, r := range tm {
		(*m)[k] = r.inner.(MetricsProcessor)
	}
	return nil
}

type MetricsService struct {
	Pipelines map[string]*MetricsPipeline `yaml:"pipelines" validate:"dive,keys,startsnotwith=lib:"`
}

type MetricsPipeline struct {
	ReceiverIDs  []string `yaml:"receivers,flow"`
	ProcessorIDs []string `yaml:"processors,flow"`
	// ExporterIDs is deprecated and ignored.
	ExporterIDs []string `yaml:"exporters,omitempty,flow"`
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
	if len(l.Exporters) > 0 {
		log.Print(`The "logging.exporters" field is no longer needed and will be ignored. This does not change any functionality. Please remove it from your configuration.`)
	}
	if l.Service == nil {
		return nil
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
	if len(m.Exporters) > 0 {
		log.Print(`The "metrics.exporters" field is deprecated and will be ignored. Please remove it from your configuration.`)
	}
	if m.Service == nil {
		return nil
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

var (
	defaultProcessors = []string{
		"lib:apache", "lib:apache2", "lib:apache_error", "lib:mongodb",
		"lib:nginx", "lib:syslog-rfc3164", "lib:syslog-rfc5424"}

	componentTypeLimits = map[string]int{
		"google_cloud_monitoring": 1,
		"hostmetrics":             1,
		"iis":                     1,
		"mssql":                   1,
	}
)

// mapKeys returns keys from a map[string]Any as a map[string]bool.
func mapKeys(m interface{}) map[string]bool {
	keys := map[string]bool{}
	switch m := m.(type) {
	case loggingReceiverMap:
		for k := range m {
			keys[k] = true
		}
	case map[string]LoggingProcessor:
		for k := range m {
			keys[k] = true
		}
	case map[string]*LoggingPipeline:
		for k := range m {
			keys[k] = true
		}
	case metricsReceiverMap:
		for k := range m {
			keys[k] = true
		}
	case metricsProcessorMap:
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
// componentType is the type of the receiver or processor, e.g. "hostmetrics".
// parameter is name of the parameter.
func parameterErrorPrefix(subagent string, kind string, id string, componentType string, parameter string) string {
	return fmt.Sprintf(`parameter %q in %q type %s %s %q`, parameter, componentType, subagent, kind, id)
}

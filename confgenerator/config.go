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
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/filter"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
	"github.com/GoogleCloudPlatform/ops-agent/internal/set"
	"github.com/go-playground/validator/v10"
	yaml "github.com/goccy/go-yaml"
	"github.com/kardianos/osext"
	promconfig "github.com/prometheus/prometheus/config"
	"golang.org/x/exp/constraints"
)

// Ops Agent config.
type UnifiedConfig struct {
	Combined *Combined `yaml:"combined,omitempty"`
	Logging  *Logging  `yaml:"logging"`
	Metrics  *Metrics  `yaml:"metrics"`
	// FIXME: OTel uses metrics/logs/traces but we appear to be using metrics/logging/traces
	Traces *Traces `yaml:"traces,omitempty"`
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

type Combined struct {
	Receivers combinedReceiverMap `yaml:"receivers,omitempty" validate:"dive,keys,startsnotwith=lib:"`
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
	sort.Strings(out)
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

func (ve validationError) Error() string {
	switch ve.Tag() {
	case "duration":
		return fmt.Sprintf("%q must be a duration of at least %s", ve.Field(), ve.Param())
	case "endswith":
		return fmt.Sprintf("%q must end with %q", ve.Field(), ve.Param())
	case "experimental":
		return experimentalValidationErrorString(ve)
	case "ip":
		return fmt.Sprintf("%q must be an IP address", ve.Field())
	case "min":
		return fmt.Sprintf("%q must be a minimum of %s", ve.Field(), ve.Param())
	case "multipleof_time":
		return fmt.Sprintf("%q must be a multiple of %s", ve.Field(), ve.Param())
	case "oneof":
		return fmt.Sprintf("%q must be one of [%s]", ve.Field(), ve.Param())
	case "required":
		return fmt.Sprintf("%q is a required field", ve.Field())
	case "required_with":
		return fmt.Sprintf("%q is required when %q is set", ve.Field(), ve.Param())
	case "startsnotwith":
		return fmt.Sprintf("%q must not start with %q", ve.Field(), ve.Param())
	case "startswith":
		return fmt.Sprintf("%q must start with %q", ve.Field(), ve.Param())
	case "url":
		return fmt.Sprintf("%q must be a URL", ve.Field())
	case "excluded_with":
		return fmt.Sprintf("%q cannot be set if one of [%s] is set", ve.Field(), ve.Param())
	case "filter":
		_, err := filter.NewFilter(ve.Value().(string))
		return fmt.Sprintf("%q: %v", ve.Field(), err)
	case "field":
		_, err := filter.NewMember(ve.Value().(string))
		return fmt.Sprintf("%q: %v", ve.Field(), err)
	case "distinctfield":
		return fmt.Sprintf("%q specified multiple times", ve.Value().(string))
	case "writablefield":
		return fmt.Sprintf("%q is not a writable field", ve.Value().(string))
	}

	return ve.FieldError.Error()
}

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
	// duration validates that the value is a valid duration and >= the parameter
	v.RegisterValidation("duration", func(fl validator.FieldLevel) bool {
		fieldStr := fl.Field().String()
		if fieldStr == "" {
			// Ignore the case where this field is not actually specified or is left empty.
			return true
		}
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
	v.RegisterStructValidation(validatePrometheusConfig, &promconfig.Config{})
	// filter validates that a Cloud Logging filter condition is valid
	v.RegisterValidation("filter", func(fl validator.FieldLevel) bool {
		_, err := filter.NewFilter(fl.Field().String())
		return err == nil
	})
	// field validates that a Cloud Logging field expression is valid
	v.RegisterValidation("field", func(fl validator.FieldLevel) bool {
		_, err := filter.NewMember(fl.Field().String())
		// TODO: Disallow specific target fields?
		return err == nil
	})
	// distinctfield validates that a key in a map refers to different fields from the other keys in the map.
	// Use this as keys,distinctfield,endkeys
	v.RegisterValidation("distinctfield", func(fl validator.FieldLevel) bool {
		// Get the map that contains this key.
		parent, parentkind, found := fl.GetStructFieldOKAdvanced(fl.Parent(), fl.StructFieldName()[:strings.Index(fl.StructFieldName(), "[")])
		if !found {
			return false
		}
		if parentkind != reflect.Map {
			fmt.Printf("not map\n")
			return false
		}
		k1 := fl.Field().String()
		field, err := filter.NewMember(k1)
		if err != nil {
			fmt.Printf("newmember %q: %v", fl.Field().String(), err)
			return false
		}
		for _, key := range parent.MapKeys() {
			k2 := key.String()
			if k1 == k2 {
				// Skip itself
				continue
			}
			field2, err := filter.NewMember(k2)
			if err != nil {
				continue
			}
			if field2.Equals(*field) {
				return false
			}
		}
		return true
	})
	// writablefield checks to make sure the field is writable
	v.RegisterValidation("writablefield", func(fl validator.FieldLevel) bool {
		m1, err := filter.NewMember(fl.Field().String())
		if err != nil {
			// The "field" validator will handle this better.
			return true
		}
		// Currently, instrumentation_source is the only field that is not writable.
		m2, err := filter.NewMember(InstrumentationSourceLabel)
		if err != nil {
			panic(err)
		}
		return !m2.Equals(*m1)
	})
	// multipleof_time validates that the value duration is a multiple of the parameter
	v.RegisterValidation("multipleof_time", func(fl validator.FieldLevel) bool {
		t, ok := fl.Field().Interface().(time.Duration)
		if !ok {
			panic(fmt.Sprintf("multipleof_time: could not convert %s to time duration", fl.Field().String()))
		}
		tfactor, err := time.ParseDuration(fl.Param())
		if err != nil {
			panic(fmt.Sprintf("multipleof_time: could not convert %s to time duration", fl.Param()))
		}
		return t%tfactor == 0
	})
	// Validates that experimental config components are enabled via EXPERIMENTAL_FEATURES
	registerExperimentalValidations(v)
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
		return UnifiedConfig{}, err
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

type Component interface {
	// Type returns the component type string as used in the configuration file (e.g. "hostmetrics")
	Type() string
}

// ConfigComponent holds the shared configuration fields that all components have.
// It is also used by itself when unmarshaling a component's configuration.
type ConfigComponent struct {
	Type string `yaml:"type" validate:"required" tracking:""`
}

type componentInterface interface {
	Component
}

// componentFactory is the value type for the componentTypeRegistry map.
type componentFactory[CI componentInterface] struct {
	// constructor creates a concrete instance for this component. For example, the "files" constructor would return a *LoggingReceiverFiles, which has an IncludePaths field.
	constructor func() CI
	// platforms is a list of platforms on which the component is valid, or any platform if the slice is empty.
	platforms []string
}

func (ct componentFactory[CI]) supportsPlatform(ctx context.Context) bool {
	platform := ctx.Value(platformKey).(string)
	for _, v := range ct.platforms {
		if v == platform {
			return true
		}
	}
	return len(ct.platforms) == 0
}

type componentTypeRegistry[CI componentInterface, M ~map[string]CI] struct {
	// Subagent is "logging" or "metric" (only used for error messages)
	Subagent string
	// Kind is "receiver" or "processor" (only used for error messages)
	Kind string
	// TypeMap contains a map of component "type" string as used in the configuration file to information about that component.
	TypeMap map[string]*componentFactory[CI]
}

func (r *componentTypeRegistry[CI, M]) RegisterType(constructor func() CI, platforms ...string) {
	name := constructor().Type()
	if _, ok := r.TypeMap[name]; ok {
		panic(fmt.Sprintf("attempt to register duplicate %s %s type: %q", r.Subagent, r.Kind, name))
	}
	if r.TypeMap == nil {
		r.TypeMap = make(map[string]*componentFactory[CI])
	}
	r.TypeMap[name] = &componentFactory[CI]{constructor, platforms}
}

// unmarshalComponentYaml is the custom unmarshaller for reading a component's configuration from the config file.
// It first unmarshals into a struct containing only the "type" field, then looks up the config struct with the full set of fields for that type, and finally unmarshals into an instance of that struct.
func (r *componentTypeRegistry[CI, M]) unmarshalComponentYaml(ctx context.Context, inner *CI, unmarshal func(interface{}) error) error {
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
	*inner = o.(CI)
	return unmarshal(*inner)
}

// GetComponentsFromRegistry returns all components that belong to the associated registry
func (r *componentTypeRegistry[CI, M]) GetComponentsFromRegistry() []Component {
	components := make([]Component, len(r.TypeMap))
	i := 0
	for _, comp := range r.TypeMap {
		components[i] = comp.constructor()
		i++
	}
	return components
}

// unmarshalValue is a bogus unmarshalling destination that just captures the unmarshal() function pointer for later reuse.
type unmarshalValue struct {
	unmarshal func(interface{}) error
}

func (v *unmarshalValue) UnmarshalYAML(ctx context.Context, unmarshal func(interface{}) error) error {
	v.unmarshal = unmarshal
	return nil
}

type unmarshalMap map[string]unmarshalValue

// unmarshalToMap unmarshals a YAML structure to a config map.
// It should be called from UnmarshalYAML() on a concrete type.
// N.B. The map type itself can't be generic because it needs to point to a specific type registry, not just a type registry type (whew).
func (r *componentTypeRegistry[CI, M]) unmarshalToMap(ctx context.Context, m *M, unmarshal func(interface{}) error) error {
	if *m == nil {
		*m = make(M)
	}
	// Step 1: Capture unmarshal functions for each component
	um := unmarshalMap{}
	if err := unmarshal(&um); err != nil {
		return err
	}
	// Step 2: Unmarshal into the destination map
	for k, u := range um {
		var inner CI
		if err := r.unmarshalComponentYaml(ctx, &inner, u.unmarshal); err != nil {
			return err
		}
		(*m)[k] = inner
	}
	return nil
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
	Component
	Components(tag string) []fluentbit.Component
}

var LoggingReceiverTypes = &componentTypeRegistry[LoggingReceiver, loggingReceiverMap]{
	Subagent: "logging", Kind: "receiver",
}

func (m *loggingReceiverMap) UnmarshalYAML(ctx context.Context, unmarshal func(interface{}) error) error {
	return LoggingReceiverTypes.unmarshalToMap(ctx, m, unmarshal)
}

// Logging receivers that listen on a port of the host
type LoggingNetworkReceiver interface {
	LoggingReceiver
	GetListenPort() uint16
}

type LoggingProcessor interface {
	Component
	// Components returns fluentbit components that implement this processor.
	// tag is the log tag that should be matched by those components, and uid is a string which should be used when needed to generate unique names.
	Components(tag string, uid string) []fluentbit.Component
}

var LoggingProcessorTypes = &componentTypeRegistry[LoggingProcessor, loggingProcessorMap]{
	Subagent: "logging", Kind: "processor",
}

func (m *loggingProcessorMap) UnmarshalYAML(ctx context.Context, unmarshal func(interface{}) error) error {
	return LoggingProcessorTypes.unmarshalToMap(ctx, m, unmarshal)
}

type LoggingService struct {
	LogLevel  string               `yaml:"log_level,omitempty" validate:"omitempty,oneof=error warn info debug trace"`
	Pipelines map[string]*Pipeline `validate:"dive,keys,startsnotwith=lib:"`
}

type Pipeline struct {
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

type OTelReceiver interface {
	Component
	Pipelines() []otel.ReceiverPipeline
}

type MetricsReceiver interface {
	OTelReceiver
}

type TracesReceiver interface {
	// TODO: Distinguish from metrics somehow?
	OTelReceiver
}

type MetricsReceiverShared struct {
	CollectionInterval string `yaml:"collection_interval" validate:"duration=10s"` // time.Duration format
}

func (m MetricsReceiverShared) CollectionIntervalString() string {
	// TODO: Remove when https://github.com/goccy/go-yaml/pull/246 is merged
	if m.CollectionInterval != "" {
		return m.CollectionInterval
	}
	return "60s"
}

type MetricsReceiverSharedTLS struct {
	Insecure           *bool  `yaml:"insecure" validate:"omitempty"`
	InsecureSkipVerify *bool  `yaml:"insecure_skip_verify" validate:"omitempty"`
	CertFile           string `yaml:"cert_file" validate:"required_with=KeyFile"`
	KeyFile            string `yaml:"key_file" validate:"required_with=CertFile"`
	CAFile             string `yaml:"ca_file" validate:"omitempty"`
}

func (m MetricsReceiverSharedTLS) TLSConfig(defaultInsecure bool) map[string]interface{} {
	if m.Insecure == nil {
		m.Insecure = &defaultInsecure
	}

	tls := map[string]interface{}{
		"insecure": *m.Insecure,
	}

	if m.InsecureSkipVerify != nil {
		tls["insecure_skip_verify"] = *m.InsecureSkipVerify
	}
	if m.CertFile != "" {
		tls["cert_file"] = m.CertFile
	}
	if m.CAFile != "" {
		tls["ca_file"] = m.CAFile
	}
	if m.KeyFile != "" {
		tls["key_file"] = m.KeyFile
	}

	return tls
}

type MetricsReceiverSharedJVM struct {
	MetricsReceiverShared `yaml:",inline"`

	Endpoint       string   `yaml:"endpoint" validate:"omitempty,hostname_port|startswith=service:jmx:"`
	Username       string   `yaml:"username" validate:"required_with=Password"`
	Password       string   `yaml:"password" validate:"required_with=Username"`
	AdditionalJars []string `yaml:"additional_jars" validate:"omitempty,dive,file"`
}

// WithDefaultEndpoint overrides the MetricReceiverSharedJVM's Endpoint if it is empty.
// It then returns a new MetricReceiverSharedJVM with this change.
func (m MetricsReceiverSharedJVM) WithDefaultEndpoint(defaultEndpoint string) MetricsReceiverSharedJVM {
	if m.Endpoint == "" {
		m.Endpoint = defaultEndpoint
	}

	return m
}

// WithDefaultAdditionalJars overrides the MetricReceiverSharedJVM's AdditionalJars if it is empty.
// It then returns a new MetricReceiverSharedJVM with this change.
func (m MetricsReceiverSharedJVM) WithDefaultAdditionalJars(defaultAdditionalJars ...string) MetricsReceiverSharedJVM {
	if len(m.AdditionalJars) == 0 {
		m.AdditionalJars = defaultAdditionalJars
	}

	return m
}

// ConfigurePipelines sets up a Receiver using the MetricsReceiverSharedJVM and the targetSystem.
// This is used alongside the passed in processors to return a single Pipeline in an array.
func (m MetricsReceiverSharedJVM) ConfigurePipelines(targetSystem string, processors []otel.Component) []otel.ReceiverPipeline {
	jarPath, err := FindJarPath()
	if err != nil {
		log.Printf(`Encountered an error discovering the location of the JMX Metrics Exporter, %v`, err)
	}

	config := map[string]interface{}{
		"target_system":       targetSystem,
		"collection_interval": m.CollectionIntervalString(),
		"endpoint":            m.Endpoint,
		"jar_path":            jarPath,
	}

	if len(m.AdditionalJars) > 0 {
		config["additional_jars"] = m.AdditionalJars
	}

	// Only set the username & password fields if provided
	if m.Username != "" {
		config["username"] = m.Username
	}
	if m.Password != "" {
		config["password"] = m.Password
	}

	return []otel.ReceiverPipeline{{
		Receiver: otel.Component{
			Type:   "jmx",
			Config: config,
		},
		Processors: map[string][]otel.Component{"metrics": processors},
	}}
}

type MetricsReceiverSharedCollectJVM struct {
	CollectJVMMetrics *bool `yaml:"collect_jvm_metrics"`
}

func (m MetricsReceiverSharedCollectJVM) TargetSystemString(targetSystem string) string {
	if m.ShouldCollectJVMMetrics() {
		targetSystem = fmt.Sprintf("%s,%s", targetSystem, "jvm")
	}
	return targetSystem
}

func (m MetricsReceiverSharedCollectJVM) ShouldCollectJVMMetrics() bool {
	return m.CollectJVMMetrics == nil || *m.CollectJVMMetrics
}

var FindJarPath = func() (string, error) {
	jarName := "opentelemetry-java-contrib-jmx-metrics.jar"

	executableDir, err := osext.ExecutableFolder()
	if err != nil {
		return jarName, fmt.Errorf("could not determine binary path for jvm receiver: %w", err)
	}

	// TODO(djaglowski) differentiate behavior via build tags
	if runtime.GOOS != "windows" {
		return filepath.Join(executableDir, "../subagents/opentelemetry-collector/", jarName), nil
	}
	return filepath.Join(executableDir, jarName), nil
}

type MetricsReceiverSharedCluster struct {
	CollectClusterMetrics *bool `yaml:"collect_cluster_metrics" validate:"omitempty"`
}

func (m MetricsReceiverSharedCluster) ShouldCollectClusterMetrics() bool {
	return m.CollectClusterMetrics == nil || *m.CollectClusterMetrics
}

var MetricsReceiverTypes = &componentTypeRegistry[MetricsReceiver, metricsReceiverMap]{
	Subagent: "metrics", Kind: "receiver",
}

func (m *metricsReceiverMap) UnmarshalYAML(ctx context.Context, unmarshal func(interface{}) error) error {
	return MetricsReceiverTypes.unmarshalToMap(ctx, m, unmarshal)
}

type CombinedReceiver interface {
	// TODO: Add more types of signals
	MetricsReceiver
}

var CombinedReceiverTypes = &componentTypeRegistry[CombinedReceiver, combinedReceiverMap]{
	Subagent: "generic", Kind: "receiver",
}

type combinedReceiverMap map[string]CombinedReceiver

func (m *combinedReceiverMap) UnmarshalYAML(ctx context.Context, unmarshal func(interface{}) error) error {
	return CombinedReceiverTypes.unmarshalToMap(ctx, m, unmarshal)
}

type MetricsProcessor interface {
	Component
	Processors() []otel.Component
}

var MetricsProcessorTypes = &componentTypeRegistry[MetricsProcessor, metricsProcessorMap]{
	Subagent: "metrics", Kind: "processor",
}

func (m *metricsProcessorMap) UnmarshalYAML(ctx context.Context, unmarshal func(interface{}) error) error {
	return MetricsProcessorTypes.unmarshalToMap(ctx, m, unmarshal)
}

type MetricsService struct {
	LogLevel  string               `yaml:"log_level,omitempty" validate:"omitempty,oneof=error warn info debug"`
	Pipelines map[string]*Pipeline `yaml:"pipelines" validate:"dive,keys,startsnotwith=lib:"`
}

type Traces struct {
	Service *TracesService `yaml:"service"`
}

type TracesService struct {
	Pipelines map[string]*Pipeline
}

func (uc *UnifiedConfig) Validate(platform string) error {
	if uc.Logging != nil {
		if err := uc.Logging.Validate(platform); err != nil {
			return err
		}
	}
	if uc.Metrics != nil {
		if err := uc.ValidateMetrics(platform); err != nil {
			return err
		}
	}
	if uc.Traces != nil {
		if err := uc.ValidateTraces(platform); err != nil {
			return err
		}
	}
	if uc.Combined != nil {
		if err := uc.ValidateCombined(); err != nil {
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
	portTaken := map[uint16]string{} // port -> receiverId map
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
		if _, err := validateComponentTypeCounts(l.Receivers, p.ReceiverIDs, subagent, "receiver"); err != nil {
			return err
		}
		if _, err := validateComponentTypeCounts(l.Processors, p.ProcessorIDs, subagent, "processor"); err != nil {
			return err
		}
		// portTaken will be modified/updated by the validation function
		if _, err := validateReceiverPorts(portTaken, l.Receivers, p.ReceiverIDs); err != nil {
			return err
		}
		if len(p.ExporterIDs) > 0 {
			log.Printf(`The "logging.service.pipelines.%s.exporters" field is deprecated and will be ignored. Please remove it from your configuration.`, id)
		}
	}
	return nil
}

func (uc *UnifiedConfig) ValidateCombined() error {
	m := uc.Metrics
	t := uc.Traces
	c := uc.Combined
	if c == nil {
		return nil
	}
	for k, _ := range c.Receivers {
		for _, f := range []struct {
			name    string
			missing bool
		}{
			{"metrics", m == nil},
			{"traces", t == nil},
			// TODO: Add "logging" here?
		} {
			if f.missing {
				return fmt.Errorf("combined receiver %q found with no %s section; separate metrics and traces pipelines are required for this receiver, or an empty %s configuration if the data is being intentionally dropped", k, f.name, f.name)
			}
		}
	}
	return nil
}

func (uc *UnifiedConfig) MetricsReceivers() (map[string]MetricsReceiver, error) {
	validReceivers := map[string]MetricsReceiver{}
	for k, v := range uc.Metrics.Receivers {
		validReceivers[k] = v
	}
	if uc.Combined != nil {
		for k, v := range uc.Combined.Receivers {
			if _, ok := uc.Metrics.Receivers[k]; ok {
				return nil, fmt.Errorf("metrics receiver %q has the same name as combined receiver %q", k, k)
			}
			if v, ok := v.(MetricsReceiver); ok {
				validReceivers[k] = v
			}
		}
	}
	return validReceivers, nil
}

func (uc *UnifiedConfig) TracesReceivers() (map[string]TracesReceiver, error) {
	validReceivers := map[string]TracesReceiver{}
	if uc.Combined != nil {
		for k, v := range uc.Combined.Receivers {
			if _, ok := v.(TracesReceiver); ok {
				validReceivers[k] = v
			}
		}
	}
	return validReceivers, nil
}

func (uc *UnifiedConfig) ValidateMetrics(platform string) error {
	m := uc.Metrics
	subagent := "metrics"
	if len(m.Exporters) > 0 {
		log.Print(`The "metrics.exporters" field is deprecated and will be ignored. Please remove it from your configuration.`)
	}
	if m.Service == nil {
		return nil
	}
	receivers, err := uc.MetricsReceivers()
	if err != nil {
		return err
	}
	for _, id := range sortedKeys(m.Service.Pipelines) {
		p := m.Service.Pipelines[id]
		if err := validateComponentKeys(receivers, p.ReceiverIDs, subagent, "receiver", id); err != nil {
			return err
		}
		if err := validateComponentKeys(m.Processors, p.ProcessorIDs, subagent, "processor", id); err != nil {
			return err
		}
		if receiverCounts, err := validateComponentTypeCounts(receivers, p.ReceiverIDs, subagent, "receiver"); err != nil {
			return err
		} else {
			if err := validateIncompatibleJVMReceivers(receiverCounts); err != nil {
				return err
			}

			if err := validateSSLConfig(receivers); err != nil {
				return err
			}
		}

		if _, err := validateComponentTypeCounts(m.Processors, p.ProcessorIDs, subagent, "processor"); err != nil {
			return err
		}

		if len(p.ExporterIDs) > 0 {
			log.Printf(`The "metrics.service.pipelines.%s.exporters" field is deprecated and will be ignored. Please remove it from your configuration.`, id)
		}
	}
	return nil
}

func (uc *UnifiedConfig) ValidateTraces(platform string) error {
	t := uc.Traces
	subagent := "traces"
	if t == nil || t.Service == nil {
		return nil
	}
	receivers, err := uc.TracesReceivers()
	if err != nil {
		return err
	}
	for _, id := range sortedKeys(t.Service.Pipelines) {
		p := t.Service.Pipelines[id]
		if err := validateComponentKeys(receivers, p.ReceiverIDs, subagent, "receiver", id); err != nil {
			return err
		}
		if len(p.ProcessorIDs) > 0 {
			return fmt.Errorf("Traces pipelines do not support processors.")
		}
		if _, err := validateComponentTypeCounts(receivers, p.ReceiverIDs, subagent, "receiver"); err != nil {
			return err
		}

		if len(p.ExporterIDs) > 0 {
			log.Printf(`The "traces.service.pipelines.%s.exporters" field is deprecated and will be ignored. Please remove it from your configuration.`, id)
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

	receiverPortLimits = []string{
		"syslog", "tcp", "fluent_forward",
	}
)

// sortedKeys returns sorted keys from a Set if the Set has a type that can be ordered.
func sortedKeys[K constraints.Ordered, V any](m map[K]V) []K {
	keys := []K{}
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return keys[i] < keys[j]
	})
	return keys
}

func validateComponentKeys[V any](components map[string]V, refs []string, subagent string, kind string, pipeline string) error {
	componentSet := set.FromMapKeys(components)
	for _, componentRef := range refs {
		if !componentSet.Contains(componentRef) {
			return fmt.Errorf("%s %s %q from pipeline %q is not defined.", subagent, kind, componentRef, pipeline)
		}
	}
	return nil
}

func validateComponentTypeCounts[C Component](components map[string]C, refs []string, subagent string, kind string) (map[string]int, error) {
	r := map[string]int{}
	for _, id := range refs {
		c, ok := components[id]
		if !ok {
			continue // Some reserved ids don't map to components.
		}
		t := c.Type()
		r[t] += 1
		if limit, ok := componentTypeLimits[t]; ok && r[t] > limit {
			if limit == 1 {
				return nil, fmt.Errorf("at most one %s %s with type %q is allowed.", subagent, kind, t)
			}
			return nil, fmt.Errorf("at most %d %s %ss with type %q are allowed.", limit, subagent, kind, t)
		}
	}
	return r, nil
}

// Validate that no two receivers are using the same port; adding new port usage to the input map `taken`
func validateReceiverPorts(taken map[uint16]string, components interface{}, pipelineRIDs []string) (map[uint16]string, error) {
	cm := reflect.ValueOf(components)
	for _, pipelineRID := range pipelineRIDs {
		v := cm.MapIndex(reflect.ValueOf(pipelineRID)) // For receivers, ids always exist in the component/receiver lists
		t := v.Interface().(Component).Type()
		for _, limitType := range receiverPortLimits {
			if t == limitType {
				// Since the type of this receiver is in the receiverPortLimits, then this receiver must be a LoggingNetworkReceiver
				port := v.Interface().(LoggingNetworkReceiver).GetListenPort()
				if portRID, ok := taken[port]; ok {
					if portRID == pipelineRID {
						// One network receiver is used by two pipelines
						return nil, fmt.Errorf("logging receiver %s listening on port %d can not be used in two pipelines.", pipelineRID, port)
					} else {
						// Two network receivers are using the same port
						return nil, fmt.Errorf("two logging receivers %s and %s can not listen on the same port %d.", portRID, pipelineRID, port)
					}
				} else {
					// Modifying the input map by adding the port and receiverID of the current pipeline to mark the port as taken
					taken[port] = pipelineRID
				}
			}
		}
	}
	return taken, nil
}

func validateIncompatibleJVMReceivers(typeCounts map[string]int) error {
	jvmReceivers := []string{"jvm", "activemq", "cassandra", "tomcat"}
	jvmReceiverCount := 0
	for _, receiverType := range jvmReceivers {
		jvmReceiverCount += typeCounts[receiverType]
	}

	if jvmReceiverCount > 1 {
		return fmt.Errorf("at most one metrics receiver of JVM types [%s] is allowed: JVM based receivers currently conflict, and only one can be configured", strings.Join(jvmReceivers, ", "))
	}

	return nil
}

func validateSSLConfig(receivers metricsReceiverMap) error {
	for receiverId, receiver := range receivers {
		for _, pipeline := range receiver.Pipelines() {
			if tlsCfg, ok := pipeline.Receiver.Config.(map[string]interface{})["tls"]; ok {
				cfg := tlsCfg.(map[string]interface{})
				// If insecure, no other fields are allowed
				if cfg["insecure"] == true {
					invalidFields := []string{}

					for _, field := range []string{"insecure_skip_verify", "cert_file", "ca_file", "key_file"} {
						if val, ok := cfg[field]; ok && val != "" {
							invalidFields = append(invalidFields, fmt.Sprintf("\"%s\"", field))
						}
					}

					if len(invalidFields) > 0 {
						return fmt.Errorf("%s are not allowed when \"insecure\" is true, which indicates TLS is disabled for receiver \"%s\"", strings.Join(invalidFields, ", "), receiverId)
					}
				}
			}
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

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
	"github.com/go-playground/validator/v10"
	yaml "github.com/goccy/go-yaml"
	"github.com/kardianos/osext"
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

func (ve validationError) Error() string {
	switch ve.Tag() {
	case "duration":
		return fmt.Sprintf("%q must be a duration of at least %s", ve.Field(), ve.Param())
	case "endswith":
		return fmt.Sprintf("%q must end with %q", ve.Field(), ve.Param())
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
	case "filter":
		_, err := filter.NewFilter(ve.Value().(string))
		return fmt.Sprintf("%q: %v", ve.Field(), err)
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
	// filter validates that a Cloud Logging filter condition is valid
	v.RegisterValidation("filter", func(fl validator.FieldLevel) bool {
		_, err := filter.NewFilter(fl.Field().String())
		return err == nil
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
	Type string `yaml:"type" validate:"required"`
}

// componentFactory is the value type for the componentTypeRegistry map.
type componentFactory struct {
	// constructor creates a concrete instance for this component. For example, the "files" constructor would return a *LoggingReceiverFiles, which has an IncludePaths field.
	constructor func() Component
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

func (r *componentTypeRegistry) RegisterType(constructor func() Component, platforms ...string) {
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
	Component
	Components(tag string) []fluentbit.Component
}

var LoggingReceiverTypes = &componentTypeRegistry{
	Subagent: "logging", Kind: "receiver",
}

// Wrapper type to store the unmarshaled YAML value.
type loggingReceiverWrapper struct {
	inner interface{}
}

func (l *loggingReceiverWrapper) UnmarshalYAML(ctx context.Context, unmarshal func(interface{}) error) error {
	return LoggingReceiverTypes.unmarshalComponentYaml(ctx, &l.inner, unmarshal)
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
	Component
	// Components returns fluentbit components that implement this procesor.
	// tag is the log tag that should be matched by those components, and uid is a string which should be used when needed to generate unique names.
	Components(tag string, uid string) []fluentbit.Component
}

var LoggingProcessorTypes = &componentTypeRegistry{
	Subagent: "logging", Kind: "processor",
}

// Wrapper type to store the unmarshaled YAML value.
type loggingProcessorWrapper struct {
	inner interface{}
}

func (l *loggingProcessorWrapper) UnmarshalYAML(ctx context.Context, unmarshal func(interface{}) error) error {
	return LoggingProcessorTypes.unmarshalComponentYaml(ctx, &l.inner, unmarshal)
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
	LogLevel  string                      `yaml:"log_level,omitempty" validate:"omitempty,oneof=error warn info debug trace"`
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
	Component
	Pipelines() []otel.Pipeline
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
	Endpoint string `yaml:"endpoint" validate:"omitempty,hostname_port|startswith=service:jmx:"`
	Username string `yaml:"username" validate:"required_with=Password"`
	Password string `yaml:"password" validate:"required_with=Username"`
}

func (m MetricsReceiverSharedJVM) JVMConfig(targetSystem string, defaultEndpoint string, collectionIntervalString string, processors []otel.Component) []otel.Pipeline {
	if m.Endpoint == "" {
		m.Endpoint = defaultEndpoint
	}

	jarPath, err := FindJarPath()
	if err != nil {
		log.Printf(`Encountered an error discovering the location of the JMX Metrics Exporter, %v`, err)
	}

	config := map[string]interface{}{
		"target_system":       targetSystem,
		"collection_interval": collectionIntervalString,
		"endpoint":            m.Endpoint,
		"jar_path":            jarPath,
	}

	// Only set the username & password fields if provided
	if m.Username != "" {
		config["username"] = m.Username
	}
	if m.Password != "" {
		config["password"] = m.Password
	}

	return []otel.Pipeline{{
		Receiver: otel.Component{
			Type:   "jmx",
			Config: config,
		},
		Processors: processors,
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

var MetricsReceiverTypes = &componentTypeRegistry{
	Subagent: "metrics", Kind: "receiver",
}

// Wrapper type to store the unmarshaled YAML value.
type metricsReceiverWrapper struct {
	inner interface{}
}

func (m *metricsReceiverWrapper) UnmarshalYAML(ctx context.Context, unmarshal func(interface{}) error) error {
	return MetricsReceiverTypes.unmarshalComponentYaml(ctx, &m.inner, unmarshal)
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
	Component
	Processors() []otel.Component
}

var MetricsProcessorTypes = &componentTypeRegistry{
	Subagent: "metrics", Kind: "processor",
}

// Wrapper type to store the unmarshaled YAML value.
type metricsProcessorWrapper struct {
	inner interface{}
}

func (m *metricsProcessorWrapper) UnmarshalYAML(ctx context.Context, unmarshal func(interface{}) error) error {
	return MetricsProcessorTypes.unmarshalComponentYaml(ctx, &m.inner, unmarshal)
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
	LogLevel  string                      `yaml:"log_level,omitempty" validate:"omitempty,oneof=error warn info debug"`
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
		if _, err := validateComponentTypeCounts(l.Receivers, p.ReceiverIDs, subagent, "receiver"); err != nil {
			return err
		}
		if _, err := validateComponentTypeCounts(l.Processors, p.ProcessorIDs, subagent, "processor"); err != nil {
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
		if receiverCounts, err := validateComponentTypeCounts(m.Receivers, p.ReceiverIDs, subagent, "receiver"); err != nil {
			return err
		} else {
			if err := validateIncompatibleJVMReceivers(receiverCounts); err != nil {
				return err
			}

			if err := validateSSLConfig(m.Receivers); err != nil {
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

func validateComponentTypeCounts(components interface{}, refs []string, subagent string, kind string) (map[string]int, error) {
	r := map[string]int{}
	cm := reflect.ValueOf(components)
	for _, id := range refs {
		v := cm.MapIndex(reflect.ValueOf(id))
		if !v.IsValid() {
			continue // Some reserved ids don't map to components.
		}
		t := v.Interface().(Component).Type()
		if _, ok := r[t]; ok {
			r[t] += 1
		} else {
			r[t] = 1
		}
		if limit, ok := componentTypeLimits[t]; ok && r[t] > limit {
			if limit == 1 {
				return nil, fmt.Errorf("at most one %s %s with type %q is allowed.", subagent, kind, t)
			}
			return nil, fmt.Errorf("at most %d %s %ss with type %q are allowed.", limit, subagent, kind, t)
		}
	}
	return r, nil
}

func validateIncompatibleJVMReceivers(typeCounts map[string]int) error {
	jvmReceivers := []string{"jvm", "cassandra", "tomcat"}
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

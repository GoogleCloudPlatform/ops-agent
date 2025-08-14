// Copyright 2021 Google LLC
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
	"errors"
	"fmt"
	"log"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
	"github.com/GoogleCloudPlatform/ops-agent/internal/platform"
	"github.com/go-playground/validator/v10"
	yaml "github.com/goccy/go-yaml"
	commonconfig "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	promconfig "github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery"
	_ "github.com/prometheus/prometheus/discovery/install" // init() of this package registers service discovery impl.
)

const minScrapeInterval = model.Duration(10 * time.Second)

type PrometheusMetrics struct {
	ConfigComponent `yaml:",inline"`

	// The Prometheus receiver is configured via a Prometheus config file.
	// See: https://prometheus.io/docs/prometheus/latest/configuration/configuration/

	// Note that since we use the OTel Prometheus receiver, there is a caveat in the regex
	// capture group syntax. Since the collector configuration supports env variable substitution
	// `$` characters in your prometheus configuration are interpreted as environment
	// variables.  If you want to use $ characters in your prometheus configuration,
	// you must escape them using `$$`.
	PromConfig promconfig.Config `yaml:"config"`
}

func (r PrometheusMetrics) Type() string {
	return "prometheus"
}

func (r PrometheusMetrics) Pipelines(ctx context.Context) ([]otel.ReceiverPipeline, error) {
	resource, err := platform.FromContext(ctx).GetResource()
	if err != nil {
		log.Printf("can't get resource metadata: %v", err)
		return nil, nil
	}
	if resource != nil {
		// Get the resource metadata for the instance we're running on.
		resourceMetadataMap := resource.PrometheusStyleMetadata()
		// Add the resource metadata to the prometheus config.
		for i := range r.PromConfig.ScrapeConfigs {
			// Iterate over the static configs.
			for j := range r.PromConfig.ScrapeConfigs[i].ServiceDiscoveryConfigs {
				staticConfigs := r.PromConfig.ScrapeConfigs[i].ServiceDiscoveryConfigs[j].(discovery.StaticConfig)
				for k := range staticConfigs {
					labels := staticConfigs[k].Labels
					if labels == nil {
						labels = model.LabelSet{}
					}
					for k, v := range resourceMetadataMap {
						// If there are conflicts, the resource metadata should take precedence.
						labels[model.LabelName(k)] = model.LabelValue(v)
					}

					staticConfigs[k].Labels = labels
				}
			}
		}
	}

	exp_otlp_exporter := experimentsFromContext(ctx)["otlp_exporter"]
	metrics_components := []otel.Component{otel.GroupByGMPAttrs_OTTL()}
	if exp_otlp_exporter {
		metrics_components = append(metrics_components, otel.MetricStartTime())
		metrics_components = append(metrics_components, otel.GCPProjectID(resource.ProjectName()))
	}
	return []otel.ReceiverPipeline{{
		Receiver: prometheusToOtelComponent(r),
		Processors: map[string][]otel.Component{
			// Expect metrics, without any additional processing.
			"metrics": metrics_components,
		},
		ExporterTypes: map[string]otel.ExporterType{
			"metrics": otel.GMP,
		},
		ResourceDetectionModes: map[string]otel.ResourceDetectionMode{
			"metrics": otel.None,
		},
	}}, nil
}

// Generate otel components for the prometheus config used. It is the same config except
// we need to escape the $ characters in the regexes.
//
// Note: We copy over the prometheus scrape configs and create new ones so calls to `Pipelines()`
// will return the same result everytime and not change the original prometheus config.
func prometheusToOtelComponent(m PrometheusMetrics) otel.Component {
	copyPromConfig, err := deepCopy(m.PromConfig)
	if err != nil {
		// This should never happen since we already validated the prometheus config.
		panic(fmt.Errorf("failed to deep copy prometheus config: %w", err))
	}

	// Escape the $ characters in the regexes.
	for i := range copyPromConfig.ScrapeConfigs {
		for j := range copyPromConfig.ScrapeConfigs[i].RelabelConfigs {
			rc := copyPromConfig.ScrapeConfigs[i].RelabelConfigs[j]
			rc.Replacement = strings.ReplaceAll(rc.Replacement, "$", "$$")
		}
		for j := range copyPromConfig.ScrapeConfigs[i].MetricRelabelConfigs {
			mrc := copyPromConfig.ScrapeConfigs[i].MetricRelabelConfigs[j]
			mrc.Replacement = strings.ReplaceAll(mrc.Replacement, "$", "$$")
		}
	}

	return otel.Component{
		Type:   "prometheus",
		Config: map[string]interface{}{"config": copyPromConfig},
	}
}

func deepCopy(config promconfig.Config) (promconfig.Config, error) {
	marshalledBytes, err := yaml.MarshalWithOptions(
		config,
		yaml.CustomMarshaler[commonconfig.Secret](func(s commonconfig.Secret) ([]byte, error) {
			return []byte(s), nil
		}),
	)
	if err != nil {
		return promconfig.Config{}, fmt.Errorf("failed to convert Prometheus Config to yaml: %w", err)
	}
	copyConfig := promconfig.Config{}
	if err := yaml.Unmarshal(marshalledBytes, &copyConfig); err != nil {
		return promconfig.Config{}, fmt.Errorf("failed to convert yaml to Prometheus Config: %w", err)
	}

	return copyConfig, nil
}

func validatePrometheusConfig(sl validator.StructLevel) {
	promConfig := sl.Current().Interface().(promconfig.Config)

	// Validate that the Prometheus config is valid.
	if field, err := validatePrometheus(promConfig); err != nil {
		fmt.Printf("Prometheus config validation failed with error: %v", err)
		sl.ReportError(reflect.ValueOf(promConfig), "config", field, err.Error(), "")
	}
}

func checkFile(fn string) error {
	// Nothing set, nothing to error on.
	if fn == "" {
		return nil
	}
	_, err := os.Stat(fn)
	if err != nil {
		// Report that the file could not be found in a platform-agnostic way.
		if os.IsNotExist(err) {
			return fmt.Errorf("file %q does not exist", fn)
		} else {
			return fmt.Errorf("error checking file %q", fn)
		}
	}
	return nil
}

func checkTLSConfig(tlsConfig commonconfig.TLSConfig) error {
	if err := checkFile(tlsConfig.CertFile); err != nil {
		return fmt.Errorf("error checking client cert file %q: %w", tlsConfig.CertFile, err)
	}
	if err := checkFile(tlsConfig.KeyFile); err != nil {
		return fmt.Errorf("error checking client key file %q: %w", tlsConfig.KeyFile, err)
	}
	if len(tlsConfig.CertFile) > 0 && len(tlsConfig.KeyFile) == 0 {
		return fmt.Errorf("client cert file %q specified without client key file", tlsConfig.CertFile)
	}
	if len(tlsConfig.KeyFile) > 0 && len(tlsConfig.CertFile) == 0 {
		return fmt.Errorf("client key file %q specified without client cert file", tlsConfig.KeyFile)
	}
	return nil
}

// validatePrometheus checks the receiver configuration is valid.
func validatePrometheus(promConfig promconfig.Config) (string, error) {
	if len(promConfig.ScrapeConfigs) == 0 {
		return "scrape_config", errors.New("no Prometheus scrape_configs")
	}

	// Reject features that Prometheus supports but that the receiver doesn't support:
	// See:
	// * https://github.com/open-telemetry/opentelemetry-collector/issues/3863
	// * https://github.com/open-telemetry/wg-prometheus/issues/3
	unsupportedFeatures := make([]string, 0, 4)
	if len(promConfig.RemoteWriteConfigs) != 0 {
		unsupportedFeatures = append(unsupportedFeatures, "remote_write")
	}
	if len(promConfig.RemoteReadConfigs) != 0 {
		unsupportedFeatures = append(unsupportedFeatures, "remote_read")
	}
	if len(promConfig.RuleFiles) != 0 {
		unsupportedFeatures = append(unsupportedFeatures, "rule_files")
	}
	if len(promConfig.AlertingConfig.AlertRelabelConfigs) != 0 {
		unsupportedFeatures = append(unsupportedFeatures, "alert_config.relabel_configs")
	}
	if len(promConfig.AlertingConfig.AlertmanagerConfigs) != 0 {
		unsupportedFeatures = append(unsupportedFeatures, "alert_config.alertmanagers")
	}
	if len(unsupportedFeatures) != 0 {
		// Sort the values for deterministic error messages.
		sort.Strings(unsupportedFeatures)
		return strings.Join(unsupportedFeatures, ","), fmt.Errorf("unsupported features:\n\t%s", strings.Join(unsupportedFeatures, "\n\t"))
	}

	for _, sc := range promConfig.ScrapeConfigs {
		if sc.ScrapeInterval < minScrapeInterval {
			sc.ScrapeInterval = minScrapeInterval
			log.Printf("scrape_interval must be at least %v; adjusting to minimum accepted value\n", minScrapeInterval)
		}
		if sc.HonorLabels {
			return "honor_labels", fmt.Errorf("error validating scrape_config for job %v: %v", sc.JobName, "honor_labels is not supported")
		}
		for _, rc := range sc.RelabelConfigs {
			if rc.TargetLabel == "location" || rc.TargetLabel == "namespace" || rc.TargetLabel == "cluster" {
				return "relabel_config", fmt.Errorf("error validating scrape_config for job %v: %v", sc.JobName, "relabel_configs cannot rename location, namespace or cluster")
			}
		}
		for _, rc := range sc.MetricRelabelConfigs {
			if rc.TargetLabel == "__name__" {
				// TODO(#2297): Remove validation after renaming is fixed
				return "metric_relabel_config", fmt.Errorf("error validating scrape_config for job %v: %v", sc.JobName, "metric_relabel_configs cannot rename __name__")
			}
			if rc.TargetLabel == "location" || rc.TargetLabel == "namespace" || rc.TargetLabel == "cluster" {
				return "metric_relabel_config", fmt.Errorf("error validating scrape_config for job %v: %v", sc.JobName, "metric_relabel_configs cannot rename location, namespace or cluster")
			}
		}

		if sc.HTTPClientConfig.Authorization != nil {
			if err := checkFile(sc.HTTPClientConfig.Authorization.CredentialsFile); err != nil {
				return "authorization.credentials_file", fmt.Errorf("error checking authorization credentials file %q: %w", sc.HTTPClientConfig.Authorization.CredentialsFile, err)
			}
		}

		if err := checkTLSConfig(sc.HTTPClientConfig.TLSConfig); err != nil {
			return "tls_config", err
		}

		for _, c := range sc.ServiceDiscoveryConfigs {
			switch c := c.(type) {
			case discovery.StaticConfig:
			default:
				return fmt.Sprintf("%T", c), fmt.Errorf("unsupported service discovery config %T", c)
			}
		}
	}

	return "", nil

}

func init() {
	MetricsReceiverTypes.RegisterType(func() MetricsReceiver { return &PrometheusMetrics{} })
}

// ExtractFeatures returns a list of features that are enabled in the receiver config.
// Must always be a subset of ListAllFeatures().
func (r PrometheusMetrics) ExtractFeatures() ([]CustomFeature, error) {
	customFeatures := make([]CustomFeature, 0)
	customFeatures = append(customFeatures, CustomFeature{
		Key:   []string{"enabled"},
		Value: "true",
	})

	for i := range r.PromConfig.ScrapeConfigs {
		sc := r.PromConfig.ScrapeConfigs[i]

		// Since we only support static_configs, there is only over one service discovery config.
		scTargetGroups := 0
		for _, c := range sc.ServiceDiscoveryConfigs {
			switch c := c.(type) {
			case discovery.StaticConfig:
				scTargetGroups += len(c)
			default:
			}
		}

		trackingMetrics := [][2]string{
			{"scheme", sc.Scheme},
			// The Ops Agent doesn't support honor_labels, so we don't need to track it.
			// {"honor_labels", strconv.FormatBool(sc.HonorLabels)},
			{"honor_timestamps", strconv.FormatBool(sc.HonorTimestamps)},
			{"scrape_interval", sc.ScrapeInterval.String()},
			{"scrape_timeout", sc.ScrapeTimeout.String()},
			{"sample_limit", fmt.Sprintf("%d", sc.SampleLimit)},
			{"label_limit", fmt.Sprintf("%d", sc.LabelLimit)},
			{"label_name_length_limit", fmt.Sprintf("%d", sc.LabelNameLengthLimit)},
			{"label_value_length_limit", fmt.Sprintf("%d", sc.LabelValueLengthLimit)},
			{"body_size_limit", fmt.Sprintf("%d", sc.BodySizeLimit)},
			{"relabel_configs", fmt.Sprintf("%d", len(sc.RelabelConfigs))},
			{"metric_relabel_configs", fmt.Sprintf("%d", len(sc.MetricRelabelConfigs))},
			{"static_config_target_groups", fmt.Sprintf("%d", scTargetGroups)},
		}

		for _, metric := range trackingMetrics {
			if metric[1] == "0" || metric[1] == "false" {
				// Skip metrics with default values.
				continue
			}
			customFeatures = append(customFeatures, CustomFeature{
				Key:   []string{"config", fmt.Sprintf("[%d]", i), "scrape_configs", metric[0]},
				Value: metric[1],
			})
		}
	}
	return customFeatures, nil
}

// ListAllFeatures returns a list of all features that the receiver supports that we track.
func (r PrometheusMetrics) ListAllFeatures() []string {
	return []string{
		"confgenerator.ConfigComponent.Type",
		"config.[].scrape_configs.scheme",
		// The Ops Agent doesn't support honor_labels, so we don't need to track it.
		// "config.[].scrape_configs.honor_labels",
		"config.[].scrape_configs.honor_timestamps",
		"config.[].scrape_configs.scrape_interval",
		"config.[].scrape_configs.scrape_timeout",
		"config.[].scrape_configs.sample_limit",
		"config.[].scrape_configs.label_limit",
		"config.[].scrape_configs.label_name_length_limit",
		"config.[].scrape_configs.label_value_length_limit",
		"config.[].scrape_configs.body_size_limit",
		"config.[].scrape_configs.relabel_configs",
		"config.[].scrape_configs.metric_relabel_configs",
		"config.[].scrape_configs.static_config_target_groups",
	}
}

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
	"errors"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
	"github.com/go-playground/validator/v10"
	commonconfig "github.com/prometheus/common/config"
	promconfig "github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery"
	_ "github.com/prometheus/prometheus/discovery/install" // init() of this package registers service discovery impl.
)

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

func (r PrometheusMetrics) Pipelines() []otel.Pipeline {
	// TODO(b/248268653): Fix the issue we have with the regex capture group syntax.
	return []otel.Pipeline{{
		Receiver: otel.Component{
			Type:   "prometheus",
			Config: map[string]interface{}{"config": r.PromConfig},
		},
	}}
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
	return err
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
		for _, rc := range sc.MetricRelabelConfigs {
			if rc.TargetLabel == "__name__" {
				// TODO(#2297): Remove validation after renaming is fixed
				return "metric_relabel_config", fmt.Errorf("error validating scrapeconfig for job %v: %v", sc.JobName, "metric_relabel_configs cannot rename __name__")
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
	MetricsReceiverTypes.RegisterType(func() Component { return &PrometheusMetrics{} })
}

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

package apps

import (
	"fmt"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
)

type MetricsReceiverVault struct {
	confgenerator.ConfigComponent          `yaml:",inline"`
	confgenerator.MetricsReceiverShared    `yaml:",inline"`
	confgenerator.MetricsReceiverSharedTLS `yaml:",inline"`

	Token       string `yaml:"token"`
	Endpoint    string `yaml:"endpoint" validate:"omitempty,hostname_port"`
	MetricsPath string `yaml:"metrics_path" validate:"omitempty,startswith=/"`
}

const (
	defaultVaultEndpoint = "localhost:8200"
	defaultMetricsPath   = "/v1/sys/metrics"
)

func (r MetricsReceiverVault) Type() string {
	return "vault"
}

func (r MetricsReceiverVault) Pipelines() []otel.Pipeline {
	if r.Endpoint == "" {
		r.Endpoint = defaultVaultEndpoint
	}
	if r.MetricsPath == "" {
		r.MetricsPath = defaultMetricsPath
	}

	tlsConfig := r.TLSConfig(true)

	scrapeConfig := map[string]interface{}{
		"job_name":        "vault",
		"scrape_interval": r.CollectionIntervalString(),
		"metrics_path":    r.MetricsPath,
		"static_configs": []map[string]interface{}{{
			"targets": []string{r.Endpoint},
		}},
	}

	if r.Token != "" {
		scrapeConfig["scheme"] = "https"
		scrapeConfig["authorization"] = map[string]interface{}{
			"credentials": r.Token,
			"type":        "Bearer",
		}
	}
	if tlsConfig["insecure"] == false {
		scrapeConfig["scheme"] = "https"
	}
	delete(tlsConfig, "insecure")
	scrapeConfig["tls_config"] = tlsConfig

	includeMetrics := []string{}

	storageMetricTransforms, newStorageMetricNames := r.addStorageMetrics()
	metricRenewRevokeTransforms, newRenewRevokeNames := r.getRenewRevokeMetricsTransforms()
	metricDetailTransforms, newMetricNames := r.getMetricTransforms()

	includeMetrics = append(includeMetrics, newStorageMetricNames...)
	includeMetrics = append(includeMetrics, newMetricNames...)
	includeMetrics = append(includeMetrics, newRenewRevokeNames...)

	return []otel.Pipeline{{
		Receiver: otel.Component{
			Type: "prometheus",
			Config: map[string]interface{}{
				"config": map[string]interface{}{
					"scrape_configs": []map[string]interface{}{
						scrapeConfig,
					},
				},
			},
		},
		Processors: []otel.Component{
			storageMetricTransforms,
			metricDetailTransforms,
			metricRenewRevokeTransforms,
			otel.MetricsFilter(
				"include",
				"strict",
				includeMetrics...,
			),
			otel.NormalizeSums(),
			otel.MetricsTransform(
				otel.AddPrefix("workload.googleapis.com"),
			),
		},
	}}
}

type metricTransformer struct {
	OldName     string
	NewName     string
	Description string
	Unit        string
}

func (r MetricsReceiverVault) addStorageMetrics() (transforms otel.Component, newMetrics []string) {
	storageLabel := "storage"

	storages := []string{
		"zookeeper",
		"swift",
		"spanner",
		"s3",
		"postgres",
		"mysql",
		"mssql",
		"gcs",
		"etcd",
		"dynamodb",
		"couchdb",
		"consul",
		"cockroachdb",
		"cassandra",
		"azure",
	}

	operations := []string{
		"put",
		"get",
		"delete",
		"list",
	}

	queries := []otel.TransformQuery{}

	for _, operation := range operations {
		operationTimeName := "vault.storage.operation." + operation + ".time"
		operationCountName := "vault.storage.operation." + operation + ".count"
		newMetrics = append(newMetrics, operationTimeName)
		newMetrics = append(newMetrics, operationCountName)
		for _, storage := range storages {
			oldMetric := "vault_" + storage + "_" + operation

			queries = append(queries, otel.SummaryCountValToSum(oldMetric, "cumulative", true))
			queries = append(queries, otel.SummarySumValToSum(oldMetric, "cumulative", true))
			queries = append(queries, otel.SetAttribute(oldMetric+"_count", storageLabel, storage))
			queries = append(queries, otel.SetAttribute(oldMetric+"_sum", storageLabel, storage))

			queries = append(queries, otel.SetName(oldMetric+"_sum", operationTimeName))
			queries = append(queries, otel.SetUnit(operationTimeName, "ms"))
			queries = append(queries, otel.SetDescription(operationTimeName, fmt.Sprintf("The duration of %s operations executed against the storage backend.", operation)))

			queries = append(queries, otel.SetName(oldMetric+"_count", operationCountName))
			queries = append(queries, otel.SetUnit(operationCountName, "{operations}"))
			queries = append(queries, otel.SetDescription(operationCountName, fmt.Sprintf("The amount of %s operations executed against the storage backend.", operation)))
		}
	}

	return otel.TransformationMetrics(queries...), newMetrics
}

func (r MetricsReceiverVault) getRenewRevokeMetricsTransforms() (transform otel.Component, newNames []string) {
	metricTransformers := []metricTransformer{
		{
			OldName:     "vault_expire_revoke",
			NewName:     "vault.token.revoke.time",
			Description: "The average time taken to revoke a token.",
			Unit:        "ms",
		},
		{
			OldName:     "vault_expire_renew",
			NewName:     "vault.token.renew.time",
			Description: "The average time taken to renew a token.",
			Unit:        "ms",
		},
	}

	queries := []otel.TransformQuery{}

	for _, metric := range metricTransformers {
		queries = append(queries, otel.SummarySumValToSum(metric.OldName, "cumulative", true))
		queries = append(queries, otel.SetName(metric.OldName+"_sum", metric.NewName))
		queries = append(queries, otel.SetUnit(metric.NewName, metric.NewName))
		queries = append(queries, otel.SetDescription(metric.NewName, metric.Description))

		newNames = append(newNames, metric.NewName)
	}
	return otel.TransformationMetrics(queries...), newNames
}

func (r MetricsReceiverVault) getMetricTransforms() (transform otel.Component, newNames []string) {
	metricTransformers := []metricTransformer{
		{
			OldName:     "vault_core_in_flight_requests",
			NewName:     "vault.core.request.count",
			Description: "The number of requests handled by the Vault core.",
			Unit:        "{requests}",
		},
		{
			OldName:     "vault_core_leadership_lost",
			NewName:     "vault.core.leader.duration",
			Description: "The average amount of time a core was the leader in high availability mode.",
			Unit:        "ms",
		},
		{
			OldName:     "vault_expire_num_leases",
			NewName:     "vault.token.lease.count",
			Description: "The number of tokens that are leased for eventual expiration.",
			Unit:        "{tokens}",
		},
		{
			OldName:     "vault_audit_log_request_failure",
			NewName:     "vault.audit.request.failed",
			Description: "The number of audit log requests that have failed.",
			Unit:        "{requests}",
		},
		{
			OldName:     "vault_audit_log_response_failure",
			NewName:     "vault.audit.response.failed",
			Description: "The number of audit log responses that have failed.",
			Unit:        "{responses}",
		},
		{
			OldName:     "vault_runtime_sys_bytes",
			NewName:     "vault.memory.usage",
			Description: "The amount of memory used by Vault.",
			Unit:        "bytes",
		},
		{
			OldName:     "vault_token_count",
			NewName:     "vault.token.count",
			Description: "The number of tokens created.",
			Unit:        "{tokens}",
		},
	}

	queries := []otel.TransformQuery{}

	for _, metric := range metricTransformers {
		if metric.OldName != "" {
			newNames = append(newNames, metric.NewName)
			queries = append(queries, otel.SetName(metric.OldName, metric.NewName))
		}
		queries = append(queries, otel.SetDescription(metric.NewName, metric.Description))
		queries = append(queries, otel.SetUnit(metric.NewName, metric.Unit))
	}
	return otel.TransformationMetrics(queries...), newNames
}

func init() {
	confgenerator.MetricsReceiverTypes.RegisterType(func() confgenerator.Component { return &MetricsReceiverVault{} })
}

type LoggingProcessorVaultJson struct {
	confgenerator.ConfigComponent `yaml:",inline"`
}

func (LoggingProcessorVaultJson) Type() string {
	return "vault_audit"
}

func (p LoggingProcessorVaultJson) Components(tag, uid string) []fluentbit.Component {
	c := []fluentbit.Component{}

	// sample log line:
	// {"time":"2022-06-07T20:34:34.392078404Z","type":"request","auth":{"token_type":"default"},"request":{"id":"aa005196-0280-381d-ebeb-1a083bdf5675","operation":"update","namespace":{"id":"root"},"path":"sys/audit/test"}}
	jsonParser := &confgenerator.LoggingProcessorParseJson{
		ParserShared: confgenerator.ParserShared{
			TimeKey:    "time",
			TimeFormat: "%Y-%m-%dT%H:%M:%S.%L%z",
		},
	}

	c = append(c,
		confgenerator.LoggingProcessorModifyFields{
			Fields: map[string]*confgenerator.ModifyField{
				InstrumentationSourceLabel: instrumentationSourceValue(p.Type()),
			},
		}.Components(tag, uid)...,
	)
	c = append(c, jsonParser.Components(tag, uid)...)
	return c
}

type LoggingReceiverVaultAuditJson struct {
	LoggingProcessorVaultJson               `yaml:",inline"`
	confgenerator.LoggingReceiverFilesMixin `yaml:",inline"`
	IncludePaths                            []string `yaml:"include_paths,omitempty" validate:"required"`
}

func (r LoggingReceiverVaultAuditJson) Components(tag string) []fluentbit.Component {
	r.LoggingReceiverFilesMixin.IncludePaths = r.IncludePaths

	r.MultilineRules = []confgenerator.MultilineRule{
		{
			StateName: "start_state",
			NextState: "cont",
			Regex:     `^{.*`,
		},
		{
			StateName: "cont",
			NextState: "cont",
			Regex:     `^(?!{.*)`,
		},
	}

	c := r.LoggingReceiverFilesMixin.Components(tag)
	return append(c, r.LoggingProcessorVaultJson.Components(tag, r.LoggingProcessorVaultJson.Type())...)
}

func init() {
	confgenerator.LoggingReceiverTypes.RegisterType(func() confgenerator.Component { return &LoggingReceiverVaultAuditJson{} })
}

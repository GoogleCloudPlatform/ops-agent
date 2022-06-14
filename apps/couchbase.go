package apps

import (
	"sort"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
)

// MetricsReceiverCouchbase is the struct for ops agent monitoring metrics for couchbase
type MetricsReceiverCouchbase struct {
	confgenerator.ConfigComponent       `yaml:",inline"`
	confgenerator.MetricsReceiverShared `yaml:",inline"`

	Endpoint string `yaml:"endpoint" validate:"omitempty,hostname_port"`
	Username string `yaml:"username" validate:"required"`
	Password string `yaml:"password" validate:"required"`
}

const defaultCouchbaseEndpoint = "localhost:8091"

// Type returns the configuration type key of the couchbase receiver
func (r MetricsReceiverCouchbase) Type() string {
	return "couchbase"
}

// Pipelines will construct the prometheus receiver configuration
func (r MetricsReceiverCouchbase) Pipelines() []otel.Pipeline {
	targets := []string{r.Endpoint}
	if r.Endpoint == "" {
		targets = []string{defaultCouchbaseEndpoint}
	}

	config := map[string]interface{}{
		"config": map[string]interface{}{
			"scrape_configs": []map[string]interface{}{
				{
					"job_name":        r.Type(),
					"scrape_interval": r.CollectionIntervalString(),
					"basic_auth": map[string]interface{}{
						"username": r.Username,
						"password": r.Password,
					},
					"metric_relabel_configs": []map[string]interface{}{
						{
							"source_labels": []string{"__name__"},
							"regex":         "(kv_ops)|(kv_vb_curr_items)|(kv_num_vbuckets)|(kv_total_memory_used_bytes)|(kv_ep_num_value_ejects)|(kv_ep_mem_high_wat)|(kv_ep_mem_low_wat)|(kv_ep_oom_errors)",
							"action":        "keep",
						},
					},
					"static_configs": []map[string]interface{}{
						{
							"targets": targets,
						},
					},
				},
			},
		},
	}
	return []otel.Pipeline{{
		Receiver: otel.Component{
			Type:   "prometheus",
			Config: config,
		},
		Processors: []otel.Component{
			otel.NormalizeSums(),
			// remove prometheus scraping meta-metrics
			otel.MetricsFilter("exclude", "strict",
				"scrape_samples_post_metric_relabeling",
				"scrape_series_added",
				"scrape_duration_seconds",
				"scrape_samples_scraped",
				"up",
			),
			otel.MetricsTransform(
				// renaming from prometheus style to otel style, order is important before workload prefix
				otel.RenameMetric("kv_ops", "couchbase.bucket.operation.count"),
				otel.RenameMetric("kv_vb_curr_items", "couchbase.bucket.item.count"),
				otel.RenameMetric("kv_num_vbuckets", "coucbhase.bucket.vbucket.count"),
				otel.RenameMetric("kv_total_memory_used_bytes", "couchbase.bucket.memory.usage"),
				otel.RenameMetric("kv_ep_num_num_value_ejects", "couchbase.bucket.memoryitem.ejection.count"),
				otel.RenameMetric("kv_ep_tmp_oom_errors", "couchbase.bucket.error.oom.count.recoverable"),
				otel.RenameMetric("kv_ep_oom_errors", "couchbase.bucket.error.oom.count.unrecoverable"),

				// combine metrics
				otel.CombineMetrics(
					`^couchbase\.bucket\.error\.oom\.count\.(?P<error_type>unrecoverable|recoverable)$$`,
					"couchbase.bucket.oom.count",
				),

				otel.UpdateMetric(
					`couchbase.bucket.operation.count`,
					map[string]interface{}{
						"action":           "aggregate_labels",
						"label_set":        []string{"bucket", "op"},
						"aggregation_type": "sum",
					},
				),
				otel.AddPrefix("workload.googleapis.com"),
			),

			otel.TransformationMetrics(
				r.transformMetrics()...,
			),
		},
	}}
}

type couchbaseMetric struct {
	description string
	castToGauge bool
	unit        string
}

var metrics = map[string]couchbaseMetric{
	"workload.googleapis.com/couchbase.bucket.operation.count": {
		description: "Number of operations on the bucket.",
		castToGauge: true,
		unit:        "{operations}",
	},
	"workload.googleapis.com/couchbase.bucket.item.count": {
		description: "Number of items that belong to the bucket.",
		castToGauge: true,
		unit:        "{items}",
	},
	"workload.googleapis.com/couchbase.bucket.vbucket.count": {
		description: "Number of non-resident vBuckets.",
		castToGauge: true,
		unit:        "{vbuckets}",
	},
	"workload.googleapis.com/couchbase.bucket.memory.usage": {
		description: "Usage of total memory available to the bucket.",
		castToGauge: true,
		unit:        "By",
	},
	"workload.googleapis.com/couchbase.bucket.item.ejection.count": {
		description: "Number of item value ejections from memory to disk.",
		castToGauge: true,
		unit:        "{ejections}",
	},
	"workload.googleapis.com/couchbase.bucket.error.oom.count": {
		description: "Number of out of memory errors.",
		castToGauge: true,
		unit:        "{errors}",
	},
	"workload.googleapis.com/couchbase.bucket.memory.high_water_mark.limit": {
		description: "The memory usage at which items will be ejected.",
		unit:        "By",
	},
	"workload.googleapis.com/couchbase.bucket.memory.low_water_mark.limit": {
		description: "The memory usage at which ejections will stop that were previously triggered by a high water mark breach.",
		unit:        "By",
	},
}

func (r MetricsReceiverCouchbase) transformMetrics() []otel.TransformQuery {
	operations := []otel.TransformQuery{}

	// persisting order so config generation is non-random
	keys := []string{}
	for k := range metrics {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, metricName := range keys {
		m := metrics[metricName]
		if m.castToGauge {
			operations = append(operations, otel.ConvertGaugeToSum(metricName))
		}
		operations = append(operations, otel.SetDescription(metricName, m.description), otel.SetUnit(metricName, m.unit))
	}
	return operations
}

func init() {
	confgenerator.MetricsReceiverTypes.RegisterType(func() confgenerator.Component { return &MetricsReceiverCouchbase{} })
}

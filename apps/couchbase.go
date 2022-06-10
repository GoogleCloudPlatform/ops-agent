package apps

import (
	"sort"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
)

// MetricsReceiverCouchbase is the struct for ops agent monitoring metrics for couchbase
type MetricsReceiverCouchbase struct {
	confgenerator.ConfigComponent       `yaml:",inline"`
	confgenerator.MetricsReceiverShared `yaml:",inline"`

	Endpoint string `yaml:"endpoint" validate:"omitempty,hostname_port"`
	Username string `yaml:"username" validate:"required_with=Password"`
	Password string `yaml:"password" validate:"required_with=Username"`
}

const defaultCouchbaseEndpoint = "localhost:8091"

// Type returns the configuration type key of the couchbase receiver
func (r MetricsReceiverCouchbase) Type() string {
	return "couchbase"
}

var metricList = []string{
	"couchbase.bucket.operation.count",
	"couchbase.bucket.item.count",
	"couchbase.bucket.vbucket.count",

	"couchbase.bucket.memory.usage.free",
	"couchbase.bucket.memory.usage.used",
	"couchbase.bucket.memory.usage",

	"couchbase.bucket.memory.high_water_mark.limit",
	"couchbase.bucket.memory.low_water_mark.limit",
	// combines
	"couchbase.bucket.error.oom.count.recoverable",
	"couchbase.bucket.error.oom.count.unrecoverable",
	// into
	"couchbase.bucket.error.oom.count",

	"couchbase.bucket.item.ejection.count",
}

// Pipelines will construct the prometheus receiver configuration
func (r MetricsReceiverCouchbase) Pipelines() []otel.Pipeline {
	config := map[string]interface{}{
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
						"regex": `(kv_ops)|\
								(kv_vb_curr_items)|\
								(kv_num_vbuckets)|\
								(kv_ep_cursor_memory_freed_bytes)|\
								(kv_total_memory_used_bytes)|\
								(kv_ep_num_value_ejects)|\
								(kv_ep_mem_high_wat)|\
								(kv_ep_mem_low_wat)|\
								(kv_ep_tmp_oom_errors)|\
								(kv_ep_oom_errors)`,
						"action": "keep",
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
				otel.AddPrefix("workload.googleapis.com"),

				// renaming from prometheus style to otel style
				otel.RenameMetric("kv_ops", "couchbase.bucket.operation.count"),
				otel.RenameMetric("kv_vb_curr_items", "couchbase.bucket.item.count"),
				otel.RenameMetric("kv_num_vbuckets", "coucbhase.bucket.vbucket.count"),
				otel.RenameMetric("kv_ep_cursor_memory_freed_bytes", "couchbase.bucket.memory.usage.free"),
				otel.RenameMetric("kv_ep_cursor_memory_used_bytes", "couchbase.bucket.memory.usage.used"),
				otel.RenameMetric("kv_ep_num_num_value_ejects", "couchbase.bucket.memoryitem.ejection.count"),
				otel.RenameMetric("kv_ep_tmp_oom_errors", "couchbase.bucket.error.oom.count.recoverable"),
				otel.RenameMetric("kv_ep_oom_errors", "couchbase.bucket.error.oom.count.unrecoverable"),

				// combine metrics
				otel.CombineMetrics(
					`^couchbase\.bucket\.error\.oom\.count\.(?P<error_type>unrecoverable|recoverable)$$`,
					"couchbase.bucket.oom.count",
				),
				otel.CombineMetrics(
					`^couchbase\.bucket\.memory\.usage\.(?P<state>free|used)$$`,
					"couchbase.bucket.memory.usage",
				),
				otel.UpdateMetric(
					`couchbase.bucket.operation.count`,
					map[string]interface{}{
						"action":           "aggregate_labels",
						"label_set":        []string{"bucket", "op"},
						"aggregation_type": "sum",
					},
				),
			),
			otel.TransformationMetrics(
				r.transformMetrics()...,
			),
		},
	}}
}

type couchbaseMetric struct {
	description string
	catToGauge  bool
	unit        string
}

var metrics = map[string]couchbaseMetric{
	"couchbase.bucket.operation.count": {
		description: "Number of operations on the bucket.",
		catToGauge:  true,
		unit:        "{operations}",
	},
	"couchbase.bucket.item.count": {
		description: "Number of items that belong to the bucket.",
		catToGauge:  true,
		unit:        "{items}",
	},
	"couchbase.bucket.vbucket.count": {
		description: "Number of non-resident vBuckets.",
		catToGauge:  true,
		unit:        "{vbuckets}",
	},
	"couchbase.bucket.memory.usage": {
		description: "Usage of total memory available to the bucket.",
		catToGauge:  true,
		unit:        "By",
	},
	"couchbase.bucket.item.ejection.count": {
		description: "Number of item value ejections from memory to disk.",
		catToGauge:  true,
		unit:        "{ejections}",
	},
	"couchbase.bucket.error.oom.count": {
		description: "Number of out of memory errors.",
		catToGauge:  true,
		unit:        "{errors}",
	},
	"couchbase.bucket.memory.high_water_mark.limit": {
		description: "The memory usage at which items will be ejected.",
		unit:        "By",
	},
	"couchbase.bucket.memory.low_water_mark.limit": {
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
		if m.catToGauge {
			operations = append(operations, otel.ConvertGaugeToSum(metricName))
		}
		operations = append(operations, otel.SetDescription(metricName, m.description), otel.SetUnit(metricName, m.unit))
	}
	return operations
}

func init() {
	confgenerator.MetricsReceiverTypes.RegisterType(func() confgenerator.Component { return &MetricsReceiverCouchbase{} })
}

// LoggingReceiverCouchbase is a struct used for generating the fluentbit component for couchbase logs
type LoggingReceiverCouchbase struct {
	confgenerator.ConfigComponent           `yaml:",inline"`
	confgenerator.LoggingReceiverFilesMixin `yaml:",inline" validate:"structonly"`
}

func (lr LoggingReceiverCouchbase) Type() string {
	return "couchbase_general"
}

// Components returns the logging components of the couchbase access logs
func (lr LoggingReceiverCouchbase) Components(tag string) []fluentbit.Component {
	if len(lr.IncludePaths) == 0 {
		lr.IncludePaths = []string{
			"/opt/couchbase/var/lib/couchbase/logs/couchdb.log",
			"/opt/couchbase/var/lib/couchbase/logs/info.log",
			"/opt/couchbase/var/lib/couchbase/logs/debug.log",
			"/opt/couchbase/var/lib/couchbase/logs/error.log",
			"/opt/couchbase/var/lib/couchbase/logs/babysitter.log",
		}
	}
	components := lr.LoggingReceiverFilesMixin.Components(tag)
	return components
}

func init() {
	confgenerator.LoggingReceiverTypes.RegisterType(func() confgenerator.Component { return &LoggingReceiverCouchbase{} })
}

// LoggingProcessorCouchbaseHTTPAccess is a struct that will generate the fluentbit components for the http access logs
type LoggingProcessorCouchbaseHTTPAccess struct {
	confgenerator.LoggingReceiverFilesMixin `yaml:",inline" validate:"structonly"`
}

// Type returns the string for the couchbase http access logs
func (lp LoggingProcessorCouchbaseHTTPAccess) Type() string {
	return "couchbase_http_access"
}

// Components returns the fluentbit components for the http access logs of couchbase
func (lp LoggingProcessorCouchbaseHTTPAccess) Components(tag string) []fluentbit.Component {
	if len(lp.IncludePaths) == 0 {
		lp.IncludePaths = []string{
			"/opt/couchbase/var/lib/couchbase/logs/http_access.log",
			"/opt/couchbase/var/lib/couchbase/logs/http_access_internal.log",
		}
	}
	components := lp.Components(tag)
	return components
}

// LoggingProcessorCouchbaseGOXDCR is a struct that iwll generate the fluentbit components for the goxdcr logs
type LoggingProcessorCouchbaseGOXDCR struct {
	confgenerator.LoggingReceiverFilesMixin `yaml:",inline" validate:"structonly"`
}

// Type returns the type string for the goxdcr logs of couchbase
func (lg LoggingProcessorCouchbaseGOXDCR) Type() string {
	return "couchbase_goxcdr"
}

// Components returns the fluentbit components for the couchbase goxdcr logs
func (lg LoggingProcessorCouchbaseGOXDCR) Components(tag string) []fluentbit.Component {
	if len(lg.IncludePaths) == 0 {
		lg.IncludePaths = []string{
			"/opt/couchbase/var/lib/couchbase/logs/goxdcr.log",
		}
	}
	return lg.LoggingReceiverFilesMixin.Components(tag)
}

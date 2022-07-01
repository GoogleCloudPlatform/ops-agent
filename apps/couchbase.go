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
	return []otel.Pipeline{
		{
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
					otel.RenameMetric("kv_ep_num_num_value_ejects", "couchbase.bucket.item.ejection.count"),
					otel.RenameMetric("kv_ep_tmp_oom_errors", "couchbase.bucket.error.oom.count.recoverable"),
					otel.RenameMetric("kv_ep_oom_errors", "couchbase.bucket.error.oom.count.unrecoverable"),

					// combine OOM metrics
					otel.CombineMetrics(
						`^couchbase\.bucket\.error\.oom\.count\.(?P<error_type>unrecoverable|recoverable)$$`,
						"couchbase.bucket.oom.count",
					),

					// Current transform processor cannot do this
					otel.UpdateMetric(
						"couchbase.bucket.item.ejection.count",
						otel.ToggleScalarDataType,
					),
					otel.UpdateMetric(
						"couchbase.bucket.error.oom.count",
						otel.ToggleScalarDataType,
					),
					otel.UpdateMetric(
						"couchbase.bucket.operation.count",
						otel.ToggleScalarDataType,
					),

					// group by bucket and op
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
				otel.TransformationMetrics(r.transformMetrics()...),
			},
		},
	}
}

type couchbaseMetric struct {
	description string
	castToSum   bool
	unit        string
}

var metrics = map[string]couchbaseMetric{
	"workload.googleapis.com/couchbase.bucket.operation.count": {
		description: "Number of operations on the bucket.",
		castToSum:   true,
		unit:        "{operations}",
	},
	"workload.googleapis.com/couchbase.bucket.item.count": {
		description: "Number of items that belong to the bucket.",
		unit:        "{items}",
	},
	"workload.googleapis.com/couchbase.bucket.vbucket.count": {
		description: "Number of non-resident vBuckets.",
		unit:        "{vbuckets}",
	},
	"workload.googleapis.com/couchbase.bucket.memory.usage": {
		description: "Usage of total memory available to the bucket.",
		unit:        "By",
	},
	"workload.googleapis.com/couchbase.bucket.item.ejection.count": {
		description: "Number of item value ejections from memory to disk.",
		castToSum:   true,
		unit:        "{ejections}",
	},
	"workload.googleapis.com/couchbase.bucket.error.oom.count": {
		description: "Number of out of memory errors.",
		castToSum:   true,
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
		if m.castToSum {
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

// Type returns the string identifier for the general couchbase logs
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
	components = append(components, confgenerator.LoggingProcessorParseMultilineRegex{
		LoggingProcessorParseRegexComplex: confgenerator.LoggingProcessorParseRegexComplex{
			Parsers: []confgenerator.RegexParser{
				{
					Regex: `^\[(?<type>[^:]*):(?<severity>[^,]*),(?<timestamp>\d+-\d+-\d+T\d+:\d+:\d+.\d+Z),(?<node_name>[^:]*):([^:]+):(?<source>[^\]]+)\](?<message>.*)$`,
					Parser: confgenerator.ParserShared{
						TimeKey:    "timestamp",
						TimeFormat: "%Y-%m-%dT%H:%M:%S.%L",
					},
				},
			},
		},
		Rules: []confgenerator.MultilineRule{
			{
				StateName: "start_state",
				NextState: "cont",
				Regex:     `^\[([^\s+:]*):`,
			},
			{
				StateName: "cont",
				NextState: "cont",
				Regex:     `^(?!\[([^\s+:]*):).*$`,
			},
		},
	}.Components(tag, lr.Type())...)

	components = append(components,
		confgenerator.LoggingProcessorModifyFields{
			Fields: map[string]*confgenerator.ModifyField{
				"severity": {
					MoveFrom: "jsonPayload.level",
					MapValues: map[string]string{
						"debug": "DEBUG",
						"info":  "INFO",
						"warn":  "WARNING",
						"error": "ERROR",
					},
					MapValuesExclusive: true,
				},
				InstrumentationSourceLabel: instrumentationSourceValue(lr.Type()),
			},
		}.Components(tag, lr.Type())...)
	return components
}

// LoggingProcessorCouchbaseHTTPAccess is a struct that will generate the fluentbit components for the http access logs
type LoggingProcessorCouchbaseHTTPAccess struct {
	confgenerator.ConfigComponent           `yaml:",inline"`
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
	c := lp.LoggingReceiverFilesMixin.Components(tag)
	c = append(c,
		confgenerator.LoggingProcessorParseRegex{
			Regex: `^(?<client_ip>[^ ]*) [^ ]* (?<user>[^ ]*) \[(?<timestamp>[^\]]*)\] "(?<method>\S+)(?: +(?<path>[^ ]*) +\S*)?" (?<status_code>[^ ]*) (?<response_size>[^ ]*) - (?<message>.*)$`,
			ParserShared: confgenerator.ParserShared{
				TimeKey:    "timestamp",
				TimeFormat: `%d/%b/%Y:%H:%M:%S %z`,
				Types: map[string]string{
					"size": "integer",
					"code": "integer",
				},
			},
		}.Components(tag, lp.Type())...,
	)
	c = append(c, confgenerator.LoggingProcessorModifyFields{
		Fields: map[string]*confgenerator.ModifyField{
			InstrumentationSourceLabel: instrumentationSourceValue(lp.Type()),
		},
	}.Components(tag, lp.Type())...)
	return c
}

// LoggingProcessorCouchbaseGOXDCR is a struct that iwll generate the fluentbit components for the goxdcr logs
type LoggingProcessorCouchbaseGOXDCR struct {
	confgenerator.ConfigComponent           `yaml:",inline"`
	confgenerator.LoggingReceiverFilesMixin `yaml:",inline" validate:"structonly"`
}

// Type returns the type string for the cross datacenter logs of couchbase
func (lg LoggingProcessorCouchbaseGOXDCR) Type() string {
	return "couchbase_goxdcr"
}

// Components returns the fluentbit components for the couchbase goxdcr logs
func (lg LoggingProcessorCouchbaseGOXDCR) Components(tag string) []fluentbit.Component {
	if len(lg.IncludePaths) == 0 {
		lg.IncludePaths = []string{
			"/opt/couchbase/var/lib/couchbase/logs/goxdcr.log",
		}
	}

	c := lg.LoggingReceiverFilesMixin.Components(tag)
	c = append(c, confgenerator.LoggingProcessorParseMultilineRegex{
		LoggingProcessorParseRegexComplex: confgenerator.LoggingProcessorParseRegexComplex{
			Parsers: []confgenerator.RegexParser{
				{
					Regex: `^(?<timestamp>\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}.\d*Z) (?<level>\w+) (?<log_type>\w+.\w+): (?<message>.*)$`,
					Parser: confgenerator.ParserShared{
						TimeKey:    "timestamp",
						TimeFormat: "%Y-%m-%dT%H:%M:%S.%L",
					},
				},
			},
		},
		Rules: []confgenerator.MultilineRule{
			{
				StateName: "start_state",
				NextState: "cont",
				Regex:     `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}`,
			},
			{
				StateName: "cont",
				NextState: "cont",
				Regex:     `^(?!\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})`,
			},
		},
	}.Components(tag, lg.Type())...)
	c = append(c,
		confgenerator.LoggingProcessorModifyFields{
			Fields: map[string]*confgenerator.ModifyField{
				"severity": {
					MoveFrom: "jsonPayload.level",
					MapValues: map[string]string{
						"DEBUG": "DEBUG",
						"INFO":  "INFO",
						"WARN":  "WARNING",
						"ERROR": "ERROR",
					},
					MapValuesExclusive: true,
				},
				InstrumentationSourceLabel: instrumentationSourceValue(lg.Type()),
			},
		}.Components(tag, lg.Type())...,
	)
	return c
}

func init() {
	confgenerator.LoggingReceiverTypes.RegisterType(func() confgenerator.Component { return &LoggingReceiverCouchbase{} })
	confgenerator.LoggingReceiverTypes.RegisterType(func() confgenerator.Component { return &LoggingProcessorCouchbaseHTTPAccess{} })
	confgenerator.LoggingReceiverTypes.RegisterType(func() confgenerator.Component { return &LoggingProcessorCouchbaseGOXDCR{} })
}

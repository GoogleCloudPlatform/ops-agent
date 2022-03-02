package apps

import (
	"fmt"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
)

type MetricsReceiverElasticsearch struct {
	confgenerator.ConfigComponent                 `yaml:",inline"`
	confgenerator.MetricsReceiverShared           `yaml:",inline"`
	confgenerator.MetricsReceiverSharedTLS        `yaml:",inline"`
	confgenerator.MetricsReceiverSharedCollectJVM `yaml:",inline"`
	confgenerator.MetricsReceiverSharedCluster    `yaml:",inline"`

	Endpoint string `yaml:"endpoint" validate:"omitempty,url,startswith=http:|startswith=https:"`

	Username string `yaml:"username" validate:"required_with=Password"`
	Password string `yaml:"password" validate:"required_with=Username"`
}

const (
	defaultElasticsearchEndpoint = "http://localhost:9200"
)

func (r MetricsReceiverElasticsearch) Type() string {
	return "elasticsearch"
}

func (r MetricsReceiverElasticsearch) Pipelines() []otel.Pipeline {
	if r.Endpoint == "" {
		r.Endpoint = defaultElasticsearchEndpoint
	}

	cfg := map[string]interface{}{
		"collection_interval":  r.CollectionIntervalString(),
		"endpoint":             r.Endpoint,
		"username":             r.Username,
		"password":             r.Password,
		"nodes":                []string{"_local"},
		"tls":                  r.TLSConfig(true),
		"skip_cluster_metrics": !r.ShouldCollectClusterMetrics(),
	}

	// Custom logic needed to skip JVM metrics, since JMX receiver is not used here.
	if !r.ShouldCollectJVMMetrics() {
		cfg["metrics"] = r.skipJVMMetricsConfig()
	}

	return []otel.Pipeline{{
		Receiver: otel.Component{
			Type:   "elasticsearch",
			Config: cfg,
		},
		Processors: []otel.Component{
			otel.NormalizeSums(),
			otel.MetricsTransform(
				otel.AddPrefix("workload.googleapis.com"),
			),
		},
	}}
}

func (r MetricsReceiverElasticsearch) skipJVMMetricsConfig() map[string]interface{} {
	jvmMetrics := []string{
		"jvm.classes.loaded",
		"jvm.gc.collections.count",
		"jvm.gc.collections.elapsed",
		"jvm.memory.heap.max",
		"jvm.memory.heap.used",
		"jvm.memory.heap.committed",
		"jvm.memory.nonheap.used",
		"jvm.memory.nonheap.committed",
		"jvm.memory.pool.max",
		"jvm.memory.pool.used",
		"jvm.threads.count",
	}

	conf := map[string]interface{}{}

	for _, metric := range jvmMetrics {
		conf[metric] = map[string]bool{
			"enabled": false,
		}
	}

	return conf
}

func init() {
	confgenerator.MetricsReceiverTypes.RegisterType(func() confgenerator.Component { return &MetricsReceiverElasticsearch{} })
}

type LoggingProcessorElasticsearchJson struct {
	confgenerator.ConfigComponent `yaml:",inline"`
}

func (LoggingProcessorElasticsearchJson) Type() string {
	return "elasticsearch_json"
}

func (p LoggingProcessorElasticsearchJson) Components(tag, uid string) []fluentbit.Component {
	c := []fluentbit.Component{}

	// sample log line:
	// {"type": "server", "timestamp": "2022-01-17T18:31:47,365Z", "level": "INFO", "component": "o.e.n.Node", "cluster.name": "elasticsearch", "node.name": "ubuntu-impish", "message": "initialized" }
	// Logs are formatted based on configuration (log4j);
	// See https://artifacts.elastic.co/javadoc/org/elasticsearch/elasticsearch/7.16.2/org/elasticsearch/common/logging/ESJsonLayout.html
	// for general layout, and https://www.elastic.co/guide/en/elasticsearch/reference/current/logging.html for general configuration of logging

	jsonParser := &confgenerator.LoggingProcessorParseJson{
		ParserShared: confgenerator.ParserShared{
			TimeKey:    "timestamp",
			TimeFormat: "%Y-%m-%dT%H:%M:%S,%L%z",
		},
	}

	c = append(c, jsonParser.Components(tag, uid)...)
	c = append(c, p.severityParser(tag, uid)...)
	c = append(c, p.nestingProcessors(tag, uid)...)

	return c
}

func (p LoggingProcessorElasticsearchJson) severityParser(tag, uid string) []fluentbit.Component {
	severityKey := "logging.googleapis.com/severity"
	return fluentbit.TranslationComponents(tag, "level", severityKey, true, []struct {
		SrcVal  string
		DestVal string
	}{
		{"TRACE", "DEBUG"},
		{"DEBUG", "DEBUG"},
		{"INFO", "INFO"},
		{"WARN", "WARNING"},
		{"DEPRECATION", "WARNING"},
		{"ERROR", "ERROR"},
		{"CRITICAL", "ERROR"},
		{"FATAL", "FATAL"},
	})
}

func (p LoggingProcessorElasticsearchJson) nestingProcessors(tag, uid string) []fluentbit.Component {
	// The majority of these prefixes come from here:
	// https://www.elastic.co/guide/en/elasticsearch/reference/7.16/audit-event-types.html#audit-event-attributes
	// Non-audit logs are formatted using the layout documented here, giving the "cluster" prefix:
	// https://artifacts.elastic.co/javadoc/org/elasticsearch/elasticsearch/7.16.2/org/elasticsearch/common/logging/ESJsonLayout.html
	prefixes := []string{
		"user.run_by",
		"user.run_as",
		"authentication.token",
		"node",
		"event",
		"authentication",
		"user",
		"origin",
		"request",
		"url",
		"host",
		"apikey",
		"cluster",
	}

	c := make([]fluentbit.Component, 0, len(prefixes))
	for _, prefix := range prefixes {
		nestProcessor := confgenerator.LoggingProcessorNestWildcard{
			Wildcard:     fmt.Sprintf("%s.*", prefix),
			NestUnder:    prefix,
			RemovePrefix: fmt.Sprintf("%s.", prefix),
		}
		c = append(c, nestProcessor.Components(tag, uid)...)
	}

	return c
}

type LoggingProcessorElasticsearchGC struct {
	confgenerator.ConfigComponent `yaml:",inline"`
}

func (LoggingProcessorElasticsearchGC) Type() string {
	return "elasticsearch_gc"
}

func (p LoggingProcessorElasticsearchGC) Components(tag, uid string) []fluentbit.Component {
	c := []fluentbit.Component{}

	regexParser := confgenerator.LoggingProcessorParseRegex{
		// Sample log line:
		// [2022-01-17T18:31:37.240+0000][652141][gc,start    ] GC(0) Pause Young (Normal) (G1 Evacuation Pause)
		Regex: `\[(?<time>\d+-\d+-\d+T\d+:\d+:\d+.\d+\+\d+)\]\[\d+\]\[(?<type>[A-z,]+)\s*\]\s*(?:GC\((?<gc_run>\d+)\))?\s*(?<message>.*)`,
		ParserShared: confgenerator.ParserShared{
			TimeKey:    "time",
			TimeFormat: "%Y-%m-%dT%H:%M:%S.%L%z",
			Types: map[string]string{
				"gc_run": "integer",
			},
		},
	}

	c = append(c, regexParser.Components(tag, uid)...)

	return c
}

type LoggingReceiverElasticsearchJson struct {
	LoggingProcessorElasticsearchJson       `yaml:",inline"`
	confgenerator.LoggingReceiverFilesMixin `yaml:",inline"`
}

func (r LoggingReceiverElasticsearchJson) Components(tag string) []fluentbit.Component {
	if len(r.IncludePaths) == 0 {
		// Default JSON logs for Elasticsearch
		r.IncludePaths = []string{
			"/var/log/elasticsearch/*_server.json",
			"/var/log/elasticsearch/*_deprecation.json",
			"/var/log/elasticsearch/*_index_search_slowlog.json",
			"/var/log/elasticsearch/*_index_indexing_slowlog.json",
			"/var/log/elasticsearch/*_audit.json",
		}
	}

	// When Elasticsearch emits stack traces, the json log may be spread across multiple lines,
	// so we need this multiline parsing to properly parse the record.
	// Example multiline log record:
	// {"type": "server", "timestamp": "2022-01-20T15:46:00,131Z", "level": "ERROR", "component": "o.e.b.ElasticsearchUncaughtExceptionHandler", "cluster.name": "elasticsearch", "node.name": "brandon-testing-elasticsearch", "message": "uncaught exception in thread [main]",
	// "stacktrace": ["org.elasticsearch.bootstrap.StartupException: java.lang.IllegalArgumentException: unknown setting [invalid.key] please check that any required plugins are installed, or check the breaking changes documentation for removed settings",
	// -- snip --
	// "at org.elasticsearch.bootstrap.Elasticsearch.init(Elasticsearch.java:166) ~[elasticsearch-7.16.2.jar:7.16.2]",
	// "... 6 more"] }

	r.MultilineRules = []confgenerator.MultilineRule{
		{
			StateName: "start_state",
			NextState: "cont",
			Regex:     `^{.*`,
		},
		{
			StateName: "cont",
			NextState: "cont",
			Regex:     `^[^{].*[,}]$`,
		},
	}

	c := r.LoggingReceiverFilesMixin.Components(tag)
	return append(c, r.LoggingProcessorElasticsearchJson.Components(tag, "elasticsearch_json")...)
}

type LoggingReceiverElasticsearchGC struct {
	LoggingProcessorElasticsearchGC         `yaml:",inline"`
	confgenerator.LoggingReceiverFilesMixin `yaml:",inline"`
}

func (r LoggingReceiverElasticsearchGC) Components(tag string) []fluentbit.Component {
	if len(r.IncludePaths) == 0 {
		// Default GC log for Elasticsearch
		r.IncludePaths = []string{
			"/var/log/elasticsearch/gc.log",
		}
	}

	c := r.LoggingReceiverFilesMixin.Components(tag)
	return append(c, r.LoggingProcessorElasticsearchGC.Components(tag, "elasticsearch_gc")...)
}

func init() {
	confgenerator.LoggingReceiverTypes.RegisterType(func() confgenerator.Component { return &LoggingReceiverElasticsearchJson{} })
	confgenerator.LoggingReceiverTypes.RegisterType(func() confgenerator.Component { return &LoggingReceiverElasticsearchGC{} })
}

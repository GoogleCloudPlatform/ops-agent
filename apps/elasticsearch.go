// Copyright 2022 Google LLC
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
	"context"
	"fmt"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
	"github.com/GoogleCloudPlatform/ops-agent/internal/secret"
)

type MetricsReceiverElasticsearch struct {
	confgenerator.ConfigComponent                 `yaml:",inline"`
	confgenerator.MetricsReceiverShared           `yaml:",inline"`
	confgenerator.MetricsReceiverSharedTLS        `yaml:",inline"`
	confgenerator.MetricsReceiverSharedCollectJVM `yaml:",inline"`
	confgenerator.MetricsReceiverSharedCluster    `yaml:",inline"`

	Endpoint string `yaml:"endpoint" validate:"omitempty,url,startswith=http:|startswith=https:"`

	Username string        `yaml:"username" validate:"required_with=Password"`
	Password secret.String `yaml:"password" validate:"required_with=Username"`
}

const (
	defaultElasticsearchEndpoint = "http://localhost:9200"
)

func (r MetricsReceiverElasticsearch) Type() string {
	return "elasticsearch"
}

func (r MetricsReceiverElasticsearch) Pipelines(_ context.Context) ([]otel.ReceiverPipeline, error) {
	if r.Endpoint == "" {
		r.Endpoint = defaultElasticsearchEndpoint
	}

	metricsConfig := map[string]interface{}{}

	// Custom logic needed to skip JVM metrics, since JMX receiver is not used here.
	if !r.ShouldCollectJVMMetrics() {
		r.skipJVMMetricsConfig(metricsConfig)
	}

	// Custom logic needed to disable index metrics, since they were previously locked behind a feature gate.
	// When support for them is added, this logic can be removed and the index name resource attribute will need
	// to be flattened to the metrics.
	r.disableIndexMetrics(metricsConfig)

	cfg := map[string]interface{}{
		"collection_interval":  r.CollectionIntervalString(),
		"endpoint":             r.Endpoint,
		"username":             r.Username,
		"password":             r.Password.SecretValue(),
		"nodes":                []string{"_local"},
		"tls":                  r.TLSConfig(true),
		"skip_cluster_metrics": !r.ShouldCollectClusterMetrics(),
		"metrics":              metricsConfig,
	}

	return []otel.ReceiverPipeline{{
		Receiver: otel.Component{
			Type:   "elasticsearch",
			Config: cfg,
		},
		Processors: map[string][]otel.Component{"metrics": {
			otel.NormalizeSums(),
			// Elasticsearch Cluster metrics come with a summary JVM heap memory that is not useful && causes DuplicateTimeSeries errors
			otel.MetricsOTTLFilter(
				[]string{
					`name == "jvm.memory.heap.used" and resource.attributes["elasticsearch.node.name"] == nil`,
				},
				[]string{}),
			otel.MetricsTransform(
				otel.AddPrefix("workload.googleapis.com"),
			),
			otel.TransformationMetrics(
				otel.SetScopeName("agent.googleapis.com/"+r.Type()),
				otel.SetScopeVersion("1.0"),
			),
			otel.MetricsRemoveServiceAttributes(),
		}},
	}}, nil
}

func (r MetricsReceiverElasticsearch) skipJVMMetricsConfig(metricsConfig map[string]interface{}) {
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

	for _, metric := range jvmMetrics {
		metricsConfig[metric] = map[string]bool{
			"enabled": false,
		}
	}
}

func (r MetricsReceiverElasticsearch) disableIndexMetrics(metricsConfig map[string]interface{}) {
	indexMetrics := []string{
		"elasticsearch.index.cache.evictions",
		"elasticsearch.index.cache.memory.usage",
		"elasticsearch.index.cache.size",
		"elasticsearch.index.documents",
		"elasticsearch.index.operations.completed",
		"elasticsearch.index.operations.merge.docs_count",
		"elasticsearch.index.operations.merge.size",
		"elasticsearch.index.operations.time",
		"elasticsearch.index.segments.count",
		"elasticsearch.index.segments.memory",
		"elasticsearch.index.segments.size",
		"elasticsearch.index.shards.size",
		"elasticsearch.index.translog.operations",
		"elasticsearch.index.translog.size",
	}

	for _, metric := range indexMetrics {
		metricsConfig[metric] = map[string]bool{
			"enabled": false,
		}
	}
}

func init() {
	confgenerator.MetricsReceiverTypes.RegisterType(func() confgenerator.MetricsReceiver { return &MetricsReceiverElasticsearch{} })
}

type LoggingProcessorMacroElasticsearchJson struct {
}

func (LoggingProcessorMacroElasticsearchJson) Type() string {
	return "elasticsearch_json"
}

func (p LoggingProcessorMacroElasticsearchJson) parseMultilineRegex() confgenerator.LoggingProcessorParseMultilineRegex {
	// When Elasticsearch emits stack traces, the json log may be spread across multiple lines,
	// so we need this multiline parsing to properly parse the record.
	// Example multiline log record:
	// {"type": "server", "timestamp": "2022-01-20T15:46:00,131Z", "level": "ERROR", "component": "o.e.b.ElasticsearchUncaughtExceptionHandler", "cluster.name": "elasticsearch", "node.name": "brandon-testing-elasticsearch", "message": "uncaught exception in thread [main]",
	// "stacktrace": ["org.elasticsearch.bootstrap.StartupException: java.lang.IllegalArgumentException: unknown setting [invalid.key] please check that any required plugins are installed, or check the breaking changes documentation for removed settings",
	// -- snip --
	// "at org.elasticsearch.bootstrap.Elasticsearch.init(Elasticsearch.java:166) ~[elasticsearch-7.16.2.jar:7.16.2]",
	// "... 6 more"] }
	return confgenerator.LoggingProcessorParseMultilineRegex{
		Rules: []confgenerator.MultilineRule{
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
		},
	}
}

func (p LoggingProcessorMacroElasticsearchJson) Expand(ctx context.Context) []confgenerator.InternalLoggingProcessor {
	// sample log line (ES 7.x):
	// {"type": "server", "timestamp": "2022-01-17T18:31:47,365Z", "level": "INFO", "component": "o.e.n.Node", "cluster.name": "elasticsearch", "node.name": "ubuntu-jammy", "message": "initialized" }
	// sample log line (ES 8.x):
	// {"@timestamp":"2025-09-19T02:20:17.920Z", "log.level": "INFO", "message":"initialized", "ecs.version": "1.2.0","service.name":"ES_ECS","event.dataset":"elasticsearch.server","process.thread.name":"main","log.logger":"org.elasticsearch.node.Node","elasticsearch.node.name":"es-test-3","elasticsearch.cluster.name":"elasticsearch"}
	// Logs are formatted based on configuration (log4j);
	// See https://artifacts.elastic.co/javadoc/org/elasticsearch/elasticsearch/7.16.2/org/elasticsearch/common/logging/ESJsonLayout.html
	// for general layout, and https://www.elastic.co/guide/en/elasticsearch/reference/current/logging.html for general configuration of logging
	processors := []confgenerator.InternalLoggingProcessor{
		p.parseMultilineRegex(),

		confgenerator.LoggingProcessorParseJson{},
		confgenerator.LoggingProcessorParseRegex{
			Regex: `(?<temp_time_key>.*)`,
			Field: "timestamp", // Custom ES 7.x format
			ParserShared: confgenerator.ParserShared{
				TimeKey:    "temp_time_key",
				TimeFormat: "%Y-%m-%dT%H:%M:%S,%L%z",
			},
		},
		confgenerator.LoggingProcessorParseRegex{
			Regex: `(?<temp_time_key>.*)`,
			Field: "@timestamp", // ECS format
			ParserShared: confgenerator.ParserShared{
				TimeKey:    "temp_time_key",
				TimeFormat: "%Y-%m-%dT%H:%M:%S.%L%z",
			},
		},
		confgenerator.LoggingProcessorParseRegex{
			Regex:       `(?<temp_level>.*)`,
			Field:       "log.level", // ECS format
			PreserveKey: true,
		},
		confgenerator.LoggingProcessorParseRegex{
			Regex:       `(?<temp_level>.*)`,
			Field:       "level", // Custom ES 7.x format
			PreserveKey: true,
		},

		confgenerator.LoggingProcessorModifyFields{
			Fields: map[string]*confgenerator.ModifyField{
				"severity": {
					MoveFrom: "jsonPayload.temp_level",
					MapValues: map[string]string{
						"TRACE":       "DEBUG",
						"DEBUG":       "DEBUG",
						"INFO":        "INFO",
						"WARN":        "WARNING",
						"DEPRECATION": "WARNING",
						"ERROR":       "ERROR",
						"CRITICAL":    "ERROR",
						"FATAL":       "FATAL",
					},
					MapValuesExclusive: true,
				},
				InstrumentationSourceLabel: instrumentationSourceValue(p.Type()),
			},
		},
	}
	processors = append(processors, p.nestingProcessors()...)
	return processors
}

func (p LoggingProcessorMacroElasticsearchJson) nestingProcessors() []confgenerator.InternalLoggingProcessor {
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
		"log",
	}

	processors := make([]confgenerator.InternalLoggingProcessor, 0, len(prefixes))
	for _, prefix := range prefixes {
		nestProcessor := confgenerator.LoggingProcessorNestWildcard{
			Wildcard:     fmt.Sprintf("%s.*", prefix),
			NestUnder:    prefix,
			RemovePrefix: fmt.Sprintf("%s.", prefix),
		}
		processors = append(processors, nestProcessor)
	}

	return processors
}

type LoggingProcessorMacroElasticsearchGC struct {
}

func (LoggingProcessorMacroElasticsearchGC) Type() string {
	return "elasticsearch_gc"
}

func (p LoggingProcessorMacroElasticsearchGC) Expand(ctx context.Context) []confgenerator.InternalLoggingProcessor {
	return []confgenerator.InternalLoggingProcessor{
		confgenerator.LoggingProcessorParseRegex{
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
		},
		confgenerator.LoggingProcessorModifyFields{
			Fields: map[string]*confgenerator.ModifyField{
				InstrumentationSourceLabel: instrumentationSourceValue(p.Type()),
			},
		},
	}
}

func loggingReceiverFilesMixinElasticsearchJson() confgenerator.LoggingReceiverFilesMixin {
	return confgenerator.LoggingReceiverFilesMixin{
		IncludePaths: []string{
			"/var/log/elasticsearch/*_server.json",
			"/var/log/elasticsearch/*_deprecation.json",
			"/var/log/elasticsearch/*_index_search_slowlog.json",
			"/var/log/elasticsearch/*_index_indexing_slowlog.json",
			"/var/log/elasticsearch/*_audit.json",
		},
	}
}

func loggingReceiverFilesMixinElasticsearchGC() confgenerator.LoggingReceiverFilesMixin {
	return confgenerator.LoggingReceiverFilesMixin{
		IncludePaths: []string{
			"/var/log/elasticsearch/gc.log",
		},
	}
}

func init() {
	confgenerator.RegisterLoggingFilesProcessorMacro[LoggingProcessorMacroElasticsearchJson](loggingReceiverFilesMixinElasticsearchJson)
	confgenerator.RegisterLoggingFilesProcessorMacro[LoggingProcessorMacroElasticsearchGC](loggingReceiverFilesMixinElasticsearchGC)
}

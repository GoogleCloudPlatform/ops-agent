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
	"context"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
)

type MetricsReceiverCassandra struct {
	confgenerator.ConfigComponent `yaml:",inline"`

	confgenerator.MetricsReceiverSharedJVM `yaml:",inline"`

	confgenerator.MetricsReceiverSharedCollectJVM `yaml:",inline"`
}

const defaultCassandraEndpoint = "localhost:7199"

func (r MetricsReceiverCassandra) Type() string {
	return "cassandra"
}

func (r MetricsReceiverCassandra) Pipelines(_ context.Context) ([]otel.ReceiverPipeline, error) {
	targetSystem := "cassandra"

	return r.MetricsReceiverSharedJVM.
		WithDefaultEndpoint(defaultCassandraEndpoint).
		WithServiceName(targetSystem).
		ConfigurePipelines(
			r.TargetSystemString(targetSystem),
			[]otel.Component{
				otel.NormalizeSums(),
				otel.MetricsTransform(
					otel.AddPrefix("workload.googleapis.com"),
				),
				otel.TransformationMetrics(
					otel.SetScopeName("agent.googleapis.com/"+r.Type()),
					otel.SetScopeVersion("1.0"),
				),
			},
		)
}

func init() {
	confgenerator.MetricsReceiverTypes.RegisterType(func() confgenerator.MetricsReceiver { return &MetricsReceiverCassandra{} })
}

type LoggingProcessorMacroCassandraSystem struct {
}

func (LoggingProcessorMacroCassandraSystem) Type() string {
	return "cassandra_system"
}

func (p LoggingProcessorMacroCassandraSystem) Expand(ctx context.Context) []confgenerator.InternalLoggingProcessor {
	return javaLogParsingComponents(p.Type())
}

type LoggingProcessorMacroCassandraDebug struct {
}

func (LoggingProcessorMacroCassandraDebug) Type() string {
	return "cassandra_debug"
}

func (p LoggingProcessorMacroCassandraDebug) Expand(ctx context.Context) []confgenerator.InternalLoggingProcessor {
	return javaLogParsingComponents(p.Type())
}

func javaLogParsingComponents(processorType string) []confgenerator.InternalLoggingProcessor {
	return []confgenerator.InternalLoggingProcessor{
		confgenerator.LoggingProcessorParseMultilineRegex{
			LoggingProcessorParseRegexComplex: confgenerator.LoggingProcessorParseRegexComplex{
				Parsers: []confgenerator.RegexParser{
					{
						// Sample line: INFO [IndexSummaryManager:1] 2021-10-07 12:57:05,003 IndexSummaryRedistribution.java:83 - Redistributing index summaries
						// Sample line: WARN [main] 2021-10-07 11:57:01,602 StartupChecks.java:329 - Maximum number of memory map areas per process (vm.max_map_count) 65530 is too low, recommended value: 1048575, you can change it with sysctl.
						// Sample line: ERROR [MemtablePostFlush:2] 2021-10-05 01:03:35,424 CassandraDaemon.java:579 - Exception in thread Thread[MemtablePostFlush:2,5,main]
						// 				org.apache.cassandra.io.FSReadError: java.io.IOException: Invalid folder descriptor trying to create log replica /folder/views-9786ac1cdd583201a7cdad556410c985
						// 					at org.apache.cassandra.db.lifecycle.LogReplica.create(LogReplica.java:59)
						// 					at org.apache.cassandra.db.lifecycle.LogReplicaSet.maybeCreateReplica(LogReplicaSet.java:87)
						// 					at org.apache.cassandra.db.lifecycle.LogFile.makeAddRecord(LogFile.java:336)
						// 					at org.apache.cassandra.db.lifecycle.LogFile.add(LogFile.java:310)
						Regex: `^(?<level>[A-Z]+)\s+\[(?<module>[^\]]+)\]\s+(?<time>\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2},\d+)\s+(?<message>(?:(?<javaClass>[\w\.]+):(?<lineNumber>\d+))?[\S\s]+)`,
						Parser: confgenerator.ParserShared{
							TimeKey:    "time",
							TimeFormat: "%Y-%m-%d %H:%M:%S,%L",
							Types: map[string]string{
								"lineNumber": "integer",
							},
						},
					},
				},
			},
			Rules: []confgenerator.MultilineRule{
				{
					StateName: "start_state",
					NextState: "cont",
					Regex:     `^[A-Z]+\s+\[[^\]]+\] \d+`,
				},
				{
					StateName: "cont",
					NextState: "cont",
					Regex:     `^(?![A-Z]+\s+\[[^\]]+\] \d+)`,
				},
			},
		},
		// Best documentation found for log levels:
		// https://docs.datastax.com/en/cassandra-oss/3.0/cassandra/configuration/configLoggingLevels.html#Loglevels
		confgenerator.LoggingProcessorModifyFields{
			Fields: map[string]*confgenerator.ModifyField{
				"severity": {
					CopyFrom: "jsonPayload.level",
					MapValues: map[string]string{
						"TRACE": "TRACE",
						"DEBUG": "DEBUG",
						"INFO":  "INFO",
						"ERROR": "ERROR",
						"WARN":  "WARNING",
					},
					MapValuesExclusive: true,
				},
				InstrumentationSourceLabel: instrumentationSourceValue(processorType),
			},
		},
	}
}

type LoggingProcessorMacroCassandraGC struct {
}

func (LoggingProcessorMacroCassandraGC) Type() string {
	return "cassandra_gc"
}

func (p LoggingProcessorMacroCassandraGC) Expand(ctx context.Context) []confgenerator.InternalLoggingProcessor {
	return []confgenerator.InternalLoggingProcessor{
		confgenerator.LoggingProcessorParseMultilineRegex{
			LoggingProcessorParseRegexComplex: confgenerator.LoggingProcessorParseRegexComplex{
				Parsers: []confgenerator.RegexParser{
					{
						// Compatible with Java versions pre-11
						// Vast majority of lines look like the first, with time stopped & time stopping
						// Sample line: 2021-10-02T04:18:28.284+0000: 3.315: Total time for which application threads were stopped: 0.0002390 seconds, Stopping threads took: 0.0000281 seconds
						// Sample line: 2021-10-05T01:20:52.695+0000: 4.434: [GC (CMS Initial Mark) [1 CMS-initial-mark: 0K(3686400K)] 36082K(4055040K), 0.0130057 secs] [Times: user=0.04 sys=0.00, real=0.01 secs]
						// Sample line: 2021-10-05T01:20:52.741+0000: 4.481: [CMS-concurrent-preclean-start]
						// Lines may also contain more detailed GC Heap information in the following lines
						Regex: `^(?<time>\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3,6}(?:Z|[+-]\d{2}:?\d{2})):\s+(?<uptime>\d+\.\d{3,6}):\s+(?<message>(?:Total time for which application threads were stopped: (?<timeStopped>\d+\.\d+) seconds, Stopping threads took: (?<timeStopping>\d+\.\d+)[\s\S]*|[\s\S]+))`,
						Parser: confgenerator.ParserShared{
							TimeKey:    "time",
							TimeFormat: "%Y-%m-%dT%H:%M:%S.%L%z",
							Types: map[string]string{
								"uptime":       "float",
								"timeStopped":  "float",
								"timeStopping": "float",
							},
						},
					},
					{
						// Compatible with Java versions 11+ (see https://bugs.openjdk.org/browse/JDK-8046148)
						// Vast majority of lines look like the first, with time stopped & time stopping
						// Sample line: [2023-05-16T14:51:23.332+0000][4.595s][5195][5217][info ] Total time for which application threads were stopped: 0.0003091 seconds, Stopping threads took: 0.0000181 seconds
						// Sample line: [2023-05-16T14:51:23.332+0000][4.595s][5195][5216][info ] GC(1) Concurrent Abortable Preclean 540.253ms
						// Sample line: [2023-05-16T14:51:23.332+0000][4.595s][5195][5217][info ] Application time: 0.0001203 seconds
						// Lines may also contain more detailed GC Heap information in the following lines
						Regex: `^\[(?<time>\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3,6}(?:Z|[+-]\d{2}:?\d{2}))\]\s?\[(?<uptime>\d+\.\d{3,6})s?\]\s?\[(?<pid>\d+)\]\s?\[(?<tid>\d+)\]\s?\[(?<level>\w+)\s?\]\s?(?<message>(?:Total time for which application threads were stopped: (?<timeStopped>\d+\.\d+) seconds, Stopping threads took: (?<timeStopping>\d+\.\d+)[\s\S]*|[\s\S]+))`,
						Parser: confgenerator.ParserShared{
							TimeKey:    "time",
							TimeFormat: "%Y-%m-%dT%H:%M:%S.%L%z",
							Types: map[string]string{
								"uptime":       "float",
								"pid":          "integer",
								"tid":          "integer",
								"timeStopped":  "float",
								"timeStopping": "float",
							},
						},
					},
				},
			},
			Rules: []confgenerator.MultilineRule{
				{
					StateName: "start_state",
					NextState: "cont",
					Regex:     `^\[?\d{4}-\d{2}-\d{2}`,
				},
				{
					StateName: "cont",
					NextState: "cont",
					Regex:     `^(?!\[?\d{4}-\d{2}-\d{2})`,
				},
			},
		},
		// Java11+ gc logs have severity in the log line
		// https://bugs.openjdk.org/browse/JDK-8046148
		confgenerator.LoggingProcessorModifyFields{
			Fields: map[string]*confgenerator.ModifyField{
				"severity": {
					CopyFrom: "jsonPayload.level",
					MapValues: map[string]string{
						"develop": "TRACE",
						"trace":   "TRACE",
						"debug":   "DEBUG",
						"info":    "INFO",
						"error":   "ERROR",
						"warning": "WARNING",
					},
					MapValuesExclusive: true,
				},
				InstrumentationSourceLabel: instrumentationSourceValue(p.Type()),
			},
		},
	}
}

func loggingReceiverFilesMixinCassandraSystem() confgenerator.LoggingReceiverFilesMixin {
	return confgenerator.LoggingReceiverFilesMixin{
		IncludePaths: []string{
			// Default log file path on Debian / Ubuntu / RHEL / CentOS
			"/var/log/cassandra/system*.log",
			// No default install position / log path for SLES
		},
	}
}

func loggingReceiverFilesMixinCassandraDebug() confgenerator.LoggingReceiverFilesMixin {
	return confgenerator.LoggingReceiverFilesMixin{
		IncludePaths: []string{
			// Default log file path on Debian / Ubuntu / RHEL / CentOS
			"/var/log/cassandra/debug*.log",
			// No default install position / log path for SLES
		},
	}
}

func loggingReceiverFilesMixinCassandraGC() confgenerator.LoggingReceiverFilesMixin {
	return confgenerator.LoggingReceiverFilesMixin{
		IncludePaths: []string{
			// Default log file path on Debian / Ubuntu / RHEL / CentOS for JDK 8
			"/var/log/cassandra/gc.log.*.current",
			// Default log file path on Debian / Ubuntu / RHEL / CentOS for JDK 11
			"/var/log/cassandra/gc.log",
			// No default install position / log path for SLES
		},
	}
}

func init() {
	confgenerator.RegisterLoggingFilesProcessorMacro[LoggingProcessorMacroCassandraSystem](loggingReceiverFilesMixinCassandraSystem)
	confgenerator.RegisterLoggingFilesProcessorMacro[LoggingProcessorMacroCassandraDebug](loggingReceiverFilesMixinCassandraDebug)
	confgenerator.RegisterLoggingFilesProcessorMacro[LoggingProcessorMacroCassandraGC](loggingReceiverFilesMixinCassandraGC)
}

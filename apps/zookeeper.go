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

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
)

func init() {
	confgenerator.MetricsReceiverTypes.RegisterType(func() confgenerator.MetricsReceiver { return &MetricsReceiverZookeeper{} })
}

type MetricsReceiverZookeeper struct {
	confgenerator.ConfigComponent `yaml:",inline"`

	confgenerator.MetricsReceiverShared `yaml:",inline"`

	Endpoint string `yaml:"endpoint" validate:"omitempty,hostname_port"`
}

const defaultZookeeperEndpoint = "localhost:2181"

func (MetricsReceiverZookeeper) Type() string {
	return "zookeeper"
}

func (r MetricsReceiverZookeeper) Pipelines(_ context.Context) ([]otel.ReceiverPipeline, error) {
	if r.Endpoint == "" {
		r.Endpoint = defaultZookeeperEndpoint
	}

	return []otel.ReceiverPipeline{{
		Receiver: otel.Component{
			Type: "zookeeper",
			Config: map[string]interface{}{
				"collection_interval": r.CollectionIntervalString(),
				"endpoint":            r.Endpoint,
			},
		},
		Processors: map[string][]otel.Component{"metrics": {
			otel.NormalizeSums(),
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

type LoggingProcessorMacroZookeeperGeneral struct{}

func (LoggingProcessorMacroZookeeperGeneral) Type() string {
	return "zookeeper_general"
}

func (p LoggingProcessorMacroZookeeperGeneral) Expand(ctx context.Context) []confgenerator.InternalLoggingProcessor {
	return []confgenerator.InternalLoggingProcessor{
		confgenerator.LoggingProcessorParseMultilineRegex{
			LoggingProcessorParseRegexComplex: confgenerator.LoggingProcessorParseRegexComplex{
				Parsers: []confgenerator.RegexParser{
					{
						// Sample log line: 2022-01-31 17:51:45,451 [myid:1] - INFO  [NIOWorkerThread-3:NIOServerCnxn@514] - Processing mntr command from /0:0:0:0:0:0:0:1:50284
						// Sample log line: 2022-02-01 00:46:33,626 [myid:1] - WARN  [SendWorker:2:QuorumCnxManager$SendWorker@1283] - Interrupted while waiting for message on queue
						// Sample log line: java.lang.InterruptedException
						// Sample log line: 	at java.base/java.util.concurrent.locks.AbstractQueuedSynchronizer$ConditionObject.reportInterruptAfterWait(AbstractQueuedSynchronizer.java:2056)
						// Sample log line: 	at java.base/java.util.concurrent.locks.AbstractQueuedSynchronizer$ConditionObject.awaitNanos(AbstractQueuedSynchronizer.java:2133)
						// Sample log line: 	at org.apache.zookeeper.util.CircularBlockingQueue.poll(CircularBlockingQueue.java:105)
						// Sample log line: 	at org.apache.zookeeper.server.quorum.QuorumCnxManager.pollSendQueue(QuorumCnxManager.java:1448)
						// Sample log line: 	at org.apache.zookeeper.server.quorum.QuorumCnxManager.access$900(QuorumCnxManager.java:99)
						// Sample log line: 	at org.apache.zookeeper.server.quorum.QuorumCnxManager$SendWorker.run(QuorumCnxManager.java:1272)
						Regex: `^(?<time>\d{4}-\d{2}-\d{2}\s\d{2}:\d{2}:\d{2},\d{3})\s\[myid:(?<myid>\d+)?\]\s-\s(?<level>\w+)\s+\[(?<thread>.+):(?<source>.+)@(?<line>\d+)\]\s+-\s*(?<message>[\S\s]*)`,
						Parser: confgenerator.ParserShared{
							TimeKey:    "time",
							TimeFormat: "%Y-%m-%d %H:%M:%S,%L",
							Types: map[string]string{
								"myid":   "integer",
								"thread": "string",
								"source": "string",
								"line":   "integer",
							},
						},
					},
					{
						// Sample log line: 2022-01-31 17:51:45,451 - INFO  [NIOWorkerThread-3:NIOServerCnxn@514] - Processing mntr command from /0:0:0:0:0:0:0:1:50284
						// Sample log line: 2022-02-01 00:46:33,626 - WARN  [SendWorker:2:QuorumCnxManager$SendWorker@1283] - Interrupted while waiting for message on queue
						// Sample log line: java.lang.InterruptedException
						// Sample log line: 	at java.base/java.util.concurrent.locks.AbstractQueuedSynchronizer$ConditionObject.reportInterruptAfterWait(AbstractQueuedSynchronizer.java:2056)
						// Sample log line: 	at java.base/java.util.concurrent.locks.AbstractQueuedSynchronizer$ConditionObject.awaitNanos(AbstractQueuedSynchronizer.java:2133)
						// Sample log line: 	at org.apache.zookeeper.util.CircularBlockingQueue.poll(CircularBlockingQueue.java:105)
						// Sample log line: 	at org.apache.zookeeper.server.quorum.QuorumCnxManager.pollSendQueue(QuorumCnxManager.java:1448)
						// Sample log line: 	at org.apache.zookeeper.server.quorum.QuorumCnxManager.access$900(QuorumCnxManager.java:99)
						// Sample log line: 	at org.apache.zookeeper.server.quorum.QuorumCnxManager$SendWorker.run(QuorumCnxManager.java:1272)
						Regex: `^(?<time>\d{4}-\d{2}-\d{2}\s\d{2}:\d{2}:\d{2},\d{3})\s-\s(?<level>\w+)\s+\[(?<thread>.+):(?<source>.+)@(?<line>\d+)\]\s+-\s*(?<message>[\S\s]*)`,
						Parser: confgenerator.ParserShared{
							TimeKey:    "time",
							TimeFormat: "%Y-%m-%d %H:%M:%S,%L",
							Types: map[string]string{
								"thread": "string",
								"source": "string",
								"line":   "integer",
							},
						},
					},
				},
			},
			StartState: `^\d{4}-\d{2}-\d{2}\s\d{2}:\d{2}:\d{2},\d{3}`,
		},
		confgenerator.LoggingProcessorModifyFields{
			Fields: map[string]*confgenerator.ModifyField{
				"severity": {
					CopyFrom: "jsonPayload.level",
					MapValues: map[string]string{
						"TRACE":    "DEBUG",
						"DEBUG":    "DEBUG",
						"INFO":     "INFO",
						"WARN":     "WARNING",
						"ERROR":    "ERROR",
						"CRITICAL": "ERROR",
						"FATAL":    "FATAL",
					},
					MapValuesExclusive: true,
				},
				InstrumentationSourceLabel: instrumentationSourceValue(p.Type()),
			},
		},
	}
}

func loggingReceiverFilesMixinZookeeperGeneral() confgenerator.LoggingReceiverFilesMixin {
	return confgenerator.LoggingReceiverFilesMixin{
		IncludePaths: []string{
			"/opt/zookeeper/logs/zookeeper-*.out",
			"/var/log/zookeeper/zookeeper.log",
		},
	}
}

func init() {
	confgenerator.RegisterLoggingFilesProcessorMacro[LoggingProcessorMacroZookeeperGeneral](loggingReceiverFilesMixinZookeeperGeneral)
}

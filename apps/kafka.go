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
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
)

type MetricsReceiverKafka struct {
	confgenerator.ConfigComponent `yaml:",inline"`

	confgenerator.MetricsReceiverShared           `yaml:",inline"`
	confgenerator.MetricsReceiverSharedJVM        `yaml:",inline"`
	confgenerator.MetricsReceiverSharedCollectJVM `yaml:",inline"`
}

const defaultKafkaEndpoint = "localhost:9999"

func (r MetricsReceiverKafka) Type() string {
	return "kafka"
}

func (r MetricsReceiverKafka) Pipelines() []otel.Pipeline {
	targetSystem := "kafka"
	return r.MetricsReceiverSharedJVM.JVMConfig(
		r.TargetSystemString(targetSystem),
		defaultKafkaEndpoint,
		r.CollectionIntervalString(),
		[]otel.Component{

			// Kafka script contains several metrics not desired by ops-agent
			// as it existed in opentelemetry-java-contrib prior to the
			// development of this integration
			otel.MetricsFilter(
				"include",
				"strict",
				"kafka.message.count",
				"kafka.request.count",
				"kafka.request.failed",
				"kafka.request.time.total",
				"kafka.network.io",
				"kafka.purgatory.size",
				"kafka.partition.count",
				"kafka.partition.offline",
				"kafka.partition.under_replicated",
				"kafka.isr.operation.count",
			),
			otel.NormalizeSums(),
			otel.MetricsTransform(
				otel.AddPrefix("workload.googleapis.com"),
			),
		},
	)
}

func init() {
	confgenerator.MetricsReceiverTypes.RegisterType(func() confgenerator.Component { return &MetricsReceiverKafka{} })
}

type LoggingProcessorKafka struct {
	confgenerator.ConfigComponent `yaml:",inline"`
}

func (LoggingProcessorKafka) Type() string {
	return "kafka"
}

func (p LoggingProcessorKafka) Components(tag string, uid string) []fluentbit.Component {
	c := confgenerator.LoggingProcessorParseMultilineRegex{
		LoggingProcessorParseRegexComplex: confgenerator.LoggingProcessorParseRegexComplex{
			Parsers: []confgenerator.RegexParser{
				{
					// Sample line: [2022-01-26 18:25:20,466] INFO Initiating client connection, connectString=localhost:2181 sessionTimeout=18000 watcher=kafka.zookeeper.ZooKeeperClient$ZooKeeperClientWatcher$@22ff4249 (org.apache.zookeeper.ZooKeeper)
					// Sample line: [2022-01-26 18:25:20,485] INFO [ZooKeeperClient Kafka server] Waiting until connected. (kafka.zookeeper.ZooKeeperClient)
					// Sample line: [2022-02-01 21:34:19,045] INFO [BrokerToControllerChannelManager broker=0 name=alterIsr]: Recorded new controller, from now on will use broker sam-test-kafka.c.otel-agent-dev.internal:9092 (id: 0 rack: null) (kafka.server.BrokerToControllerRequestThread)
					// Sample line: [2022-02-01 21:34:21,230] INFO [ExpirationReaper-0-Produce]: Starting (kafka.server.DelayedOperationPurgatory$ExpiredOperationReaper)
					// Sample line: [2022-02-01 21:34:26,063] INFO [LogLoader partition=quickstart-events-1, dir=/tmp/kafka-logs] Loading producer state till offset 0 with message format version 2 (kafka.log.Log$)
					// Sample line: [2022-01-26 18:25:20,462] INFO Client environment:java.class.path=/opt/kafka/bin/../libs/activation-1.1.1.jar:/opt/kafka/bin/... (org.apache.zookeeper.ZooKeeper)
					// Sample line: [2022-01-26 18:25:21,107] INFO KafkaConfig values:
					// 		advertised.listeners = null
					// 		alter.config.policy.class.name = null
					// 		alter.log.dirs.replication.quota.window.num = 11
					// 		alter.log.dirs.replication.quota.window.size.seconds = 1
					Regex: `^\[(?<time>\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2},\d+)\]\s+(?<level>[A-Z]+)(?:\s+\[(?<source>.*)\]:?)?\s+(?<message>[\s\S]*)(?=\s+\([\w\s\.\$]+\)$|\s+$)(?:\s+\((?<logger>[\w\s\.\$]+)\))?`,
					Parser: confgenerator.ParserShared{
						TimeKey:    "time",
						TimeFormat: "%Y-%m-%d %H:%M:%S,%L",
					},
				},
			},
		},
		Rules: []confgenerator.MultilineRule{
			{
				StateName: "start_state",
				NextState: "cont",
				Regex:     `\[\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2},\d+\]`,
			},
			{
				StateName: "cont",
				NextState: "cont",
				Regex:     `^(?!\[\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2},\d+\])`,
			},
		},
	}.Components(tag, uid)

	// Log levels are just log4j log levels
	// https://logging.apache.org/log4j/2.x/log4j-api/apidocs/org/apache/logging/log4j/Level.html
	c = append(c,
		fluentbit.TranslationComponents(tag, "level", "logging.googleapis.com/severity", true,
			[]struct{ SrcVal, DestVal string }{
				{"TRACE", "TRACE"},
				{"DEBUG", "DEBUG"},
				{"INFO", "INFO"},
				{"ERROR", "ERROR"},
				{"WARN", "WARNING"},
				{"FATAL", "CRITICAL"},
			},
		)...,
	)

	return c
}

type LoggingReceiverKafka struct {
	LoggingProcessorKafka                   `yaml:",inline"`
	confgenerator.LoggingReceiverFilesMixin `yaml:",inline" validate:"structonly"`
}

func (r LoggingReceiverKafka) Components(tag string) []fluentbit.Component {
	if len(r.IncludePaths) == 0 {
		r.IncludePaths = []string{
			// No default package installers, these are common log paths from installs online
			"/var/log/kafka/*.log",
			"/opt/kafka/logs/server.log",
			"/opt/kafka/logs/controller.log",
		}
	}
	c := r.LoggingReceiverFilesMixin.Components(tag)
	c = append(c, r.LoggingProcessorKafka.Components(tag, "kafka")...)
	return c
}

func init() {
	confgenerator.LoggingProcessorTypes.RegisterType(func() confgenerator.Component { return &LoggingProcessorKafka{} })
	confgenerator.LoggingReceiverTypes.RegisterType(func() confgenerator.Component { return &LoggingReceiverKafka{} })
}

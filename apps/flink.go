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
	"errors"
	"fmt"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
)

type MetricsReceiverFlink struct {
	confgenerator.ConfigComponent       `yaml:",inline"`
	confgenerator.MetricsReceiverShared `yaml:",inline"`
	Endpoint                            string `yaml:"endpoint" validate:"omitempty,url,startswith=http:"`
}

func (MetricsReceiverFlink) Type() string {
	return "flink"
}

const defaultFlinkEndpoint = "http://localhost:8081"

func (r MetricsReceiverFlink) Pipelines(_ context.Context) ([]otel.ReceiverPipeline, error) {
	if r.Endpoint == "" {
		r.Endpoint = defaultFlinkEndpoint
	}
	return []otel.ReceiverPipeline{{
		Receiver: otel.Component{
			Type: "flinkmetrics",
			Config: map[string]interface{}{
				"collection_interval": r.CollectionIntervalString(),
				"endpoint":            r.Endpoint,
			},
		},
		Processors: map[string][]otel.Component{"metrics": {
			otel.NormalizeSums(),
			otel.MetricsTransform(
				otel.UpdateMetric("flink.jvm.gc.collections.count", otel.RenameLabel("name", "garbage_collector_name")),
				otel.UpdateMetric("flink.jvm.gc.collections.time", otel.RenameLabel("name", "garbage_collector_name")),
				otel.UpdateMetric("flink.operator.record.count", otel.RenameLabel("name", "operator_name")),
				otel.UpdateMetric("flink.operator.watermark.output", otel.RenameLabel("name", "operator_name")),
				otel.AddPrefix("workload.googleapis.com"),
			),
			otel.TransformationMetrics(
				otel.FlattenResourceAttribute("host.name", "host_name"),
				otel.FlattenResourceAttribute("flink.taskmanager.id", "taskmanager_id"),
				otel.FlattenResourceAttribute("flink.job.name", "job_name"),
				otel.FlattenResourceAttribute("flink.task.name", "task_name"),
				otel.FlattenResourceAttribute("flink.subtask.index", "subtask_index"),
				otel.FlattenResourceAttribute("flink.resource.type", "resource_type"),
			),
			otel.ModifyInstrumentationScope(r.Type(), "1.0"),
		}},
	}}, nil
}

func init() {
	confgenerator.MetricsReceiverTypes.RegisterType(func() confgenerator.MetricsReceiver { return &MetricsReceiverFlink{} })
}

type LoggingProcessorFlink struct {
	confgenerator.ConfigComponent `yaml:",inline"`
}

func (LoggingProcessorFlink) Type() string {
	return "flink"
}

func (p LoggingProcessorFlink) Components(ctx context.Context, tag string, uid string) []fluentbit.Component {
	c := confgenerator.LoggingProcessorParseMultilineRegex{
		LoggingProcessorParseRegexComplex: confgenerator.LoggingProcessorParseRegexComplex{
			Parsers: []confgenerator.RegexParser{
				{
					// Standalone session example
					// Sample line: 2022-04-22 11:51:35,718 INFO  org.apache.flink.runtime.jobmaster.JobMaster                 [] - Close ResourceManager connection 668abb5d496646a153262b5896fd935d: Stopping JobMaster for job 'Streaming WordCount' (2538c8dff66c8cf6ec58ad32b149e23f).

					// Taskexecutor example
					// 2022-04-23 16:13:05,459 INFO  org.apache.flink.runtime.taskexecutor.TaskExecutor           [] - Could not resolve ResourceManager address akka.tcp://flink@localhost:6123/user/rpc/resourcemanager_*, retrying in 10000 ms: Could not connect to rpc endpoint under address akka.tcp://flink@localhost:6123/user/rpc/resourcemanager_*.

					// Client example
					// Sample line: 2022-04-22 11:51:32,901 INFO  org.apache.flink.client.program.rest.RestClusterClient       [] - Submitting job 'Streaming WordCount' (2538c8dff66c8cf6ec58ad32b149e23f).

					Regex: `^(?<time>\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2},\d+)\s+(?<level>[A-Z]+)\s+(?<source>[^ ]*)(?<message>[\s\S]*)`,
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
				Regex:     `^\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2},\d+`,
			},
			{
				StateName: "cont",
				NextState: "cont",
				Regex:     `^(?!\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2},\d+)`,
			},
		},
	}.Components(ctx, tag, uid)

	// Log levels are just log4j log levels
	// https://logging.apache.org/log4j/2.x/log4j-api/apidocs/org/apache/logging/log4j/Level.html
	c = append(c,
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
						"FATAL": "CRITICAL",
					},
					MapValuesExclusive: true,
				},
				InstrumentationSourceLabel: instrumentationSourceValue(p.Type()),
			},
		}.Components(ctx, tag, uid)...,
	)

	return c
}

type LoggingReceiverFlink struct {
	LoggingProcessorFlink                   `yaml:",inline"`
	confgenerator.LoggingReceiverFilesMixin `yaml:",inline" validate:"structonly"`
}

func (r LoggingReceiverFlink) Components(ctx context.Context, tag string) []fluentbit.Component {
	c := r.LoggingReceiverFilesMixin.Components(ctx, tag)
	c = append(c, r.LoggingProcessorFlink.Components(ctx, tag, "flink")...)
	return c
}

type LoggingReceiverProcessor interface {
	Type() string
	Processors(ctx context.Context) []confgenerator.LoggingProcessor
}

type LoggingReceiverMixin interface {
	Components(ctx context.Context, tag string) []fluentbit.Component
}

type LoggingReceiverProcessorReceiver[R LoggingReceiverMixin, P LoggingReceiverProcessor] struct {
	confgenerator.ConfigComponent `yaml:",inline"`
	ReceiverMixin                 R `yaml:",inline"`
	ProcessorMixin                P `yaml:",inline"`
}

func (rpr *LoggingReceiverProcessorReceiver[R, P]) Type() string {
	return rpr.ProcessorMixin.Type()
}

func (rpr *LoggingReceiverProcessorReceiver[R, P]) Components(ctx context.Context, tag string) []fluentbit.Component {
	processors := rpr.ProcessorMixin.Processors(ctx)
	if len(processors) == 1 {
		return processors[0].Components(ctx, tag, rpr.Type())
	}
	var c []fluentbit.Component
	for i, p := range processors {
		c = append(c, p.Components(ctx, tag, fmt.Sprintf("%s.%i", rpr.Type(), i))...)
	}
	return c
}

func (rpr *LoggingReceiverProcessorReceiver[R, P]) Pipelines(ctx context.Context) ([]otel.ReceiverPipeline, error) {
	if r, ok := any(rpr.ReceiverMixin).(confgenerator.OTelReceiver); ok {
		rps, err := r.Pipelines(ctx)
		if err != nil {
			return nil, err
		}
		for _, pipeline := range rps {
			processors := rpr.ProcessorMixin.Processors(ctx)
			for _, p := range processors {
				if p, ok := p.(confgenerator.OTelProcessor); ok {
					c, err := p.Processors(ctx)
					if err != nil {
						return nil, err
					}
					pipeline.Processors["logs"] = append(pipeline.Processors["logs"], c...)
				} else {
					return nil, errors.New("unimplemented")
				}
			}
		}
		return rps, nil
	}
	return nil, errors.New("unimplemented")
}

func init() {
	confgenerator.LoggingProcessorTypes.RegisterType(func() confgenerator.LoggingProcessor { return &LoggingProcessorFlink{} })
	confgenerator.LoggingReceiverTypes.RegisterType(func() confgenerator.LoggingReceiver {
		return &LoggingReceiverFlink{
			LoggingReceiverFilesMixin: confgenerator.LoggingReceiverFilesMixin{
				IncludePaths: []string{
					"/opt/flink/log/flink-*-standalonesession-*.log",
					"/opt/flink/log/flink-*-taskexecutor-*.log",
					"/opt/flink/log/flink-*-client-*.log",
				},
			},
		}
	})
}

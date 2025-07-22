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
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
	"github.com/GoogleCloudPlatform/ops-agent/internal/platform"
)

type MetricsReceiverActiveDirectoryDS struct {
	confgenerator.ConfigComponent       `yaml:",inline"`
	confgenerator.MetricsReceiverShared `yaml:",inline"`
}

func (r MetricsReceiverActiveDirectoryDS) Type() string {
	return "active_directory_ds"
}

func (r MetricsReceiverActiveDirectoryDS) Pipelines(_ context.Context) ([]otel.ReceiverPipeline, error) {
	return []otel.ReceiverPipeline{{
		Receiver: otel.Component{
			Type: "active_directory_ds",
			Config: map[string]interface{}{
				"collection_interval": r.CollectionIntervalString(),
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
		}},
	}}, nil
}

func init() {
	confgenerator.MetricsReceiverTypes.RegisterType(func() confgenerator.MetricsReceiver { return &MetricsReceiverActiveDirectoryDS{} }, platform.Windows)
}

type LoggingProcessorMacroActiveDirectoryDS struct {
	confgenerator.ConfigComponent `yaml:",inline"`
}

func (p LoggingProcessorMacroActiveDirectoryDS) Type() string {
	return "active_directory_ds"
}

func (p LoggingProcessorMacroActiveDirectoryDS) Expand(ctx context.Context) []confgenerator.InternalLoggingProcessor {
	return []confgenerator.InternalLoggingProcessor{}
}

func (p LoggingProcessorMacroActiveDirectoryDS) Components(ctx context.Context, tag, uid string) []fluentbit.Component {
	return []fluentbit.Component{}
}

type LoggingReceiverMacroActiveDirectoryDS struct {
	confgenerator.ConfigComponent `yaml:",inline"`
	Channels                      []string `yaml:"channels"`
}

func (r LoggingReceiverMacroActiveDirectoryDS) Type() string {
	return "active_directory_ds"
}

func (r LoggingReceiverMacroActiveDirectoryDS) Expand(ctx context.Context) (confgenerator.InternalLoggingReceiver, []confgenerator.InternalLoggingProcessor) {
	l := confgenerator.LoggingReceiverWindowsEventLog{
		Channels: r.Channels,
	}
	processor := LoggingProcessorMacroActiveDirectoryDS{}

	return l, processor.Expand(ctx)
}

func init() {
	confgenerator.RegisterLoggingReceiverMacro[LoggingReceiverMacroActiveDirectoryDS](func() LoggingReceiverMacroActiveDirectoryDS {
		return LoggingReceiverMacroActiveDirectoryDS{}
	})
}

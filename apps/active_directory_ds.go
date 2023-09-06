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

func (r MetricsReceiverActiveDirectoryDS) Pipelines(_ context.Context) []otel.ReceiverPipeline {
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
			otel.ModifyInstrumentationScope(r.Type(), "1.0"),
		}},
	}}
}

func init() {
	confgenerator.MetricsReceiverTypes.RegisterType(func() confgenerator.MetricsReceiver { return &MetricsReceiverActiveDirectoryDS{} }, platform.Windows)
}

type LoggingReceiverActiveDirectoryDS struct {
	confgenerator.ConfigComponent `yaml:",inline"`
}

func (r LoggingReceiverActiveDirectoryDS) Type() string {
	return "active_directory_ds"
}

func (r LoggingReceiverActiveDirectoryDS) Components(ctx context.Context, tag string) []fluentbit.Component {
	l := confgenerator.LoggingReceiverWindowsEventLog{
		Channels: []string{"Directory Service", "Active Directory Web Services"},
	}

	c := append(l.Components(ctx, tag),
		confgenerator.LoggingProcessorModifyFields{
			Fields: map[string]*confgenerator.ModifyField{
				InstrumentationSourceLabel: instrumentationSourceValue(r.Type()),
			},
		}.Components(ctx, tag, "active_directory_ds")...,
	)
	return c
}

func init() {
	confgenerator.LoggingReceiverTypes.RegisterType(func() confgenerator.LoggingReceiver { return &LoggingReceiverActiveDirectoryDS{} }, platform.Windows)
}

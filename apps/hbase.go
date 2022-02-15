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
	"fmt"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
)

type MetricsReceiverHbase struct {
	confgenerator.ConfigComponent          `yaml:",inline"`
	confgenerator.MetricsReceiverShared    `yaml:",inline"`
	confgenerator.MetricsReceiverSharedJVM `yaml:",inline"`

	CollectJVMMetics *bool `yaml:"collect_jvm_metrics"`
}

const defaultHbaseEndpoint = "localhost:10101"

func (r MetricsReceiverHbase) Type() string {
	return "hbase"
}

func (r MetricsReceiverHbase) Pipelines() []otel.Pipeline {
	targetSystem := "hbase"
	if r.CollectJVMMetics == nil || *r.CollectJVMMetics {
		targetSystem = fmt.Sprintf("%s,%s", targetSystem, "jvm")
	}

	return r.MetricsReceiverSharedJVM.JVMConfig(
		targetSystem,
		defaultHbaseEndpoint,
		r.CollectionIntervalString(),
		[]otel.Component{
			otel.NormalizeSums(),
			otel.MetricsTransform(
				otel.AddPrefix("workload.googleapis.com"),
				otel.UpdateMetric("hbase.region_server.*",
					otel.AggregateLabels("max", "state"),
				),
			),
		},
	)
}

func init() {
	confgenerator.MetricsReceiverTypes.RegisterType(func() confgenerator.Component { return &MetricsReceiverHbase{} })
}

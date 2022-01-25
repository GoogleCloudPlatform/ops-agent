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
	"log"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
)

type MetricsReceiverHbase struct {
	confgenerator.ConfigComponent `yaml:",inline"`

	confgenerator.MetricsReceiverShared `yaml:",inline"`

	Endpoint string `yaml:"endpoint" validate:"omitempty,hostname_port|startswith=service:jmx:"`
	Username string `yaml:"username" validate:"required_with=Password"`
	Password string `yaml:"password" validate:"required_with=Username"`

	CollectJVMMetics *bool `yaml:"collect_jvm_metrics"`
}

const defaultHbaseEndpoint = "localhost:10101"

func (r MetricsReceiverHbase) Type() string {
	return "hbase"
}

func (r MetricsReceiverHbase) Pipelines() []otel.Pipeline {
	if r.Endpoint == "" {
		r.Endpoint = defaultHbaseEndpoint
	}

	jarPath, err := FindJarPath()
	if err != nil {
		log.Printf(`Encountered an error discovering the location of the JMX Metrics Exporter, %v`, err)
	}

	targetSystem := "hbase"
	if r.CollectJVMMetics == nil || *r.CollectJVMMetics {
		targetSystem = fmt.Sprintf("%s,%s", targetSystem, "jvm")
	}

	config := map[string]interface{}{
		"target_system":       targetSystem,
		"collection_interval": r.CollectionIntervalString(),
		"endpoint":            r.Endpoint,
		"jar_path":            jarPath,
	}

	// Only set the username & password fields if provided
	if r.Username != "" && r.Password != "" {
		config["username"] = r.Username
		config["password"] = r.Password
	}

	return []otel.Pipeline{{
		Receiver: otel.Component{
			Type:   "jmx",
			Config: config,
		},
		Processors: []otel.Component{
			otel.NormalizeSums(),
			otel.MetricsTransform(
				otel.AddPrefix("workload.googleapis.com"),
			),
		},
	}}
}

func init() {
	confgenerator.MetricsReceiverTypes.RegisterType(func() confgenerator.Component { return &MetricsReceiverHbase{} })
}

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

package confgenerator

import (
	"fmt"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
	"github.com/kardianos/osext"
	"os"
	"path/filepath"
	"runtime"
)

type MetricsReceiverJVM struct {
	ConfigComponent `yaml:",inline"`

	MetricsReceiverShared `yaml:",inline"`

	Endpoint string `yaml:"endpoint" validate:"omitempty,url"`
}

const defaultJVMEndpoint = "localhost:9999"

func (r MetricsReceiverJVM) Type() string {
	return "jvm"
}

func (r MetricsReceiverJVM) Pipelines() []otel.Pipeline {
	if r.Endpoint == "" {
		r.Endpoint = defaultJVMEndpoint
	}

	jarPath, err := findJarPath()
	if err != nil {
		_ = err // TODO(djaglowski) return nil, fmt.Errorf("create jvmreceiver: %v, err)
	}

	return []otel.Pipeline{{
		Receiver: otel.Component{
			Type: "jmx",
			Config: map[string]interface{}{
				"target_system":       "jvm",
				"collection_interval": r.CollectionIntervalString(),
				"endpoint":            r.Endpoint,
				"jar_path":            jarPath,
			},
		},
		Processors: []otel.Component{
			otel.NormalizeSums(),
			otel.MetricsTransform(
				otel.AddPrefix("workload.googleapis.com"),
			),
		},
	}}
}

func findJarPath() (string, error) {
	jarName := "opentelemetry-jmx-metrics.jar"

	// TODO(djaglowski) differentiate behavior via build tags
	if runtime.GOOS != "windows" {
		return filepath.Join(os.Getenv("BIN_DIRECTORY"), jarName), nil
	}

	jarDir, err := osext.ExecutableFolder()
	if err != nil {
		return jarName, fmt.Errorf("could not determine binary path: %w", err)
	}
	return filepath.Join(jarDir, jarName), nil
}

func init() {
	metricsReceiverTypes.registerType(func() component { return &MetricsReceiverJVM{} })
}

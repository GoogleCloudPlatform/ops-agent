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
	"path/filepath"
	"runtime"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
	"github.com/kardianos/osext"
)

type MetricsReceiverJVM struct {
	confgenerator.ConfigComponent `yaml:",inline"`

	confgenerator.MetricsReceiverShared `yaml:",inline"`

	Endpoint string `yaml:"endpoint" validate:"omitempty,url"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

const defaultJVMEndpoint = "localhost:9999"

func (r MetricsReceiverJVM) Type() string {
	return "jvm"
}

func (r MetricsReceiverJVM) Pipelines() []otel.Pipeline {
	if r.Endpoint == "" {
		r.Endpoint = defaultJVMEndpoint
	}

	jarPath, err := FindJarPath()
	if err != nil {
		log.Printf(`Encountered an error discovering the location of the JMX Metrics Exporter, %v`, err)
	}

	config := map[string]interface{}{
		"target_system":       "jvm",
		"collection_interval": r.CollectionIntervalString(),
		"endpoint":            r.Endpoint,
		"jar_path":            jarPath,
	}

	// Only set the username & password fields if provided
	if r.Username != "" {
		config["username"] = r.Username
	}
	if r.Password != "" {
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

var FindJarPath = func() (string, error) {
	jarName := "opentelemetry-java-contrib-jmx-metrics.jar"

	executableDir, err := osext.ExecutableFolder()
	if err != nil {
		return jarName, fmt.Errorf("could not determine binary path for jvm receiver: %w", err)
	}

	// TODO(djaglowski) differentiate behavior via build tags
	if runtime.GOOS != "windows" {
		return filepath.Join(executableDir, "../subagents/opentelemetry-collector/", jarName), nil
	}
	return filepath.Join(executableDir, jarName), nil
}

func init() {
	confgenerator.MetricsReceiverTypes.RegisterType(func() confgenerator.Component { return &MetricsReceiverJVM{} })
}

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

package fluentbit

import (
	"os"
	"strconv"
)

const MetricsPort = 20202
const ExperimentalMetricsPortEnv = "EXPERIMENTAL_OPS_AGENT_FLUENT_BIT_METRICS_PORT"

func GetPort() uint16 {
	if portStr := os.Getenv(ExperimentalMetricsPortEnv); portStr != "" {
		if port, err := strconv.ParseUint(portStr, 10, 16); err == nil {
			return uint16(port)
		}
	}
	return MetricsPort
}

func MetricsInputComponent() Component {
	return Component{
		Kind: "INPUT",
		Config: map[string]string{
			"Name":            "fluentbit_metrics",
			"Scrape_On_Start": "True",
			"Scrape_Interval": "60",
		},
	}
}

func MetricsOutputComponent(port int) Component {
	return Component{
		Kind: "OUTPUT",
		Config: map[string]string{
			// https://docs.fluentbit.io/manual/pipeline/outputs/prometheus-exporter
			"Name":  "prometheus_exporter",
			"Match": "*",
			"host":  "0.0.0.0",
			"port":  strconv.Itoa(port),
		},
	}
}

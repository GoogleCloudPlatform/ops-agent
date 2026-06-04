// Copyright 2020 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// // Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package confgenerator

import (
	"os"
	"testing"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
)

func TestPortOverriddenByEnv(t *testing.T) {
	// Set env vars
	os.Setenv(otel.ExperimentalMetricsPortEnv, "40001")
	defer os.Unsetenv(otel.ExperimentalMetricsPortEnv)

	uc := &UnifiedConfig{}

	if port := uc.GetOtelMetricsPort(); port != 40001 {
		t.Errorf("Expected OTel port 40001, got %d", port)
	}
}

func TestPortDefaultWhenEnvEmpty(t *testing.T) {
	// Ensure env vars are not set
	os.Unsetenv(otel.ExperimentalMetricsPortEnv)

	uc := &UnifiedConfig{}

	if port := uc.GetOtelMetricsPort(); port != otel.MetricsPort {
		t.Errorf("Expected OTel default port %d, got %d", otel.MetricsPort, port)
	}
}

func TestPortInvalidEnvFallbacksToDefault(t *testing.T) {
	// Set invalid env vars
	os.Setenv(otel.ExperimentalMetricsPortEnv, "65536") // Out of range for uint16
	defer os.Unsetenv(otel.ExperimentalMetricsPortEnv)

	uc := &UnifiedConfig{}

	if port := uc.GetOtelMetricsPort(); port != otel.MetricsPort {
		t.Errorf("Expected OTel default port %d for invalid env, got %d", otel.MetricsPort, port)
	}
}

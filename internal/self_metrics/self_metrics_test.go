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

package self_metrics_test

import (
	"testing"

	"github.com/GoogleCloudPlatform/ops-agent/apps"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/internal/self_metrics"
	"gotest.tools/v3/assert"
)

var (
	tests = []struct {
		name             string
		config           *confgenerator.UnifiedConfig
		enabledReceivers self_metrics.EnabledReceivers
	}{
		{
			name:   "builtin_linux",
			config: apps.BuiltInConfStructs["linux"],
			enabledReceivers: self_metrics.EnabledReceivers{
				MetricsReceiverCountsByType: map[string]int{"hostmetrics": 1},
				LogsReceiverCountsByType:    map[string]int{"files": 1},
			},
		},
		{
			name:   "builtin_windows",
			config: apps.BuiltInConfStructs["windows"],
			enabledReceivers: self_metrics.EnabledReceivers{
				MetricsReceiverCountsByType: map[string]int{"hostmetrics": 1, "iis": 1, "mssql": 1},
				LogsReceiverCountsByType:    map[string]int{"windows_event_log": 1},
			},
		},
		{
			name: "combined_receiver",
			config: &confgenerator.UnifiedConfig{
				Combined: &confgenerator.Combined{
					Receivers: map[string]confgenerator.CombinedReceiver{
						"otlp": apps.ReceiverOTLP{},
					},
				},
				Logging: &confgenerator.Logging{
					Service: &confgenerator.LoggingService{},
				},
				Metrics: &confgenerator.Metrics{
					Service: &confgenerator.MetricsService{
						Pipelines: map[string]*confgenerator.Pipeline{
							"otlp": {
								ReceiverIDs: []string{"otlp"},
							},
						},
					},
				},
			},
			enabledReceivers: self_metrics.EnabledReceivers{
				MetricsReceiverCountsByType: map[string]int{"otlp": 1},
				LogsReceiverCountsByType:    map[string]int{},
			},
		},
	}
)

func TestEnabledReceiversDefaultConfig(t *testing.T) {
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			eR, err := self_metrics.CountEnabledReceivers(test.config)
			assert.NilError(t, err)
			assert.DeepEqual(t, eR, test.enabledReceivers)
		})
	}
}

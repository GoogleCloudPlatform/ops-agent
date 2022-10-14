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

	"github.com/GoogleCloudPlatform/ops-agent/internal/self_metrics"
	"github.com/GoogleCloudPlatform/ops-agent/apps"
	"gotest.tools/v3/assert"
)

var (
	platforms               = []string{"linux", "windows"}
	defaultEnabledReceivers = map[string]self_metrics.EnabledReceivers{
		"linux": self_metrics.EnabledReceivers{
			MetricsReceiverCountsByType: map[string]int{"hostmetrics": 1},
			LogsReceiverCountsByType:    map[string]int{"files": 1},
		},
		"windows": self_metrics.EnabledReceivers{
			MetricsReceiverCountsByType: map[string]int{"hostmetrics": 1, "iis": 1, "mssql": 1},
			LogsReceiverCountsByType:    map[string]int{"windows_event_log": 1},
		},
	}
)

func TestEnabledReceiversDefaultConfig(t *testing.T) {
	for _, p := range platforms {
		t.Run(p, func(t *testing.T) {
			uc := apps.BuiltInConfStructs[p]
			eR, err := self_metrics.CountEnabledReceivers(uc)
			assert.NilError(t, err)
			assert.DeepEqual(t, eR, defaultEnabledReceivers[p])
		})
	}
}

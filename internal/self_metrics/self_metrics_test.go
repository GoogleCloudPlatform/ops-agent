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

package self_metrics

import (
	"testing"

	"github.com/GoogleCloudPlatform/ops-agent/apps"
	"github.com/google/go-cmp/cmp"
	"gotest.tools/v3/assert"
)

const (
	testdataDir           = "testdata"
	builtinConfigFileName = "built-in-config-%s.yaml"
)

var (
	platforms               = []string{"linux", "windows"}
	defaultEnabledReceivers = map[string]enabledReceivers{
		"linux": enabledReceivers{
			metric: map[string]int{"hostmetrics": 1},
			log:    map[string]int{"files": 1},
		},
		"windows": enabledReceivers{
			metric: map[string]int{"hostmetrics": 1, "iis": 1, "mssql": 1},
			log:    map[string]int{"windows_event_log": 1},
		},
	}
)

func TestEnabledReceiversDefaultConfig(t *testing.T) {

	for _, p := range platforms {
		uc := apps.BuiltInConfStructs[p]

		eR, err := GetEnabledReceivers(uc)
		if err != nil {
			t.Fatal(err)
		}

		assert.DeepEqual(t, eR, defaultEnabledReceivers[p], cmp.AllowUnexported(enabledReceivers{}))
	}
}

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

package agentmetricsprocessor

import (
	"go.opentelemetry.io/collector/pdata/pmetric"
)

const labelName = "blank"

func (mtp *agentMetricsProcessor) addBlankLabel(rms pmetric.ResourceMetricsSlice) error {
	for i := 0; i < rms.Len(); i++ {
		ilms := rms.At(i).ScopeMetrics()
		for j := 0; j < ilms.Len(); j++ {
			metrics := ilms.At(j).Metrics()
			for k := 0; k < metrics.Len(); k++ {
				metric := metrics.At(k)
				var found bool
				for _, name := range mtp.cfg.BlankLabelMetrics {
					if name == metric.Name() {
						found = true
					}
				}
				if !found {
					continue
				}
				if err := forEachPoint(metric, addBlankLabel); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func addBlankLabel(lm labelsMapper) error {
	sm := lm.Attributes()
	sm.PutStr(labelName, "")
	return nil
}

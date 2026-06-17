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

//go:build gpu
// +build gpu

package nvmlreceiver

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNvmlMetricSetFloat64(t *testing.T) {
	var metric deviceMetric
	metric.setFloat64(23.0)
	require.Equal(t, metric.asFloat64(), 23.0)
	metric.setFloat64(43.0)
	require.Equal(t, metric.asFloat64(), 43.0)
}

func TestNvmlMetricSetInt64(t *testing.T) {
	var metric deviceMetric
	metric.setInt64(23)
	require.Equal(t, metric.asInt64(), int64(23))
	metric.setInt64(43)
	require.Equal(t, metric.asInt64(), int64(43))
}

// Copyright 2023 Google LLC
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

package feature_tracking_metadata

import (
	"fmt"

	"cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"github.com/GoogleCloudPlatform/ops-agent/internal/set"
)

type MissingExpectedFeatureTrackingMetricsError struct {
	MissingExpectedFeatures set.Set[FeatureTracking]
}

func (e *MissingExpectedFeatureTrackingMetricsError) Error() string {
	return fmt.Sprintf("missing feature tracking metrics: %v", e.MissingExpectedFeatures)
}

type FeatureTracking struct {
	Module  string
	Feature string
	Key     string
	Value   string
}

type FeatureTrackingContainer struct {
	Features []*FeatureTracking `yaml:"features"`
}

func AssertFeatureTrackingMetrics(series []*monitoringpb.TimeSeries, features []*FeatureTracking) error {
	expectedFeaturesSlice := make([]FeatureTracking, 0)

	for _, f := range features {
		expectedFeaturesSlice = append(expectedFeaturesSlice, *f)
	}
	expectedFeatures := set.FromSlice(expectedFeaturesSlice)

	for _, s := range series {
		labels := s.Metric.Labels
		f := FeatureTracking{
			Module:  labels["module"],
			Feature: labels["feature"],
			Key:     labels["key"],
			Value:   labels["value"],
		}
		expectedFeatures.Remove(f)
	}

	if len(expectedFeatures) != 0 {
		return &MissingExpectedFeatureTrackingMetricsError{MissingExpectedFeatures: expectedFeatures}
	}

	return nil
}

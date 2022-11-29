package feature_tracking

import (
	"errors"
	"fmt"

	"github.com/GoogleCloudPlatform/ops-agent/internal/set"
	monitoringpb "google.golang.org/genproto/googleapis/monitoring/v3"
)

var errMissingFeatureTrackingMetrics = errors.New("missing feature tracking metrics")

type MissingExpectedFeatureTrackingMetricsError struct {
	Err                     error
	MissingExpectedFeatures set.Set[FeatureTracking]
}

func (e *MissingExpectedFeatureTrackingMetricsError) Error() string {
	return e.Err.Error() + ": " + fmt.Sprintf("%v", e.MissingExpectedFeatures)
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
		return &MissingExpectedFeatureTrackingMetricsError{MissingExpectedFeatures: expectedFeatures, Err: errMissingFeatureTrackingMetrics}
	}

	return nil
}

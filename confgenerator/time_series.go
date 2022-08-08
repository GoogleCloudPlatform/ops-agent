// Copyright 2020 Google LLC
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

package confgenerator

import (
	"time"
	timestamp "github.com/golang/protobuf/ptypes/timestamp"
	metricpb "google.golang.org/genproto/googleapis/api/metric"
	monitoringpb "google.golang.org/genproto/googleapis/monitoring/v3"
	monitoredres "google.golang.org/genproto/googleapis/api/monitoredres"
)

type TimeSeries interface {
    ToTimeSeries(instance_id, zone string) []*monitoringpb.TimeSeries
}

type Metric struct {
    metric_type string
    metric_labels map[string]string
    metric_kind metricpb.MetricDescriptor_MetricKind
    value_type metricpb.MetricDescriptor_ValueType
    value *monitoringpb.TypedValue
}

func (m Metric) ToTimeSeries(instance_id, zone string) *monitoringpb.TimeSeries{
    now := &timestamp.Timestamp{
		Seconds: time.Now().Unix(),
	}

	return &monitoringpb.TimeSeries{
		MetricKind: m.metric_kind,
		ValueType: m.value_type,
		Metric: &metricpb.Metric{
			Type: m.metric_type,
			Labels: m.metric_labels,
		},
		Resource: &monitoredres.MonitoredResource{
			Type: "gce_instance",
			Labels: map[string]string{
				"instance_id": instance_id,
				"zone":        zone,
			},
		},
		Points: []*monitoringpb.Point{{
			Interval: &monitoringpb.TimeInterval{
				StartTime: now,
				EndTime:   now,
			},
			Value: m.value,
		}},
	}
}

type EnabledReceivers struct {
	metric map[string]int
	log map[string]int
}

func GetEnabledReceivers(uc UnifiedConfig) (EnabledReceivers, error) {
	eR := EnabledReceivers{
		metric : make(map[string]int),
		log : make(map[string]int),
	}

	// Logging Pipelines
	for _, p := range uc.Logging.Service.Pipelines {
		for _, rID := range p.ReceiverIDs {
			rType := uc.Logging.Receivers[rID].Type()
			eR.log[rType] += 1
		}
	}

	// Metrics Pipelines
	for _, p := range uc.Metrics.Service.Pipelines {
		for _, rID := range p.ReceiverIDs {
			rType := uc.Metrics.Receivers[rID].Type()
			eR.metric[rType] += 1
		}
	}

	return eR, nil
}

func (e EnabledReceivers) ToMetric() []Metric{

	metricList := make([]Metric, 0)
	
	for rType, count := range e.metric {
		m := Metric{
			metric_type : "agent.googleapis.com/agent/ops_agent/enabled_receivers",
		    metric_labels : map[string]string{
                "receiver_type":  rType,
                "telemetry_type": "metrics",
            },
		    metric_kind : metricpb.MetricDescriptor_GAUGE,
		    value_type : metricpb.MetricDescriptor_INT64,
		    value : &monitoringpb.TypedValue{
				Value: &monitoringpb.TypedValue_Int64Value{
					Int64Value: int64(count),
				},
			},
		}

		metricList = append(metricList, m)
	}

	for rType, count := range e.log {
		m := Metric{
			metric_type : "agent.googleapis.com/agent/ops_agent/enabled_receivers",
		    metric_labels : map[string]string{
                "receiver_type":  rType,
                "telemetry_type": "logs",
            },
		    metric_kind : metricpb.MetricDescriptor_GAUGE,
		    value_type : metricpb.MetricDescriptor_INT64,
		    value : &monitoringpb.TypedValue{
				Value: &monitoringpb.TypedValue_Int64Value{
					Int64Value: int64(count),
				},
			},
		}

		metricList = append(metricList, m)
	}

	return metricList
}
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

package health_checks

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/logging"
	monitoring "cloud.google.com/go/monitoring/apiv3/v2"
	"cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/googleapis/gax-go/v2/apierror"
	metricpb "google.golang.org/genproto/googleapis/api/metric"
	"google.golang.org/genproto/googleapis/api/monitoredres"
)

func Ping(ctx context.Context, client monitoring.MetricClient,
	projectId string, instanceId string, zone string) error {
	now := &timestamp.Timestamp{
		Seconds: time.Now().Unix(),
	}
	metricType := "agent.googleapis.com/agent/ops_agent/enabled_receivers"
	value := &monitoringpb.TypedValue{
		Value: &monitoringpb.TypedValue_Int64Value{
			Int64Value: int64(0),
		},
	}
	req := &monitoringpb.CreateTimeSeriesRequest{
		Name: projectId,
		TimeSeries: []*monitoringpb.TimeSeries{{
			Metric: &metricpb.Metric{
				Type: metricType,
			},
			Resource: &monitoredres.MonitoredResource{
				Type: "gce_instance",
				Labels: map[string]string{
					"instance_id": instanceId,
					"zone":        zone,
				},
			},
			Points: []*monitoringpb.Point{{
				Interval: &monitoringpb.TimeInterval{
					StartTime: now,
					EndTime:   now,
				},
				Value: value,
			}},
		}},
	}

	return client.CreateTimeSeries(ctx, req)
}

type APICheck struct{}

func (c APICheck) Name() string {
	return "API Check"
}

func (c APICheck) RunCheck() error {
	ctx := context.Background()
	gceMetadata, err := getGCEMetadata()
	if err != nil {
		return fmt.Errorf("can't get GCE metadata: %w", err)
	}
	projectId := gceMetadata.Project
	instanceId := gceMetadata.InstanceID
	zone := gceMetadata.Zone

	// New Logging Client
	logClient, err := logging.NewClient(ctx, projectId)
	if err != nil {
		return err
	}
	healthChecksLogger.Printf("logging client was created successfully.")

	if err := logClient.Ping(ctx); err != nil {
		if apiErr, ok := err.(*apierror.APIError); ok {
			switch apiErr.Reason() {
			case "SERVICE_DISABLED":
				return LOG_API_DISABLED_ERR
			case "IAM_PERMISSION_DENIED":
				return LOG_API_PERMISSION_ERR
			default:
				return err
			}
		}
		return err
	}
	logClient.Close()

	// New Monitoring Client
	monClient, err := monitoring.NewMetricClient(ctx)
	if err != nil {
		return err
	}
	healthChecksLogger.Printf("monitoring client was created successfully.")

	err = Ping(ctx, *monClient, projectId, instanceId, zone)
	if err != nil {
		if apiErr, ok := err.(*apierror.APIError); ok {
			switch apiErr.Reason() {
			case "SERVICE_DISABLED":
				return MON_API_DISABLED_ERR
			case "IAM_PERMISSION_DENIED":
				return MON_API_PERMISSION_ERR
			default:
				return err
			}
		}
		return err
	}

	monClient.Close()

	return nil
}

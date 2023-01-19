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
	"errors"
	"fmt"
	"log"
	"time"

	"cloud.google.com/go/logging"
	monitoring "cloud.google.com/go/monitoring/apiv3/v2"
	"cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/resourcedetector"
	"github.com/googleapis/gax-go/v2/apierror"
	metricpb "google.golang.org/genproto/googleapis/api/metric"
	"google.golang.org/genproto/googleapis/api/monitoredres"
	"google.golang.org/grpc/codes"
	timestamppb "google.golang.org/protobuf/types/known/timestamppb"
)

func getGCEMetadata() (resourcedetector.GCEResource, error) {
	MetadataResource, err := resourcedetector.GetResource()
	if err != nil {
		return resourcedetector.GCEResource{}, fmt.Errorf("can't get resource metadata: %w", err)
	}
	if gceMetadata, ok := MetadataResource.(resourcedetector.GCEResource); ok {
		return gceMetadata, nil
	} else {
		return resourcedetector.GCEResource{}, fmt.Errorf("not in GCE")
	}
}

// monitoringPing reports whether the client's connection to the monitoring service and the
// authentication configuration are valid. To accomplish this, monitoringPing writes a
// time series point with empty values to an Ops Agent specific metric.
// This method mirrors the "(c *Client) Ping" method in "cloud.google.com/go/logging".
func monitoringPing(ctx context.Context, client monitoring.MetricClient, gceMetadata resourcedetector.GCEResource) error {
	metricType := "agent.googleapis.com/agent/ops_agent/enabled_receivers"
	now := &timestamppb.Timestamp{
		Seconds: time.Now().Unix(),
	}
	value := &monitoringpb.TypedValue{
		Value: &monitoringpb.TypedValue_Int64Value{
			Int64Value: int64(0),
		},
	}
	req := &monitoringpb.CreateTimeSeriesRequest{
		Name: "projects/" + gceMetadata.Project,
		TimeSeries: []*monitoringpb.TimeSeries{{
			MetricKind: metricpb.MetricDescriptor_GAUGE,
			ValueType:  metricpb.MetricDescriptor_INT64,
			Metric: &metricpb.Metric{
				Type: metricType,
			},
			Resource: &monitoredres.MonitoredResource{
				Type: "gce_instance",
				Labels: map[string]string{
					"instance_id": gceMetadata.InstanceID,
					"zone":        gceMetadata.Zone,
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

func (c APICheck) RunCheck(logger *log.Logger) error {
	ctx := context.Background()
	gceMetadata, err := getGCEMetadata()
	if err != nil {
		return fmt.Errorf("can't get GCE metadata: %w", err)
	}
	logger.Println(gceMetadata)

	// New Logging Client
	logClient, err := logging.NewClient(ctx, gceMetadata.Project)
	if err != nil {
		return err
	}
	logger.Printf("logging client was created successfully.")

	if err := logClient.Ping(ctx); err != nil {
		logger.Println(err)
		var apiErr *apierror.APIError
		if errors.As(err, &apiErr) {
			switch apiErr.Reason() {
			case "SERVICE_DISABLED":
				return LOG_API_DISABLED_ERR
			case "ACCESS_TOKEN_SCOPE_INSUFFICIENT":
				return LOG_API_SCOPE_ERR
			case "IAM_PERMISSION_DENIED":
				return LOG_API_PERMISSION_ERR
			}

			switch apiErr.GRPCStatus().Code() {
			case codes.PermissionDenied:
				return LOG_API_PERMISSION_ERR
			case codes.Unauthenticated:
				return LOG_API_SCOPE_ERR
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
	logger.Printf("monitoring client was created successfully.")

	if err := monitoringPing(ctx, *monClient, gceMetadata); err != nil {
		logger.Println(err)
		var apiErr *apierror.APIError
		if errors.As(err, &apiErr) {
			switch apiErr.Reason() {
			case "SERVICE_DISABLED":
				return MON_API_DISABLED_ERR
			case "ACCESS_TOKEN_SCOPE_INSUFFICIENT":
				return MON_API_SCOPE_ERR
			case "IAM_PERMISSION_DENIED":
				return MON_API_PERMISSION_ERR
			}

			switch apiErr.GRPCStatus().Code() {
			case codes.PermissionDenied:
				return MON_API_PERMISSION_ERR
			case codes.Unauthenticated:
				return MON_API_SCOPE_ERR
			}
		}
		return err
	}
	monClient.Close()

	return nil
}

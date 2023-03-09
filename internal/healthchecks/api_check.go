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

package healthchecks

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

const (
	ServiceDisabled              = "SERVICE_DISABLED"
	AccessTokenScopeInsufficient = "ACCESS_TOKEN_SCOPE_INSUFFICIENT"
	IamPermissionDenied          = "IAM_PERMISSION_DENIED"
)

func getGCEMetadata() (resourcedetector.GCEResource, error) {
	MetadataResource, err := resourcedetector.GetResource()
	if err != nil {
		return resourcedetector.GCEResource{}, fmt.Errorf("can't get resource metadata: %w", err)
	}
	if gceMetadata, ok := MetadataResource.(resourcedetector.GCEResource); ok {
		return gceMetadata, nil
	}
	return resourcedetector.GCEResource{}, fmt.Errorf("not in GCE")
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

type LoggingAPICheck struct{}

func (c LoggingAPICheck) Name() string {
	return "Logging API Check"
}

func (c LoggingAPICheck) RunCheck(logger *log.Logger) error {
	ctx := context.Background()
	gceMetadata, err := getGCEMetadata()
	if err != nil {
		return fmt.Errorf("can't get GCE metadata: %w", err)
	}
	logger.Printf("gce metadata: %+v", gceMetadata)

	// New Logging Client
	logClient, err := logging.NewClient(ctx, gceMetadata.Project)
	if err != nil {
		return err
	}
	defer logClient.Close()
	logger.Printf("logging client was created successfully")

	if err := logClient.Ping(ctx); err != nil {
		logger.Println(err)
		var apiErr *apierror.APIError
		if errors.As(err, &apiErr) {
			switch apiErr.Reason() {
			case ServiceDisabled:
				return LogApiDisabledErr
			case AccessTokenScopeInsufficient:
				return LogApiScopeErr
			case IamPermissionDenied:
				return LogApiPermissionErr
			}

			switch apiErr.GRPCStatus().Code() {
			case codes.PermissionDenied:
				return LogApiPermissionErr
			case codes.Unauthenticated:
				return LogApiScopeErr
			}
		}

		return err
	}

	return nil
}

type MonitoringAPICheck struct{}

func (c MonitoringAPICheck) Name() string {
	return "Monitoring API Check"
}

func (c MonitoringAPICheck) RunCheck(logger *log.Logger) error {
	ctx := context.Background()
	gceMetadata, err := getGCEMetadata()
	if err != nil {
		return fmt.Errorf("can't get GCE metadata: %w", err)
	}
	logger.Printf("gce metadata: %+v", gceMetadata)

	// New Monitoring Client
	monClient, err := monitoring.NewMetricClient(ctx)
	if err != nil {
		return err
	}
	defer monClient.Close()
	logger.Printf("monitoring client was created successfully")

	if err := monitoringPing(ctx, *monClient, gceMetadata); err != nil {
		logger.Println(err)
		var apiErr *apierror.APIError
		if errors.As(err, &apiErr) {
			switch apiErr.Reason() {
			case ServiceDisabled:
				return MonApiDisabledErr
			case AccessTokenScopeInsufficient:
				return MonApiScopeErr
			case IamPermissionDenied:
				return MonApiPermissionErr
			}

			switch apiErr.GRPCStatus().Code() {
			case codes.PermissionDenied:
				return MonApiPermissionErr
			case codes.Unauthenticated:
				return MonApiScopeErr
			}
		}
		return err
	}

	return nil
}

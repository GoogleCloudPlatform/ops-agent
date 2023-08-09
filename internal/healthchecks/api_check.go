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
	"time"

	"cloud.google.com/go/logging"
	monitoring "cloud.google.com/go/monitoring/apiv3/v2"
	"cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/resourcedetector"
	"github.com/GoogleCloudPlatform/ops-agent/internal/logs"
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

const metricType = "agent.googleapis.com/agent/ops_agent/enabled_receivers"

// monitoringPing reports whether the client's connection to the monitoring service and the
// authentication configuration are valid. To accomplish this, monitoringPing writes a
// time series point with empty values to an Ops Agent specific metric.
// This method mirrors the "(c *Client) Ping" method in "cloud.google.com/go/logging".
func monitoringPing(ctx context.Context, client monitoring.MetricClient, resource resourcedetector.Resource) error {
	var req *monitoringpb.CreateTimeSeriesRequest
	now := &timestamppb.Timestamp{
		Seconds: time.Now().Unix(),
	}
	value := &monitoringpb.TypedValue{
		Value: &monitoringpb.TypedValue_Int64Value{
			Int64Value: int64(0),
		},
	}
	switch resource.GetType() {
	case resourcedetector.BMS:
		if bmsResource, ok := resource.(resourcedetector.BMSResource); ok {
			req = createTimeSeriesRequestForBMS(now, value, bmsResource)
		}
	case resourcedetector.GCE:
		if gceResource, ok := resource.(resourcedetector.GCEResource); ok {
			req = createTimeSeriesRequestForGCE(now, value, gceResource)
		}
	default:
		return fmt.Errorf("unrecognized platform")
	}
	return client.CreateTimeSeries(ctx, req)
}

func createTimeSeriesRequestForBMS(now *timestamppb.Timestamp, value *monitoringpb.TypedValue, bmsMetadata resourcedetector.BMSResource) *monitoringpb.CreateTimeSeriesRequest {
	return &monitoringpb.CreateTimeSeriesRequest{
		Name: "projects/" + bmsMetadata.Project,
		TimeSeries: []*monitoringpb.TimeSeries{{
			MetricKind: metricpb.MetricDescriptor_GAUGE,
			ValueType:  metricpb.MetricDescriptor_INT64,
			Metric: &metricpb.Metric{
				Type: metricType,
			},
			Resource: &monitoredres.MonitoredResource{
				Type: "baremetalsolution.googleapis.com/Instance",
				Labels: map[string]string{
					"instance_id": bmsMetadata.InstanceID,
					"location":    bmsMetadata.Location,
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
}

func createTimeSeriesRequestForGCE(now *timestamppb.Timestamp, value *monitoringpb.TypedValue, gceMetadata resourcedetector.GCEResource) *monitoringpb.CreateTimeSeriesRequest {
	return &monitoringpb.CreateTimeSeriesRequest{
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
}

func runLoggingCheck(logger logs.StructuredLogger, resource resourcedetector.Resource) error {
	ctx := context.Background()
	var project string
	switch resource.GetType() {
	case resourcedetector.BMS:
		if bmsResource, ok := resource.(resourcedetector.BMSResource); ok {
			project = bmsResource.Project
		}
	case resourcedetector.GCE:
		if gceResource, ok := resource.(resourcedetector.GCEResource); ok {
			project = gceResource.Project
		}
	default:
		return fmt.Errorf("unrecognized platform")
	}

	// New Logging Client
	logClient, err := logging.NewClient(ctx, project)
	if err != nil {
		return err
	}
	defer logClient.Close()
	logger.Infof("logging client was created successfully")

	if err := logClient.Ping(ctx); err != nil {
		logger.Infof(err.Error())
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
				return LogApiUnauthenticatedErr
			case codes.DeadlineExceeded:
				return LogApiConnErr
			}
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return LogApiConnErr
		}
		return err
	}

	return nil
}

func runMonitoringCheck(logger logs.StructuredLogger, resource resourcedetector.Resource) error {
	ctx := context.Background()

	// New Monitoring Client
	monClient, err := monitoring.NewMetricClient(ctx)
	if err != nil {
		return err
	}
	defer monClient.Close()
	logger.Infof("monitoring client was created successfully")

	if err := monitoringPing(ctx, *monClient, resource); err != nil {
		logger.Infof(err.Error())
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
				return MonApiUnauthenticatedErr
			case codes.DeadlineExceeded:
				return MonApiConnErr
			}
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return MonApiConnErr
		}
		return err
	}

	return nil
}

type APICheck struct{}

func (c APICheck) Name() string {
	return "API Check"
}

func (c APICheck) RunCheck(logger logs.StructuredLogger, resource resourcedetector.Resource) error {
	monErr := runMonitoringCheck(logger, resource)
	logErr := runLoggingCheck(logger, resource)
	return errors.Join(monErr, logErr)
}

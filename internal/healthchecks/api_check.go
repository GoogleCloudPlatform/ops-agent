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
	"github.com/cenkalti/backoff/v4"
	"github.com/googleapis/gax-go/v2/apierror"
	metricpb "google.golang.org/genproto/googleapis/api/metric"
	"google.golang.org/grpc/codes"
	timestamppb "google.golang.org/protobuf/types/known/timestamppb"
)

const (
	ServiceDisabled              = "SERVICE_DISABLED"
	AccessTokenScopeInsufficient = "ACCESS_TOKEN_SCOPE_INSUFFICIENT"
	IamPermissionDenied          = "IAM_PERMISSION_DENIED"
	MaxMonitoringPingRetries     = 1
)

func createMonitoringPingRequest(resource resourcedetector.Resource) *monitoringpb.CreateTimeSeriesRequest {
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
		Name: "projects/" + resource.ProjectName(),
		TimeSeries: []*monitoringpb.TimeSeries{{
			MetricKind: metricpb.MetricDescriptor_GAUGE,
			ValueType:  metricpb.MetricDescriptor_INT64,
			Metric: &metricpb.Metric{
				Type: metricType,
			},
			Resource: resource.MonitoredResource(),
			Points: []*monitoringpb.Point{{
				Interval: &monitoringpb.TimeInterval{
					StartTime: now,
					EndTime:   now,
				},
				Value: value,
			}},
		}},
	}
	return req
}

// monitoringPing reports whether the client's connection to the monitoring service and the
// authentication configuration are valid. To accomplish this, monitoringPing writes a
// time series point with empty values to an Ops Agent specific metric.
// This method mirrors the "(c *Client) Ping" method in "cloud.google.com/go/logging".
func monitoringPing(ctx context.Context, client monitoring.MetricClient, resource resourcedetector.Resource) error {
	// Points written to a time series must be at least 5 seconds apart. Because `monitoringPing` might
	// be called multiple times in quick succession, the first attempted request to `CreateTimeSeries`
	// may fail. We can retry the request >5 seconds later in such cases.
	// https://cloud.google.com/monitoring/quotas
	pingBackoff := backoff.WithMaxRetries(backoff.NewConstantBackOff(6*time.Second), MaxMonitoringPingRetries)
	pingOperation := func() error { return client.CreateTimeSeries(ctx, createMonitoringPingRequest(resource)) }
	return backoff.Retry(pingOperation, pingBackoff)
}

func runLoggingCheck(logger logs.StructuredLogger, resource resourcedetector.Resource) error {
	ctx := context.Background()

	// New Logging Client
	logClient, err := logging.NewClient(ctx, resource.ProjectName())
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
			case codes.Unavailable:
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
			case codes.Unavailable:
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

func (c APICheck) RunCheck(logger logs.StructuredLogger) error {
	resource, err := resourcedetector.GetResource()
	if err != nil {
		return fmt.Errorf("failed to detect the resource: %v", err)
	}
	monErr := runMonitoringCheck(logger, resource)
	logErr := runLoggingCheck(logger, resource)
	return errors.Join(monErr, logErr)
}

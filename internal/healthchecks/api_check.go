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
	"strings"
	"sync"
	"time"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/resourcedetector"
	"github.com/GoogleCloudPlatform/ops-agent/internal/logs"
	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	metricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	metricsprpb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	"golang.org/x/oauth2/google"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/oauth"
	"google.golang.org/grpc/status"
)

const (
	ServiceDisabled              = "SERVICE_DISABLED"
	AccessTokenScopeInsufficient = "ACCESS_TOKEN_SCOPE_INSUFFICIENT"
	IamPermissionDenied          = "IAM_PERMISSION_DENIED"
)

func createTelemetryMetricsRequest(resource resourcedetector.Resource) *metricspb.ExportMetricsServiceRequest {
	return &metricspb.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricsprpb.ResourceMetrics{
			{
				Resource: &resourcepb.Resource{
					Attributes: func() []*commonpb.KeyValue {
						attrs := []*commonpb.KeyValue{
							{
								Key: "gcp.project_id",
								Value: &commonpb.AnyValue{
									Value: &commonpb.AnyValue_StringValue{
										StringValue: resource.ProjectName(),
									},
								},
							},
						}
						for k, v := range resource.OTelResourceAttributes() {
							attrs = append(attrs, &commonpb.KeyValue{
								Key: k,
								Value: &commonpb.AnyValue{
									Value: &commonpb.AnyValue_StringValue{
										StringValue: v,
									},
								},
							})
						}
						return attrs
					}(),
				},
				ScopeMetrics: []*metricsprpb.ScopeMetrics{
					{
						Scope: &commonpb.InstrumentationScope{},
						Metrics: []*metricsprpb.Metric{
							{
								Name: "agent.googleapis.com/agent/ops_agent/enabled_receivers",
								Data: &metricsprpb.Metric_Gauge{
									Gauge: &metricsprpb.Gauge{
										DataPoints: []*metricsprpb.NumberDataPoint{
											{
												Value: &metricsprpb.NumberDataPoint_AsInt{
													AsInt: 0,
												},
												TimeUnixNano: uint64(time.Now().UnixNano()),
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func createTelemetryLogsRequest(resource resourcedetector.Resource) *collogspb.ExportLogsServiceRequest {
	currentTimeNano := uint64(time.Now().UnixNano())
	return &collogspb.ExportLogsServiceRequest{
		ResourceLogs: []*logspb.ResourceLogs{
			{
				Resource: &resourcepb.Resource{
					Attributes: []*commonpb.KeyValue{
						{Key: "gcp.project_id", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: resource.ProjectName()}}},
					},
				},
				ScopeLogs: []*logspb.ScopeLogs{
					{
						Scope: &commonpb.InstrumentationScope{},
						LogRecords: []*logspb.LogRecord{
							{
								ObservedTimeUnixNano: currentTimeNano,
								TimeUnixNano:         currentTimeNano,
								SeverityText:         "DEBUG",
								SeverityNumber:       5,
								Body: &commonpb.AnyValue{
									Value: &commonpb.AnyValue_StringValue{StringValue: "Health check log entry"},
								},
								Attributes: []*commonpb.KeyValue{
									{Key: "instrumentation_source", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "agent.googleapis.com/health_check"}}},
								},
							},
						},
					},
				},
			},
		},
	}
}

func runTelemetryMetricsCheck(logger logs.StructuredLogger, resource resourcedetector.Resource) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	creds, err := google.FindDefaultCredentials(ctx,
		"https://www.googleapis.com/auth/monitoring.write",
	)
	if err != nil {
		return fmt.Errorf("failed to find default credentials: %v", err)
	}

	conn, err := grpc.NewClient(
		"telemetry.googleapis.com:443",
		grpc.WithTransportCredentials(credentials.NewTLS(nil)),
		grpc.WithPerRPCCredentials(oauth.TokenSource{TokenSource: creds.TokenSource}),
	)
	if err != nil {
		return err
	}
	defer conn.Close()
	logger.Infof("telemetry client was created successfully")

	client := metricspb.NewMetricsServiceClient(conn)

	req := createTelemetryMetricsRequest(resource)

	_, err = client.Export(ctx, req)
	if err != nil {
		stat, ok := status.FromError(err)
		if ok {
			for _, detail := range stat.Details() {
				if info, ok := detail.(*errdetails.ErrorInfo); ok {
					if info.Reason == AccessTokenScopeInsufficient {
						return MonApiScopeErr
					}
				}
			}
			switch stat.Code() {
			case codes.PermissionDenied:
				if strings.Contains(stat.Message(), "disabled") {
					return TelApiDisabledErr
				}
				return TelMetricsApiPermissionErr
			case codes.Unauthenticated:
				return TelApiUnauthenticatedErr
			case codes.DeadlineExceeded, codes.Unavailable:
				return TelApiConnErr
			}
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return TelApiConnErr
		}
		return err
	}
	return nil
}

func runTelemetryLogsCheck(logger logs.StructuredLogger, resource resourcedetector.Resource) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	creds, err := google.FindDefaultCredentials(ctx,
		"https://www.googleapis.com/auth/logging.write",
	)
	if err != nil {
		return fmt.Errorf("failed to find default credentials: %v", err)
	}

	conn, err := grpc.NewClient(
		"telemetry.googleapis.com:443",
		grpc.WithTransportCredentials(credentials.NewTLS(nil)),
		grpc.WithPerRPCCredentials(oauth.TokenSource{TokenSource: creds.TokenSource}),
	)
	if err != nil {
		return err
	}
	defer conn.Close()
	logger.Infof("telemetry client was created successfully")

	client := collogspb.NewLogsServiceClient(conn)

	req := createTelemetryLogsRequest(resource)

	_, err = client.Export(ctx, req)
	if err != nil {
		stat, ok := status.FromError(err)
		if ok {
			for _, detail := range stat.Details() {
				if info, ok := detail.(*errdetails.ErrorInfo); ok {
					if info.Reason == AccessTokenScopeInsufficient {
						return LogApiScopeErr
					}
				}
			}
			switch stat.Code() {
			case codes.PermissionDenied:
				if strings.Contains(stat.Message(), "disabled") {
					return TelApiDisabledErr
				}
				return TelLogsApiPermissionErr
			case codes.Unauthenticated:
				return TelApiUnauthenticatedErr
			case codes.DeadlineExceeded, codes.Unavailable:
				return TelApiConnErr
			}
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return TelApiConnErr
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

	var monOrTelErr error
	var logOrTelLogsErr error
	var wg sync.WaitGroup

	wg.Add(2)
	logger.Infof("Running Telemetry API checks")
	go func() {
		defer wg.Done()
		monOrTelErr = runTelemetryMetricsCheck(logger, resource)
	}()
	go func() {
		defer wg.Done()
		logOrTelLogsErr = runTelemetryLogsCheck(logger, resource)
	}()
	wg.Wait()

	return errors.Join(monOrTelErr, logOrTelLogsErr)
}

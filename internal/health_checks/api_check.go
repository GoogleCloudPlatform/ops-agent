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

	"cloud.google.com/go/logging"
	monitoring "cloud.google.com/go/monitoring/apiv3/v2"
	"cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"github.com/googleapis/gax-go/v2/apierror"
	"google.golang.org/api/iterator"
)

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

	// New Logging Client
	logClient, err := logging.NewClient(ctx, projectId)
	if err != nil {
		return err
	}
	HealtChecksLogger.Printf("logging client was created successfully.")

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
	req := &monitoringpb.ListMetricDescriptorsRequest{
		Name: "projects/" + projectId,
	}
	it := monClient.ListMetricDescriptors(ctx, req)
	for {
		resp, err := it.Next()
		if err == iterator.Done {
			break
		}
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
		_ = resp
	}
	monClient.Close()

	return nil
}

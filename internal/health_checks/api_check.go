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
	"log"
	"context"
	"fmt"

	"cloud.google.com/go/logging"
	monitoring "cloud.google.com/go/monitoring/apiv3/v2"
)

type APICheck struct {}

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
	if logClient != nil {
		log.Printf("logging client was created successfully.")
	} else {
		return LOG_API_DISABLED_ERR
	}
	if err := logClient.Ping(ctx); err != nil {
		return LOG_API_DISABLED_ERR
	}
	logClient.Close()

	// New Monitoring Client
	monClient, err := monitoring.NewMetricClient(ctx)
	if err != nil {
		return MON_API_DISABLED_ERR
	}
	if monClient != nil {
		log.Printf("monitoring-api-disabled")
	} else {
		return MON_API_DISABLED_ERR
	}
	monClient.Close()

	return nil
}
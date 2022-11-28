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
)

type APICheck struct {
	HealthCheck
}

func (c APICheck) RunCheck() error {
	ctx := context.Background()
	gceMetadata, err := getGCEMetadata()
	if err != nil {
		compositeError := fmt.Errorf("can't get GCE metadata: %w", err)
		c.Error(compositeError)
		return compositeError
	}
	projectId := gceMetadata.Project

	// New Logging Client
	logClient, err := logging.NewClient(ctx, projectId)
	if err != nil {
		c.Error(err)
		return err
	}
	if logClient != nil {
		c.Log("logging client was created successfully.")
	} else {
		c.Fail("logging-api-disabled")
	}
	if err := logClient.Ping(ctx); err != nil {
		// c.Fail("logging client didn't Ping successfully.", "check the logging api is enabled.")
		c.Fail("logging-api-disabled")
	}
	logClient.Close()

	// New Monitoring Client
	monClient, err := monitoring.NewMetricClient(ctx)
	if err != nil {
		c.Error(err)
		return err
	}
	if monClient != nil {
		c.Log("monitoring-api-disabled")
	} else {
		c.Fail("monitoring-api-disabled")
	}
	monClient.Close()

	return nil
}

func init() {
	GCEHealthChecks.RegisterCheck("API Check", &APICheck{HealthCheck: NewHealthCheck()})
}

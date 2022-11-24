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
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/resourcedetector"

	monitoring "cloud.google.com/go/monitoring/apiv3/v2"
)

type APICheck struct {
	HealthCheck
}

func (c APICheck) RunCheck(uc *confgenerator.UnifiedConfig) error {
	var project string
	c.LogMessage("Get MetadataResource : ")
	MetadataResource, err := resourcedetector.GetResource()
	if err != nil {
		return fmt.Errorf("can't get resource metadata: %w", err)
	}
	if gceMetadata, ok := MetadataResource.(resourcedetector.GCEResource); ok {
		c.LogMessage(fmt.Sprintf("==> gceMetadata : %+v \n \n", gceMetadata))
		project = gceMetadata.Project
	} else {
		// Not on GCE
		project = "Not-on-GCE"
	}
	c.LogMessage(fmt.Sprintf("==> project : %s \n \n", project))

	c.LogMessage("\n> APICheck \n \n")
	ctx := context.Background()

	// New Logging Client
	c.LogMessage("==> New Logging Client \n")
	logClient, err := logging.NewClient(ctx, project)
	if err != nil {
		c.Fail("Logging Client didn't create successfully.", "Check the Logging API is enabled.")
		// c.LogMessage(err)
		// return "", err
	}
	if err := logClient.Ping(ctx); err != nil {
		c.Fail("Logging Client didn't Ping successfully.", "Check the Logging API is enabled.")
		// c.LogMessage(err)
		// return err
	}
	c.LogMessage("Logging API Ping succeded")
	logClient.Close()

	// New Monitoring Client
	c.LogMessage("==> New Monitoring Client \n \n")
	monClient, err := monitoring.NewMetricClient(ctx)
	if err != nil {
		c.Fail("Monitoring Client didn't create successfully.", "Check the Monitoring API is enabled.")
		// c.LogMessage(err)
		// return err
	}
	c.LogMessage("==> Monitoring Client successfully created")
	monClient.Close()

	return nil
}

func init() {
	GCEHealthChecks.RegisterCheck("API Check", &APICheck{HealthCheck: NewHealthCheck()})
}

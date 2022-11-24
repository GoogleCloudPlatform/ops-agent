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
	"fmt"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/resourcedetector"
)

var (
	// Expected scopes
	requiredLoggingScopes = []string{
		"https://www.googleapis.com/auth/logging.write",
		"https://www.googleapis.com/auth/logging.admin",
	}
	requiredMonitoringScopes = []string{
		"https://www.googleapis.com/auth/monitoring.write",
		"https://www.googleapis.com/auth/monitoring.admin",
	}
)

func constainsAtLeastOne(searchSlice []string, querySlice []string) (bool, error) {
	for _, query := range querySlice {
		for _, searchElement := range searchSlice {
			if query == searchElement {
				return true, nil
			}
		}
	}
	return false, nil
}

type PermissionsCheck struct {
	HealthCheck
}

func (c PermissionsCheck) RunCheck(uc *confgenerator.UnifiedConfig) error {

	var project string
	var defaultScopes []string
	c.LogMessage("Get MetadataResource : ")
	MetadataResource, err := resourcedetector.GetResource()
	if err != nil {
		return fmt.Errorf("can't get resource metadata: %w", err)
	}
	if gceMetadata, ok := MetadataResource.(resourcedetector.GCEResource); ok {
		c.LogMessage(fmt.Sprintf("==> gceMetadata : %+v \n \n", gceMetadata))
		project = gceMetadata.Project
		defaultScopes = gceMetadata.DefaultScopes
	} else {
		// Not on GCE
		project = "Not-on-GCE"
	}
	c.LogMessage(fmt.Sprintf("==> project : %s \n \n", project))

	found, err := constainsAtLeastOne(defaultScopes, requiredLoggingScopes)
	if err != nil {
		return err
	} else if found {
		c.LogMessage("==> Logging Scopes are enough to run the Ops Agent.")
	} else {
		c.Fail("Logging Scopes are not enough to run the Ops Agent.", "Add log.writer or log.admin role.")
	}

	found, err = constainsAtLeastOne(defaultScopes, requiredMonitoringScopes)
	if err != nil {
		return err
	} else if found {
		c.LogMessage("Monitoring Scopes are enough to run the Ops Agent.")
	} else {
		c.Fail("Monitoring Scopes are not enough to run the Ops Agent.", "Add monitoring.writer or monitoring.admin role.")
	}

	return nil
}

func init() {
	GCEHealthChecks.RegisterCheck("Permissions Check", &PermissionsCheck{HealthCheck: NewHealthCheck()})
}

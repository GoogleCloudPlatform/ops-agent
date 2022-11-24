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

func (c PermissionsCheck) RunCheck() error {
	gceMetadata, err := getGCEMetadata()
    if err != nil {
    	compositeError := fmt.Errorf("can't get GCE metadata: %w", err)
    	c.Error(compositeError)
        return compositeError
    }
    defaultScopes := gceMetadata.DefaultScopes

	found, err := constainsAtLeastOne(defaultScopes, requiredLoggingScopes)
	if err != nil {
		c.Error(err)
		return err
	} else if found {
		c.Log("Logging Scopes are enough to run the Ops Agent.")
	} else {
		c.Fail("Logging Scopes are not enough to run the Ops Agent.", "Add log.writer or log.admin role.")
	}

	found, err = constainsAtLeastOne(defaultScopes, requiredMonitoringScopes)
	if err != nil {
		c.Error(err)
		return err
	} else if found {
		c.Log("Monitoring Scopes are enough to run the Ops Agent.")
	} else {
		c.Fail("Monitoring Scopes are not enough to run the Ops Agent.", "Add monitoring.writer or monitoring.admin role.")
	}

	return nil
}

func init() {
	GCEHealthChecks.RegisterCheck("Permissions Check", &PermissionsCheck{HealthCheck: NewHealthCheck()})
}

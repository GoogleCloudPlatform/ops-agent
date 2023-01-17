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
	"io/ioutil"
	"os"

	"cloud.google.com/go/compute/metadata"
	"golang.org/x/oauth2/google"
)

var (
	// Expected scopes
	requiredLoggingScopes = []string{
		"https://www.googleapis.com/auth/logging.write",
		"https://www.googleapis.com/auth/logging.admin",
		"https://www.googleapis.com/auth/cloud-platform",
	}
	requiredMonitoringScopes = []string{
		"https://www.googleapis.com/auth/monitoring.write",
		"https://www.googleapis.com/auth/monitoring.admin",
		"https://www.googleapis.com/auth/cloud-platform",
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

type PermissionsCheck struct{}

func (c PermissionsCheck) Name() string {
	return "Permissions Check"
}

// PermissionsChecks searches for "Application Default Credentials".
//
// It looks for credentials in the following places,
// preferring the first location found:
//
//  1. A JSON file whose path is specified by the
//     GOOGLE_APPLICATION_CREDENTIALS environment variable.
//     For workload identity federation, refer to
//     https://cloud.google.com/iam/docs/how-to#using-workload-identity-federation on
//     how to generate the JSON configuration file for on-prem/non-Google cloud
//     platforms.
//  2. On Google Compute Engine, Google App Engine standard second generation runtimes
//     (>= Go 1.11), and Google App Engine flexible environment, it fetches
//     credentials from the metadata server.

func (c PermissionsCheck) RunCheck() error {
	// First, try the environment variable.
	const envVar = "GOOGLE_APPLICATION_CREDENTIALS"
	if filename := os.Getenv(envVar); filename != "" {
		b, err := ioutil.ReadFile(filename)
		if err != nil {
			return fmt.Errorf("error reading credentials using %v environment variable: %v", envVar, err)
		}

		ctx := context.Background()
		params := google.CredentialsParams{
			Scopes: []string{requiredLoggingScopes[0], requiredMonitoringScopes[0]},
		}

		_, err = google.CredentialsFromJSONWithParams(ctx, b, params)
		if err != nil {
			return fmt.Errorf("error getting credentials using %v environment variable: %v", envVar, err)
		}

		return CREDENTIALS_UNVERIFIABLE
	}

	// Second try metadata server
	if metadata.OnGCE() {
		gceMetadata, err := getGCEMetadata()
		if err != nil {
			return fmt.Errorf("can't get GCE metadata: %w", err)
		}
		defaultScopes := gceMetadata.DefaultScopes

		found, err := constainsAtLeastOne(defaultScopes, requiredLoggingScopes)
		if err != nil {
			return err
		} else if found {
			healthChecksLogger.Printf("Logging Scopes are enough to run the Ops Agent.")
		} else {
			return LOG_API_PERMISSION_ERR
		}

		found, err = constainsAtLeastOne(defaultScopes, requiredMonitoringScopes)
		if err != nil {
			return err
		} else if found {
			healthChecksLogger.Printf("Monitoring Scopes are enough to run the Ops Agent.")
		} else {
			return MON_API_PERMISSION_ERR
		}
	}

	return nil
}

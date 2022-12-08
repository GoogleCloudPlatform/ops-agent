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
	"log"
	"strings"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/resourcedetector"
	"go.uber.org/multierr"
)

func getGCEMetadata() (resourcedetector.GCEResource, error) {
	MetadataResource, err := resourcedetector.GetResource()
	if err != nil {
		return resourcedetector.GCEResource{}, fmt.Errorf("can't get resource metadata: %w", err)
	}
	if gceMetadata, ok := MetadataResource.(resourcedetector.GCEResource); ok {
		log.Printf("gceMetadata : %+v", gceMetadata)
		return gceMetadata, nil
	} else {
		return resourcedetector.GCEResource{}, fmt.Errorf("not in GCE")
	}
}

type HealthCheck interface {
	Name() string
	RunCheck() error
}

type HealthCheckRegistry []HealthCheck

func (r HealthCheckRegistry) RunAllHealthChecks() (string, error) {
	var multiErr error
	var result []string
	result = append(result, "========================================")
	result = append(result, "Health Checks : ")
	for _, c := range r {
		err := c.RunCheck()
		if err != nil {
			if healthError, ok := err.(HealthCheckError); ok {
				result = append(result, fmt.Sprintf("Check: %s, Result: FAIL", c.Name()))
				result = append(result, fmt.Sprintf("Failure: %s", healthError.message))
				result = append(result, fmt.Sprintf("Solution: %s \n", healthError.action))
			} else {
				result = append(result, fmt.Sprintf("Check: %s, Result: ERROR", c.Name()))
				result = append(result, fmt.Sprintf("Detail: %s \n", err.Error()))
			}
		} else {
			result = append(result, fmt.Sprintf("Check: %s, Result: PASS \n", c.Name()))
		}

		if err != nil {
			multiErr = multierr.Append(multiErr, err)
		}
	}
	result = append(result, "===========================================================")

	return strings.Join(result, "\n"), multiErr
}
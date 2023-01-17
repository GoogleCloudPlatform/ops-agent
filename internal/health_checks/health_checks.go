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
	"os"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/resourcedetector"
)

var (
	healthChecksLogger  *log.Logger
	healthChecksLogPath = "/var/log/google-cloud-ops-agent/health_checks_log.txt"
)

func getGCEMetadata() (resourcedetector.GCEResource, error) {
	MetadataResource, err := resourcedetector.GetResource()
	if err != nil {
		return resourcedetector.GCEResource{}, fmt.Errorf("can't get resource metadata: %w", err)
	}
	if gceMetadata, ok := MetadataResource.(resourcedetector.GCEResource); ok {
		healthChecksLogger.Printf("gceMetadata : %+v", gceMetadata)
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

func (r HealthCheckRegistry) RunAllHealthChecks() []string {
	var result []string
	var message string
	for _, c := range r {
		err := c.RunCheck()
		if err != nil {
			if healthError, ok := err.(HealthCheckError); ok {
				message = fmt.Sprintf("%s - Result: FAIL, ERROR_CODE: %s, Failure: %s, Solution: %s",
					c.Name(), healthError.Code, healthError.Message, healthError.Action)
			} else {
				message = fmt.Sprintf("%s - Result: ERROR, Detail: %s", c.Name(), err.Error())
			}
		} else {
			message = fmt.Sprintf("%s - Result: PASS", c.Name())
		}
		healthChecksLogger.Print(message)
		result = append(result, message)
	}

	return result
}

func init() {
	file, err := os.OpenFile(healthChecksLogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		log.Fatal(err)
	}

	healthChecksLogger = log.New(file, "", log.Ldate|log.Ltime|log.Lshortfile)
}

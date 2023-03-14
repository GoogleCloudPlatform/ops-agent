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
	"log"
	"net/http"
)

var (
	requests = [...]networkCheckRequest{
		{
			name:             "Logging API",
			url:              "https://logging.googleapis.com/$discovery/rest",
			successMessage:   "Request to the Logging API was successful.",
			healthCheckError: LogApiConnErr,
		},
		{
			name:             "Monitoring API",
			url:              "https://monitoring.googleapis.com/$discovery/rest",
			successMessage:   "Request to the Monitoring API was successful.",
			healthCheckError: MonApiConnErr,
		},
		{
			name:             "Packages API",
			url:              "https://packages.cloud.google.com/",
			successMessage:   "Request to packages.cloud.google.com was successful.",
			healthCheckError: PacApiConnErr,
		},
		{
			name:             "dl.google.com",
			url:              "https://dl.google.com",
			successMessage:   "Request to dl.google.com was successful.",
			healthCheckError: DLApiConnErr,
		},
		{
			name:             "GCE Metadata Server",
			url:              "http://metadata.google.internal/computeMetadata/v1",
			successMessage:   "Request to the GCE Metadata server was successful.",
			healthCheckError: MetaApiConnErr,
		},
	}
)

type NetworkCheck struct{}

func (c NetworkCheck) Name() string {
	return "Network Check"
}

type networkCheckRequest struct {
	name             string
	url              string
	successMessage   string
	healthCheckError HealthCheckError
}

func (c NetworkCheck) RunCheck(logger *log.Logger) error {
	for _, r := range requests {
		response, err := http.Get(r.url)
		if err != nil {
			if isTimeoutError(err) || isConnectionRefusedError(err) {
				return r.healthCheckError
			}
			return err
		}
		logger.Printf("%s response status: %s", r.name, response.Status)
		switch response.StatusCode {
		case http.StatusOK:
			logger.Printf(r.successMessage)
		default:
			return r.healthCheckError
		}
	}

	return nil
}

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
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/GoogleCloudPlatform/ops-agent/internal/logs"
	"github.com/GoogleCloudPlatform/ops-agent/internal/platform"
	"github.com/cenkalti/backoff/v4"
)

const MaxRequestElapsedTime = 30 * time.Second

type networkRequest struct {
	name             string
	url              string
	successMessage   string
	healthCheckError HealthCheckError
}

var (
	commonRequests = []networkRequest{
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
		// TODO(b/321220138): restore this once there's a more reliable endpoint.
		// {
		// 	name:             "Packages API",
		// 	url:              "https://packages.cloud.google.com",
		// 	successMessage:   "Request to packages.cloud.google.com was successful.",
		// 	healthCheckError: PacApiConnErr,
		// },
		{
			name:             "dl.google.com",
			url:              "https://dl.google.com",
			successMessage:   "Request to dl.google.com was successful.",
			healthCheckError: DLApiConnErr,
		},
	}
	gceRequests = []networkRequest{
		{
			name:             "GCE Metadata Server",
			url:              "http://metadata.google.internal",
			successMessage:   "Request to the GCE Metadata server was successful.",
			healthCheckError: MetaApiConnErr,
		},
	}
)

func (r networkRequest) SendRequest(logger logs.StructuredLogger) error {
	var response *http.Response
	var err error
	bf := backoff.NewExponentialBackOff()
	bf.MaxElapsedTime = MaxRequestElapsedTime
	expTicker := backoff.NewTicker(bf)

	for range expTicker.C {
		response, err = http.Get(r.url)
		if err == nil && response.StatusCode == http.StatusOK {
			expTicker.Stop()
			break
		}
	}
	if err != nil {
		if isTimeoutError(err) || isConnectionRefusedError(err) {
			return r.healthCheckError
		}
		return err
	}
	logger.Infof("%s response status: %s", r.name, response.Status)
	switch response.StatusCode {
	case http.StatusOK:
		logger.Infof(r.successMessage)
	default:
		return r.healthCheckError
	}
	return nil
}

type NetworkCheck struct{}

func (c NetworkCheck) Name() string {
	return "Network Check"
}

func (c NetworkCheck) RunCheck(logger logs.StructuredLogger) error {
	var networkErrors []error
	ctx := context.TODO()
	p := platform.FromContext(ctx)
	for _, r := range commonRequests {
		networkErrors = append(networkErrors, r.SendRequest(logger))
	}
	if p.ResourceOverride == nil || p.ResourceOverride.MonitoredResource().Type == "gce_instance" {
		for _, r := range gceRequests {
			networkErrors = append(networkErrors, r.SendRequest(logger))
		}
	}

	return errors.Join(networkErrors...)
}

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
	"sync"
	"time"

	"github.com/GoogleCloudPlatform/ops-agent/internal/logs"
	"github.com/GoogleCloudPlatform/ops-agent/internal/platform"
)

type networkRequest struct {
	name             string
	url              string
	successMessage   string
	healthCheckError HealthCheckError
	successCodes     []int
}

func (r networkRequest) isSuccess(statusCode int) bool {
	if len(r.successCodes) == 0 {
		return statusCode == http.StatusOK
	}
	for _, code := range r.successCodes {
		if statusCode == code {
			return true
		}
	}
	return false
}

var (
	commonRequests = []networkRequest{
		{
			name:             "Telemetry API",
			url:              "https://telemetry.googleapis.com/$discovery/rest",
			successMessage:   "Request to the Telemetry API was successful.",
			healthCheckError: TelApiConnErr,
			successCodes:     []int{http.StatusOK, http.StatusForbidden},
		},
		{
			name: "Packages API",
			// We don't really care that the thing being fetched is an RPM key, just
			// that we *can* fetch it at all, regardless of what distro we're running
			// under.
			url:              "https://packages.cloud.google.com/yum/doc/rpm-package-key.gpg",
			successMessage:   "Request to packages.cloud.google.com was successful.",
			healthCheckError: PacApiConnErr,
		},
		{
			name:             "dl.google.com",
			url:              "https://dl.google.com/cloudagents/add-google-cloud-ops-agent-repo.sh",
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

	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	response, err = client.Get(r.url)
	if err != nil {
		if isTimeoutError(err) || isConnectionRefusedError(err) {
			return r.healthCheckError
		}
		return err
	}
	defer response.Body.Close()
	logger.Infof("%s response status: %s", r.name, response.Status)
	if r.isSuccess(response.StatusCode) {
		logger.Infof(r.successMessage)
	} else {
		return r.healthCheckError
	}
	return nil
}

type NetworkCheck struct{}

func (c NetworkCheck) Name() string {
	return "Network Check"
}

func (c NetworkCheck) RunCheck(logger logs.StructuredLogger) error {
	ctx := context.TODO()
	p := platform.FromContext(ctx)
	var requests []networkRequest
	requests = append(requests, commonRequests...)
	if p.ResourceOverride == nil || p.ResourceOverride.MonitoredResource().Type == "gce_instance" {
		requests = append(requests, gceRequests...)
	}

	networkErrors := make([]error, len(requests))
	var wg sync.WaitGroup

	for i, r := range requests {
		wg.Add(1)
		go func(index int, req networkRequest) {
			defer wg.Done()
			networkErrors[index] = req.SendRequest(logger)
		}(i, r)
	}

	wg.Wait()
	return errors.Join(networkErrors...)
}

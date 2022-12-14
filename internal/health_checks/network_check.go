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
	"log"
	"net/http"
)

var (
	// API urls
	loggingAPIUrl    = "https://logging.googleapis.com/$discovery/rest"
	monitoringAPIUrl = "https://monitoring.googleapis.com/$discovery/rest"
)

// func runGetHTTPRequest(url string) (string, string, error) {
// 	resp, err := http.Get(url)
// 	if err != nil {
// 		return "", "", err
// 	}
// 	defer resp.Body.Close()

// 	status := resp.Status
// 	b, err := io.ReadAll(resp.Body)
// 	if err != nil {
// 		return "", "", err
// 	}

// 	return status, string(b), nil
// }

type NetworkCheck struct{}

func (c NetworkCheck) Name() string {
	return "Network Check"
}

func (c NetworkCheck) RunCheck() error {

	// Request to logging API
	response, err := http.Get(loggingAPIUrl)
	log.Printf("http request status : %s", response.Status)
	if err != nil {
		return err
	}
	switch response.StatusCode {
	case http.StatusOK:
		log.Printf("Request to the Logging API was successful.")
	default:
		return LOG_API_CONN_ERR
	}

	// Request to monitoring API
	response, err = http.Get(monitoringAPIUrl)
	log.Printf("http request status : %s", response.Status)
	if err != nil {
		return err
	}
	switch response.StatusCode {
	case http.StatusOK:
		log.Printf("Request to the Logging API was successful.")
	default:
		return LOG_API_CONN_ERR
	}

	return nil
}

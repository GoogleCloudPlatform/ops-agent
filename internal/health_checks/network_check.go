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
    "io"
    "fmt"
    "net/http"

    // "github.com/GoogleCloudPlatform/ops-agent/confgenerator/resourcedetector"
    // "github.com/GoogleCloudPlatform/ops-agent/confgenerator"

    // "context"

    // metricsscope "cloud.google.com/go/monitoring/metricsscope/apiv1"
    // metricsscopepb "cloud.google.com/go/monitoring/metricsscope/apiv1/metricsscopepb"
    // monitoring "cloud.google.com/go/monitoring/apiv3/v2"
)

var (
    // API urls
    loggingAPIUrl = "https://logging.googleapis.com/$discovery/rest"
    monitoringAPIUrl = "https://monitoring.googleapis.com/$discovery/rest"
)

func runGetHTTPRequest(url string) (string, string, error) {
    resp, err := http.Get(url)
    if err != nil {
        return "", "", err
    }
    defer resp.Body.Close()

    status := resp.Status
    b, err := io.ReadAll(resp.Body)
    if err != nil {
        return "", "", err
    }

    return status, string(b), nil
}

type NetworkCheck struct{}

func (c NetworkCheck) RunCheck() (string, error) {
    fmt.Println("\n> NetworkCheck \n \n")

    // Request to logging API
    status, response, err := runGetHTTPRequest(loggingAPIUrl)
    fmt.Println("==>" + status)
    if err != nil {
        fmt.Println(err)
        return "", err
    }
    if status == "200 OK" {
        fmt.Println("==> Query to loggingAPIUrl was successful.")
    } else {
        fmt.Println("==> Query to loggingAPIUrl was not successful.")
        return "", fmt.Errorf("Query to loggingAPIUrl was not successful.")
    }

    // Request to monitoring API
    status, response, err = runGetHTTPRequest(monitoringAPIUrl)
    fmt.Println("==>" + status)
    if err != nil {
        fmt.Println(err)
        return "", err
    }
    if status == "200 OK" {
        fmt.Println("==> Query to monitoringAPIUrl was successful.")
    } else {
        fmt.Println("==> Query to monitoringAPIUrl was not successful.")
        return "", fmt.Errorf("Query to monitoringAPIUrl was not successful.")
    }

    response = response + ""
    return "PASS", nil
}

func init() {
    GCEHealthChecks.RegisterCheck("network_check", NetworkCheck{})
}
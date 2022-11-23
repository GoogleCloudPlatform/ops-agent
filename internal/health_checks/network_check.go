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
    "github.com/GoogleCloudPlatform/ops-agent/confgenerator"
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

type NetworkCheck struct{
    HealthCheck
}

func (c NetworkCheck) RunCheck(uc *confgenerator.UnifiedConfig) error {

    // Request to logging API
    status, _, err := runGetHTTPRequest(loggingAPIUrl)
    c.LogMessage("==>" + status)
    if err != nil {
        fmt.Println(err)
        return err
    }
    if status == "200 OK" {
        c.LogMessage("==> Query to loggingAPIUrl was successful.")
    } else {
        c.Fail("Query to loggingAPIUrl was not successful.", "Check your connection.")
    }

    // Request to monitoring API
    status, _, err = runGetHTTPRequest(monitoringAPIUrl)
    c.LogMessage("==>" + status)
    if err != nil {
        fmt.Println(err)
        return err
    }
    if status == "200 OK" {
        c.LogMessage("==> Query to monitoringAPIUrl was successful.")
    } else {
        c.Fail("Query to monitoringAPIUrl was not successful.", "Check your connection.")
    }

    return nil
}

func init() {
    GCEHealthChecks.RegisterCheck("Network Check", &NetworkCheck{HealthCheck: NewHealthCheck()})
}
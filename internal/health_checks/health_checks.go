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
    "log"
    "fmt"
    "time"
    "net"
    "net/http"

    "go.uber.org/multierr"
    "cloud.google.com/go/logging"
    "github.com/GoogleCloudPlatform/ops-agent/confgenerator/resourcedetector"
    "github.com/GoogleCloudPlatform/ops-agent/confgenerator"

    "context"

    // metricsscope "cloud.google.com/go/monitoring/metricsscope/apiv1"
    // metricsscopepb "cloud.google.com/go/monitoring/metricsscope/apiv1/metricsscopepb"
    monitoring "cloud.google.com/go/monitoring/apiv3/v2"
)

var (
    // MetadataResource is the resource metadata for the instance we're running on.
    // Note: This is a global variable so that it can be set in tests.
    MetadataResource resourcedetector.Resource

    // Expected scopes
    requiredLoggingScopes = []string{
        "https://www.googleapis.com/auth/logging.write",
        "https://www.googleapis.com/auth/logging.admin",
    }
    requiredMonitoringScopes = []string{
        "https://www.googleapis.com/auth/monitoring.write",
        "https://www.googleapis.com/auth/monitoring.admin",
    }

    // API urls
    loggingAPIUrl = "https://logging.googleapis.com/$discovery/rest"
    monitoringAPIUrl = "https://monitoring.googleapis.com/$discovery/rest"
)

func Health_Checks(uc *confgenerator.UnifiedConfig) error {

    MetadataResource, err := resourcedetector.GetResource()
    if err != nil {
        log.Fatalf("can't get resource metadata: %w", err)
    }

    var projectId string
    var defaultScopes []string
    fmt.Println("Get MetadataResource : ")
    if gceMetadata, ok := MetadataResource.(resourcedetector.GCEResource); ok {
        fmt.Println(fmt.Sprintf("gceMetadata : %+v \n \n", gceMetadata))
        projectId = gceMetadata.Project
        defaultScopes = gceMetadata.DefaultScopes
    } else {
        // Not on GCE
        projectId = "Not-on-GCE"
    }
    fmt.Println(fmt.Sprintf("projectId : %s \n \n", projectId))

    var multiErr error
    fmt.Println("Health_Checks \n \n")

    if err := NetworkCheck(); err != nil {
        fmt.Println(fmt.Sprintf("NetworkCheckErr : %s \n \n", err))
    }
    multiErr = multierr.Append(multiErr, err)

    if err := PermissionsCheck(defaultScopes); err != nil {
        fmt.Println(fmt.Sprintf("PermissionsCheckErr : %s \n \n", err))
    }
    multiErr = multierr.Append(multiErr, err)

    if err := APICheck(projectId); err != nil {
        fmt.Println(fmt.Sprintf("APICheckErr : %s \n \n", err))
    }
    multiErr = multierr.Append(multiErr, err)

    if err := PortsCheck(uc); err != nil {
        fmt.Println(fmt.Sprintf("PortsCheckErr : %s \n \n", err))
    }
    multiErr = multierr.Append(multiErr, err)

	return multiErr
}

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

func NetworkCheck() error {
    fmt.Println("\n NetworkCheck \n \n")

    // Request to logging API
    status, response, err := runGetHTTPRequest(loggingAPIUrl)
    fmt.Println(status)
    if err != nil {
        fmt.Println(err)
        return err
    }
    if status == "200 OK" {
        fmt.Println("Query to loggingAPIUrl was successful.")
    } else {
        fmt.Println("Query to loggingAPIUrl was not successful.")
        return fmt.Errorf("Query to loggingAPIUrl was not successful.")
    }

    // Request to monitoring API
    status, response, err = runGetHTTPRequest(monitoringAPIUrl)
    fmt.Println(status)
    if err != nil {
        fmt.Println(err)
        return err
    }
    if status == "200 OK" {
        fmt.Println("Query to monitoringAPIUrl was successful.")
    } else {
        fmt.Println("Query to monitoringAPIUrl was not successful.")
        return fmt.Errorf("Query to monitoringAPIUrl was not successful.")
    }

    response = response + ""
    return nil
}

func APICheck(project string) error {
    fmt.Println("\n APICheck \n \n")
	ctx := context.Background()

    // New Logging Client
    fmt.Println("New Logging Client \n")
    logClient, err := logging.NewClient(ctx, project)
    if err != nil {
        fmt.Println(err)
        return err
    }
    if err := logClient.Ping(ctx); err != nil {
        fmt.Println(err)
        return err
    }
    fmt.Println("==> Logging API Ping succeded")
    logClient.Close()

    // New Monitoring Client
    fmt.Println("New Monitoring Client \n \n")
    monClient, err := monitoring.NewMetricClient(ctx)
    if err != nil {
        fmt.Println(err)
        return err
    }
    fmt.Println("==> Monitoring Client successfully created")
    monClient.Close()

	return nil
}

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

func PermissionsCheck(defaultScopes []string) error {
    fmt.Println("\n PermissionsCheck \n \n")
    
    found, err := constainsAtLeastOne(defaultScopes, requiredLoggingScopes)
    if err != nil {
        return err
    } else if found {
        fmt.Println("Logging Scopes are enough to run the Ops Agent.")
    } else {
        fmt.Println("Logging Scopes are not enough to run the Ops Agent.")
        return fmt.Errorf("Logging Scopes are not enough to run the Ops Agent.")
    }

    found, err = constainsAtLeastOne(defaultScopes, requiredMonitoringScopes)
    if err != nil {
        return err
    } else if found {
        fmt.Println("Monitoring Scopes are enough to run the Ops Agent.")
    } else {
        fmt.Println("Monitoring Scopes are not enough to run the Ops Agent.")
        return fmt.Errorf("Monitoring Scopes are not enough to run the Ops Agent.")
    }

    fmt.Println("\n PermissionsCheck PASSED \n \n")

    return nil
}

func check_port(host string, port string) {

    timeout := time.Second
    c, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), timeout)
    if err != nil {
        fmt.Println("Connection Error:", err)
    }
    if c != nil {
        defer c.Close()
        fmt.Println("Opened", net.JoinHostPort(host, port))    
    }
}

func PortsCheck(uc *confgenerator.UnifiedConfig) error {
    fmt.Println("\n PortsCheck \n \n")

    // Check prometheus exporter host port : 0.0.0.0 : 20202
    host := "0.0.0.0"
    port := "20202"
    check_port(host, port)
    return nil
}
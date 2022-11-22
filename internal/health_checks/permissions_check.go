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

    "github.com/GoogleCloudPlatform/ops-agent/confgenerator/resourcedetector"
    "github.com/GoogleCloudPlatform/ops-agent/confgenerator"
)

var (
    // Expected scopes
    requiredLoggingScopes = []string{
        "https://www.googleapis.com/auth/logging.write",
        "https://www.googleapis.com/auth/logging.admin",
    }
    requiredMonitoringScopes = []string{
        "https://www.googleapis.com/auth/monitoring.write",
        "https://www.googleapis.com/auth/monitoring.admin",
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

func (c PermissionsCheck) RunCheck(uc *confgenerator.UnifiedConfig) (string, error) {

    var project string
    var defaultScopes []string
    fmt.Println("Get MetadataResource : ")
    MetadataResource, err := resourcedetector.GetResource()
    if err != nil {
        return "", fmt.Errorf("can't get resource metadata: %w", err)
    }
    if gceMetadata, ok := MetadataResource.(resourcedetector.GCEResource); ok {
        fmt.Println(fmt.Sprintf("==> gceMetadata : %+v \n \n", gceMetadata))
        project = gceMetadata.Project
        defaultScopes = gceMetadata.DefaultScopes
    } else {
        // Not on GCE
        project = "Not-on-GCE"
    }
    fmt.Println(fmt.Sprintf("==> project : %s \n \n", project))

    fmt.Println("\n> PermissionsCheck \n \n")
    
    found, err := constainsAtLeastOne(defaultScopes, requiredLoggingScopes)
    if err != nil {
        return "", err
    } else if found {
        fmt.Println("==> Logging Scopes are enough to run the Ops Agent.")
    } else {
        fmt.Println("==> Logging Scopes are not enough to run the Ops Agent.")
        return "", fmt.Errorf("Logging Scopes are not enough to run the Ops Agent.")
    }

    found, err = constainsAtLeastOne(defaultScopes, requiredMonitoringScopes)
    if err != nil {
        return "", err
    } else if found {
        fmt.Println("==> Monitoring Scopes are enough to run the Ops Agent.")
    } else {
        fmt.Println("==> Monitoring Scopes are not enough to run the Ops Agent.")
        return "", fmt.Errorf("Monitoring Scopes are not enough to run the Ops Agent.")
    }

    fmt.Println("\n> PermissionsCheck PASSED \n \n")

    return "PASS", nil
}


func init() {
    GCEHealthChecks.RegisterCheck("permissions_check", PermissionsCheck{})
}
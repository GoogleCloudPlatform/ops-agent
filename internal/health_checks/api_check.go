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
    "cloud.google.com/go/logging"
    "github.com/GoogleCloudPlatform/ops-agent/confgenerator/resourcedetector"
    "github.com/GoogleCloudPlatform/ops-agent/confgenerator"
    "context"

    monitoring "cloud.google.com/go/monitoring/apiv3/v2"
)

type APICheck struct{}

func (c APICheck) RunCheck(uc *confgenerator.UnifiedConfig) (string, error) {
    var project string
    fmt.Println("Get MetadataResource : ")
    MetadataResource, err := resourcedetector.GetResource()
    if err != nil {
        return "", fmt.Errorf("can't get resource metadata: %w", err)
    }
    if gceMetadata, ok := MetadataResource.(resourcedetector.GCEResource); ok {
        fmt.Println(fmt.Sprintf("==> gceMetadata : %+v \n \n", gceMetadata))
        project = gceMetadata.Project
    } else {
        // Not on GCE
        project = "Not-on-GCE"
    }
    fmt.Println(fmt.Sprintf("==> project : %s \n \n", project))

    fmt.Println("\n> APICheck \n \n")
    ctx := context.Background()

    // New Logging Client
    fmt.Println("==> New Logging Client \n")
    logClient, err := logging.NewClient(ctx, project)
    if err != nil {
        fmt.Println(err)
        return "", err
    }
    if err := logClient.Ping(ctx); err != nil {
        fmt.Println(err)
        return "", err
    }
    fmt.Println("==> Logging API Ping succeded")
    logClient.Close()

    // New Monitoring Client
    fmt.Println("==> New Monitoring Client \n \n")
    monClient, err := monitoring.NewMetricClient(ctx)
    if err != nil {
        fmt.Println(err)
        return "", err
    }
    fmt.Println("==> Monitoring Client successfully created")
    monClient.Close()

    return "PASS", nil
}

func init() {
    //var elem HealthCheck
    //elem = APICheck{}
    GCEHealthChecks.RegisterCheck("api_check", APICheck{})
}
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
    "fmt"

    "cloud.google.com/go/logging"
    "github.com/GoogleCloudPlatform/ops-agent/confgenerator/resourcedetector"
    "github.com/GoogleCloudPlatform/ops-agent/confgenerator"

    "context"

    metricsscope "cloud.google.com/go/monitoring/metricsscope/apiv1"
    metricsscopepb "cloud.google.com/go/monitoring/metricsscope/apiv1/metricsscopepb"
)

var (
    // MetadataResource is the resource metadata for the instance we're running on.
    // Note: This is a global variable so that it can be set in tests.
    MetadataResource resourcedetector.Resource
)

func Health_Checks(uc *confgenerator.UnifiedConfig) error {

    MetadataResource, err := resourcedetector.GetResource()
    if err != nil {
        log.Fatalf("can't get resource metadata: %w", err)
    }

    var projectId string
    fmt.Println("Get MetadataResource : ")
    if gceMetadata, ok := MetadataResource.(resourcedetector.GCEResource); ok {
        fmt.Println(fmt.Sprintf("gceMetadata : %+v \n \n", gceMetadata))
        projectId = gceMetadata.Project
    } else {
        // Not on GCE
        projectId = "Not-on-GCE"
    }
    fmt.Println(fmt.Sprintf("projectId : %s \n \n", projectId))


    fmt.Println("Health_Checks \n \n")

    if err := APICheck(projectId); err != nil {
        log.Fatalf("APICheck : %s \n \n", err)
    }

    if err := PortsCheck(uc); err != nil {
        log.Fatalf("PortsCheck : %s \n \n", err)
    }

	return nil
}

func APICheck(project string) error {
    fmt.Println("APICheck \n \n")
	ctx := context.Background()
    client, err := logging.NewClient(ctx, project)
    if err != nil {
            fmt.Println(err)
            return err
    }
    if err := client.Ping(ctx); err != nil {
            fmt.Println(err)
            return err
    }
    fmt.Println("Ping succeded")

	return nil
}

func PortsCheck(uc *confgenerator.UnifiedConfig) error {
    fmt.Println("PortsCheck \n \n")
    return nil
}

func PermissionsCheck(project string) error {
    fmt.Println("PermissionsCheck \n \n")
    ctx := context.Background()
    c, err := metricsscope.NewMetricsScopesClient(ctx)
    if err != nil {
        return err
    }
    defer c.Close()

    req := &metricsscopepb.ListMetricsScopesByMonitoredProjectRequest{
        // TODO: Fill request struct fields.
        // See https://pkg.go.dev/cloud.google.com/go/monitoring/metricsscope/apiv1/metricsscopepb#ListMetricsScopesByMonitoredProjectRequest.
        
    }
    resp, err := c.ListMetricsScopesByMonitoredProject(ctx, req)
    if err != nil {
        return err
    }
    _ = resp

    return nil
}
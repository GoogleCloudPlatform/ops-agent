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

    "context"

    apikeys "cloud.google.com/go/apikeys/apiv2"
    "google.golang.org/api/iterator"

    apikeyspb "google.golang.org/genproto/googleapis/api/apikeys/v2"
)
var (
    // MetadataResource is the resource metadata for the instance we're running on.
    // Note: This is a global variable so that it can be set in tests.
    MetadataResource resourcedetector.Resource
)

func Health_Checks() error {

    MetadataResource, err := resourcedetector.GetResource()
    if err != nil {
        log.Fatalf("can't get resource metadata: %w", err)
    }

    var projectId string
    if gceMetadata, ok := MetadataResource.(resourcedetector.GCEResource); ok {
        projectId = gceMetadata.Project
    } else {
        // Not on GCE
        projectId = "Not-on-GCE"
    }
    fmt.Println(projectId)

    if err := APIChecks(projectId); err != nil {
        log.Fatalf("APIChecks : %s", err)
    }
    
	fmt.Println("Health_Checks")
	ctx := context.Background()
    // This snippet has been automatically generated and should be regarded as a code template only.
    // It will require modifications to work:
    // - It may require correct/in-range values for request initialization.
    // - It may require specifying regional endpoints when creating the service client as shown in:
    //   https://pkg.go.dev/cloud.google.com/go#hdr-Client_Options
    c, err := apikeys.NewClient(ctx)
    fmt.Println(c)
    if err != nil {
            // TODO: Handle error.
    }
    defer c.Close()

    req := &apikeyspb.ListKeysRequest{
    	Parent: "fcovalente-dev",
        // TODO: Fill request struct fields.
        // See https://pkg.go.dev/google.golang.org/genproto/googleapis/api/apikeys/v2#ListKeysRequest.
    }
    it := c.ListKeys(ctx, req)
    for {
            resp, err := it.Next()
            fmt.Println(resp)
            if err == iterator.Done {
                    break
            }
            if err != nil {
            		fmt.Println(err)
                    // TODO: Handle error.
                    break
            }
            // TODO: Use resp.
            _ = resp
    }

	return nil
}

func APIChecks(project string) error {

	ctx := context.Background()
    client, err := logging.NewClient(ctx, project)
    if err != nil {
            // TODO: Handle error.
            fmt.Println(err)
    }
    if err := client.Ping(ctx); err != nil {
            // TODO: Handle error.
            fmt.Println(err)
    }
    fmt.Println("Ping succeded")

	return nil
}